/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package awstasks

import (
	"fmt"

	"reflect"
	"sort"
	"strings"

	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
	"k8s.io/kops/upup/pkg/fi/cloudup/cloudformation"
	"k8s.io/kops/upup/pkg/fi/cloudup/terraform"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/golang/glog"
)

const CloudTagInstanceGroupRolePrefix = "k8s.io/role/"

// AutoscalingGroup defined the specification for a autoscaling group
type AutoscalingGroup struct {
	Name      *string
	Lifecycle *fi.Lifecycle

	Granularity         *string
	LaunchConfiguration *LaunchConfiguration
	MaxSize             *int64
	Metrics             []string
	MinSize             *int64
	Subnets             []*Subnet
	SuspendProcesses    *[]string
	Tags                map[string]string
}

var _ fi.CompareWithID = &AutoscalingGroup{}

// CompareWithID is used to compare resource ids
func (e *AutoscalingGroup) CompareWithID() *string {
	return e.Name
}

// findAutoscalingGroup is responsible for finding the autoscaling group
func findAutoscalingGroup(cloud awsup.AWSCloud, name string) (*autoscaling.Group, error) {
	request := &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []*string{&name},
	}

	var found []*autoscaling.Group
	err := cloud.Autoscaling().DescribeAutoScalingGroupsPages(request, func(p *autoscaling.DescribeAutoScalingGroupsOutput, lastPage bool) (shouldContinue bool) {
		for _, g := range p.AutoScalingGroups {
			// Check for "Delete in progress" (the only use of
			// .Status). We won't be able to update or create while
			// this is true, but filtering it out here makes the error
			// messages slightly clearer.
			if g.Status != nil {
				glog.Warningf("Skipping AutoScalingGroup %v: %v", *g.AutoScalingGroupName, *g.Status)
				continue
			}

			if aws.StringValue(g.AutoScalingGroupName) == name {
				found = append(found, g)
			} else {
				glog.Warningf("Got ASG with unexpected name %q", aws.StringValue(g.AutoScalingGroupName))
			}
		}

		return true
	})

	if err != nil {
		return nil, fmt.Errorf("error listing AutoscalingGroups: %v", err)
	}

	if len(found) == 0 {
		return nil, nil
	}

	if len(found) != 1 {
		return nil, fmt.Errorf("Found multiple AutoscalingGroups with name %q", name)
	}

	return found[0], nil
}

// Find is just a wrapper for the find method
func (e *AutoscalingGroup) Find(c *fi.Context) (*AutoscalingGroup, error) {
	cloud := c.Cloud.(awsup.AWSCloud)

	g, err := findAutoscalingGroup(cloud, *e.Name)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, nil
	}

	actual := &AutoscalingGroup{}
	actual.Name = g.AutoScalingGroupName
	actual.MinSize = g.MinSize
	actual.MaxSize = g.MaxSize

	if g.VPCZoneIdentifier != nil {
		subnets := strings.Split(*g.VPCZoneIdentifier, ",")
		for _, subnet := range subnets {
			actual.Subnets = append(actual.Subnets, &Subnet{ID: aws.String(subnet)})
		}
	}

	for _, enabledMetric := range g.EnabledMetrics {
		actual.Metrics = append(actual.Metrics, aws.StringValue(enabledMetric.Metric))
		actual.Granularity = enabledMetric.Granularity
	}
	sort.Strings(actual.Metrics)

	if len(g.Tags) != 0 {
		actual.Tags = make(map[string]string)
		for _, tag := range g.Tags {
			actual.Tags[*tag.Key] = *tag.Value
		}
	}

	if fi.StringValue(g.LaunchConfigurationName) == "" {
		glog.Warningf("autoscaling Group %q had no LaunchConfiguration", fi.StringValue(g.AutoScalingGroupName))
	} else {
		actual.LaunchConfiguration = &LaunchConfiguration{ID: g.LaunchConfigurationName}
	}

	if subnetSlicesEqualIgnoreOrder(actual.Subnets, e.Subnets) {
		actual.Subnets = e.Subnets
	}

	processes := []string{}
	for _, p := range g.SuspendedProcesses {
		processes = append(processes, *p.ProcessName)
	}

	actual.SuspendProcesses = &processes

	// Avoid spurious changes
	actual.Lifecycle = e.Lifecycle

	return actual, nil
}

func (e *AutoscalingGroup) normalize(c *fi.Context) error {
	sort.Strings(e.Metrics)

	return nil
}

func (e *AutoscalingGroup) Run(c *fi.Context) error {
	err := e.normalize(c)
	if err != nil {
		return err
	}
	c.Cloud.(awsup.AWSCloud).AddTags(e.Name, e.Tags)

	return fi.DefaultDeltaRunMethod(e, c)
}

func (s *AutoscalingGroup) CheckChanges(a, e, changes *AutoscalingGroup) error {
	if a != nil {
		if e.Name == nil {
			return fi.RequiredField("Name")
		}
	}
	return nil
}

func (e *AutoscalingGroup) buildTags(cloud fi.Cloud) map[string]string {
	tags := make(map[string]string)
	for k, v := range e.Tags {
		tags[k] = v
	}
	return tags
}

// RenderAWS is responsible for creating the autoscaling group
func (_ *AutoscalingGroup) RenderAWS(t *awsup.AWSAPITarget, a, e, changes *AutoscalingGroup) error {
	tags := []*autoscaling.Tag{}

	for k, v := range e.buildTags(t.Cloud) {
		tags = append(tags, &autoscaling.Tag{
			Key:               aws.String(k),
			PropagateAtLaunch: aws.Bool(true),
			ResourceId:        e.Name,
			ResourceType:      aws.String("auto-scaling-group"),
			Value:             aws.String(v),
		})
	}

	if a == nil {
		glog.V(2).Infof("Creating autoscaling Group with Name: %q", *e.Name)

		request := &autoscaling.CreateAutoScalingGroupInput{
			AutoScalingGroupName:    e.Name,
			LaunchConfigurationName: e.LaunchConfiguration.ID,
			MaxSize:                 e.MaxSize,
			MinSize:                 e.MinSize,
			Tags:                    tags,
		}

		var subnetIDs []string
		for _, s := range e.Subnets {
			subnetIDs = append(subnetIDs, *s.ID)
		}
		request.VPCZoneIdentifier = aws.String(strings.Join(subnetIDs, ","))

		if _, err := t.Cloud.Autoscaling().CreateAutoScalingGroup(request); err != nil {
			return fmt.Errorf("error creating AutoscalingGroup: %v", err)
		}

		if _, err := t.Cloud.Autoscaling().EnableMetricsCollection(&autoscaling.EnableMetricsCollectionInput{
			AutoScalingGroupName: e.Name,
			Granularity:          e.Granularity,
			Metrics:              aws.StringSlice(e.Metrics),
		}); err != nil {
			return fmt.Errorf("error enabling metrics collection for AutoscalingGroup: %v", err)
		}

		if len(*e.SuspendProcesses) > 0 {
			toSuspend := []*string{}
			for _, p := range *e.SuspendProcesses {
				toSuspend = append(toSuspend, &p)
			}

			processQuery := &autoscaling.ScalingProcessQuery{}
			processQuery.AutoScalingGroupName = e.Name
			processQuery.ScalingProcesses = toSuspend

			_, err := t.Cloud.Autoscaling().SuspendProcesses(processQuery)
			if err != nil {
				return fmt.Errorf("error suspending processes: %v", err)
			}
		}
	} else {
		request := &autoscaling.UpdateAutoScalingGroupInput{
			AutoScalingGroupName: e.Name,
		}

		if changes.LaunchConfiguration != nil {
			request.LaunchConfigurationName = e.LaunchConfiguration.ID
			changes.LaunchConfiguration = nil
		}
		if changes.MinSize != nil {
			request.MinSize = e.MinSize
			changes.MinSize = nil
		}
		if changes.MaxSize != nil {
			request.MaxSize = e.MaxSize
			changes.MaxSize = nil
		}
		if changes.Subnets != nil {
			var subnetIDs []string
			for _, s := range e.Subnets {
				subnetIDs = append(subnetIDs, *s.ID)
			}
			request.VPCZoneIdentifier = aws.String(strings.Join(subnetIDs, ","))
			changes.Subnets = nil
		}

		var updateTagsRequest *autoscaling.CreateOrUpdateTagsInput
		var deleteTagsRequest *autoscaling.DeleteTagsInput
		if changes.Tags != nil {
			updateTagsRequest = &autoscaling.CreateOrUpdateTagsInput{Tags: tags}

			if a != nil && len(a.Tags) > 0 {
				deleteTagsRequest = &autoscaling.DeleteTagsInput{}
				deleteTagsRequest.Tags = e.getASGTagsToDelete(a.Tags)
			}

			changes.Tags = nil
		}

		if changes.Metrics != nil || changes.Granularity != nil {
			// TODO: Support disabling metrics?
			if len(e.Metrics) != 0 {
				_, err := t.Cloud.Autoscaling().EnableMetricsCollection(&autoscaling.EnableMetricsCollectionInput{
					AutoScalingGroupName: e.Name,
					Granularity:          e.Granularity,
					Metrics:              aws.StringSlice(e.Metrics),
				})
				if err != nil {
					return fmt.Errorf("error enabling metrics collection for AutoscalingGroup: %v", err)
				}
				changes.Metrics = nil
				changes.Granularity = nil
			}
		}

		if changes.SuspendProcesses != nil {
			toSuspend := processCompare(e.SuspendProcesses, a.SuspendProcesses)
			toResume := processCompare(a.SuspendProcesses, e.SuspendProcesses)

			if len(toSuspend) > 0 {
				suspendProcessQuery := &autoscaling.ScalingProcessQuery{}
				suspendProcessQuery.AutoScalingGroupName = e.Name
				suspendProcessQuery.ScalingProcesses = toSuspend

				_, err := t.Cloud.Autoscaling().SuspendProcesses(suspendProcessQuery)
				if err != nil {
					return fmt.Errorf("error suspending processes: %v", err)
				}
			}
			if len(toResume) > 0 {
				resumeProcessQuery := &autoscaling.ScalingProcessQuery{}
				resumeProcessQuery.AutoScalingGroupName = e.Name
				resumeProcessQuery.ScalingProcesses = toResume

				_, err := t.Cloud.Autoscaling().ResumeProcesses(resumeProcessQuery)
				if err != nil {
					return fmt.Errorf("error resuming processes: %v", err)
				}
			}
			changes.SuspendProcesses = nil
		}

		empty := &AutoscalingGroup{}
		if !reflect.DeepEqual(empty, changes) {
			glog.Warningf("cannot apply changes to AutoScalingGroup: %v", changes)
		}

		glog.V(2).Infof("Updating autoscaling group %s", *e.Name)

		if _, err := t.Cloud.Autoscaling().UpdateAutoScalingGroup(request); err != nil {
			return fmt.Errorf("error updating AutoscalingGroup: %v", err)
		}

		if updateTagsRequest != nil {
			if _, err := t.Cloud.Autoscaling().CreateOrUpdateTags(updateTagsRequest); err != nil {
				return fmt.Errorf("error updating AutoscalingGroup tags: %v", err)
			}
		}

		if deleteTagsRequest != nil && len(deleteTagsRequest.Tags) > 0 {
			if _, err := t.Cloud.Autoscaling().DeleteTags(deleteTagsRequest); err != nil {
				return fmt.Errorf("error deleting old AutoscalingGroup tags: %v", err)
			}
		}
	}

	// TODO: Use PropagateAtLaunch = false for tagging?

	return nil // We have
}

// processCompare returns processes that exist in a but not in b
func processCompare(a *[]string, b *[]string) []*string {
	notInB := []*string{}
	for _, ap := range *a {
		found := false
		for _, bp := range *b {
			if ap == bp {
				found = true
				break
			}
		}
		if !found {
			notFound := ap
			notInB = append(notInB, &notFound)
		}
	}
	return notInB
}

// getASGTagsToDelete loops through the currently set tags and builds a list of
// tags to be deleted from the Autoscaling Group
func (e *AutoscalingGroup) getASGTagsToDelete(currentTags map[string]string) []*autoscaling.Tag {
	tagsToDelete := []*autoscaling.Tag{}

	for k, v := range currentTags {
		if _, ok := e.Tags[k]; !ok {
			tagsToDelete = append(tagsToDelete, &autoscaling.Tag{
				Key:          aws.String(k),
				Value:        aws.String(v),
				ResourceId:   e.Name,
				ResourceType: aws.String("auto-scaling-group"),
			})
		}
	}
	return tagsToDelete
}

type terraformASGTag struct {
	Key               *string `json:"key"`
	Value             *string `json:"value"`
	PropagateAtLaunch *bool   `json:"propagate_at_launch"`
}
type terraformAutoscalingGroup struct {
	Name                    *string              `json:"name,omitempty"`
	LaunchConfigurationName *terraform.Literal   `json:"launch_configuration,omitempty"`
	MaxSize                 *int64               `json:"max_size,omitempty"`
	MinSize                 *int64               `json:"min_size,omitempty"`
	VPCZoneIdentifier       []*terraform.Literal `json:"vpc_zone_identifier,omitempty"`
	Tags                    []*terraformASGTag   `json:"tag,omitempty"`
	MetricsGranularity      *string              `json:"metrics_granularity,omitempty"`
	EnabledMetrics          []*string            `json:"enabled_metrics,omitempty"`
	SuspendedProcesses      []*string            `json:"suspended_processes,omitempty"`
}

// RenderTerraform is responsible for rendering the terraform
func (_ *AutoscalingGroup) RenderTerraform(t *terraform.TerraformTarget, a, e, changes *AutoscalingGroup) error {

	tf := &terraformAutoscalingGroup{
		Name:                    e.Name,
		MinSize:                 e.MinSize,
		MaxSize:                 e.MaxSize,
		LaunchConfigurationName: e.LaunchConfiguration.TerraformLink(),
		MetricsGranularity:      e.Granularity,
		EnabledMetrics:          aws.StringSlice(e.Metrics),
	}

	for _, s := range e.Subnets {
		tf.VPCZoneIdentifier = append(tf.VPCZoneIdentifier, s.TerraformLink())
	}

	tags := e.buildTags(t.Cloud)
	// Make sure we output in a stable order
	var tagKeys []string
	for k := range tags {
		tagKeys = append(tagKeys, k)
	}
	sort.Strings(tagKeys)
	for _, k := range tagKeys {
		v := tags[k]
		tf.Tags = append(tf.Tags, &terraformASGTag{
			Key:               fi.String(k),
			Value:             fi.String(v),
			PropagateAtLaunch: fi.Bool(true),
		})
	}

	if e.LaunchConfiguration != nil {
		// Create TF output variable with security group ids
		// This is in the launch configuration, but the ASG has the information about the instance group type

		role := ""
		for k := range e.Tags {
			if strings.HasPrefix(k, CloudTagInstanceGroupRolePrefix) {
				suffix := strings.TrimPrefix(k, CloudTagInstanceGroupRolePrefix)
				if role != "" && role != suffix {
					return fmt.Errorf("Found multiple role tags: %q vs %q", role, suffix)
				}
				role = suffix
			}
		}

		if role != "" {
			for _, sg := range e.LaunchConfiguration.SecurityGroups {
				if err := t.AddOutputVariableArray(role+"_security_group_ids", sg.TerraformLink()); err != nil {
					return err
				}
			}
			if err := t.AddOutputVariableArray(role+"_autoscaling_group_ids", e.TerraformLink()); err != nil {
				return err
			}
		}

		if role == "node" {
			for _, s := range e.Subnets {
				if err := t.AddOutputVariableArray(role+"_subnet_ids", s.TerraformLink()); err != nil {
					return err
				}
			}
		}
	}

	var processes []*string
	if e.SuspendProcesses != nil {
		for _, p := range *e.SuspendProcesses {
			processes = append(processes, fi.String(p))
		}
	}
	tf.SuspendedProcesses = processes

	return t.RenderResource("aws_autoscaling_group", *e.Name, tf)
}

func (e *AutoscalingGroup) TerraformLink() *terraform.Literal {
	return terraform.LiteralProperty("aws_autoscaling_group", *e.Name, "id")
}

type cloudformationASGTag struct {
	Key               *string `json:"Key"`
	Value             *string `json:"Value"`
	PropagateAtLaunch *bool   `json:"PropagateAtLaunch"`
}

type cloudformationASGMetricsCollection struct {
	Granularity *string   `json:"Granularity"`
	Metrics     []*string `json:"Metrics"`
}
type cloudformationAutoscalingGroup struct {
	Name                    *string                               `json:"AutoScalingGroupName,omitempty"`
	LaunchConfigurationName *cloudformation.Literal               `json:"LaunchConfigurationName,omitempty"`
	MaxSize                 *int64                                `json:"MaxSize,omitempty"`
	MinSize                 *int64                                `json:"MinSize,omitempty"`
	VPCZoneIdentifier       []*cloudformation.Literal             `json:"VPCZoneIdentifier,omitempty"`
	Tags                    []*cloudformationASGTag               `json:"Tags,omitempty"`
	MetricsCollection       []*cloudformationASGMetricsCollection `json:"MetricsCollection,omitempty"`

	LoadBalancerNames []*cloudformation.Literal `json:"LoadBalancerNames,omitempty"`
	TargetGroupARNs   []*cloudformation.Literal `json:"TargetGroupARNs,omitempty"`
}

func (_ *AutoscalingGroup) RenderCloudformation(t *cloudformation.CloudformationTarget, a, e, changes *AutoscalingGroup) error {
	tf := &cloudformationAutoscalingGroup{
		Name:    e.Name,
		MinSize: e.MinSize,
		MaxSize: e.MaxSize,
		MetricsCollection: []*cloudformationASGMetricsCollection{
			{
				Granularity: e.Granularity,
				Metrics:     aws.StringSlice(e.Metrics),
			},
		},
		LaunchConfigurationName: e.LaunchConfiguration.CloudformationLink(),
	}

	for _, s := range e.Subnets {
		tf.VPCZoneIdentifier = append(tf.VPCZoneIdentifier, s.CloudformationLink())
	}

	tags := e.buildTags(t.Cloud)
	// Make sure we output in a stable order
	var tagKeys []string
	for k := range tags {
		tagKeys = append(tagKeys, k)
	}
	sort.Strings(tagKeys)
	for _, k := range tagKeys {
		v := tags[k]
		tf.Tags = append(tf.Tags, &cloudformationASGTag{
			Key:               fi.String(k),
			Value:             fi.String(v),
			PropagateAtLaunch: fi.Bool(true),
		})
	}

	return t.RenderResource("AWS::AutoScaling::AutoScalingGroup", *e.Name, tf)
}

func (e *AutoscalingGroup) CloudformationLink() *cloudformation.Literal {
	return cloudformation.Ref("AWS::AutoScaling::AutoScalingGroup", *e.Name)
}
