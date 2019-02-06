/*
Copyright 2018 The Kubernetes Authors.

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

package alitasks

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/ess"
	"github.com/golang/glog"

	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/aliup"
	"k8s.io/kops/upup/pkg/fi/cloudup/terraform"
)

//go:generate fitask -type=LaunchConfiguration

type LaunchConfiguration struct {
	Lifecycle       *fi.Lifecycle
	Name            *string
	ConfigurationId *string

	ImageId            *string
	InstanceType       *string
	SystemDiskSize     *int
	SystemDiskCategory *string

	RAMRole       *RAMRole
	ScalingGroup  *ScalingGroup
	SSHKey        *SSHKey
	UserData      *fi.ResourceHolder
	SecurityGroup *SecurityGroup

	Tags map[string]string
}

var _ fi.CompareWithID = &LaunchConfiguration{}

func (l *LaunchConfiguration) CompareWithID() *string {
	return l.ConfigurationId
}

func (l *LaunchConfiguration) Find(c *fi.Context) (*LaunchConfiguration, error) {

	if l.ScalingGroup == nil || l.ScalingGroup.ScalingGroupId == nil {
		glog.V(4).Infof("ScalingGroup / ScalingGroupId not found for %s, skipping Find", fi.StringValue(l.Name))
		return nil, nil
	}

	cloud := c.Cloud.(aliup.ALICloud)

	describeScalingConfigurationsArgs := &ess.DescribeScalingConfigurationsArgs{
		RegionId:                 common.Region(cloud.Region()),
		ScalingConfigurationName: common.FlattenArray{fi.StringValue(l.Name)},
		ScalingGroupId:           fi.StringValue(l.ScalingGroup.ScalingGroupId),
	}

	if l.ScalingGroup != nil && l.ScalingGroup.ScalingGroupId != nil {
		describeScalingConfigurationsArgs.ScalingGroupId = fi.StringValue(l.ScalingGroup.ScalingGroupId)
	}

	configList, _, err := cloud.EssClient().DescribeScalingConfigurations(describeScalingConfigurationsArgs)
	if err != nil {
		return nil, fmt.Errorf("error finding ScalingConfigurations: %v", err)
	}
	// Don't exist ScalingConfigurations with specified  Name.
	if len(configList) == 0 {
		return nil, nil
	}
	if len(configList) > 1 {
		glog.V(4).Infof("The number of specified ScalingConfigurations with the same name and ScalingGroupId exceeds 1, diskName:%q", *l.Name)
	}

	glog.V(2).Infof("found matching LaunchConfiguration: %q", *l.Name)

	actual := &LaunchConfiguration{}
	actual.ImageId = fi.String(configList[0].ImageId)
	actual.InstanceType = fi.String(configList[0].InstanceType)
	actual.SystemDiskCategory = fi.String(string(configList[0].SystemDiskCategory))
	actual.ConfigurationId = fi.String(configList[0].ScalingConfigurationId)
	actual.Name = fi.String(configList[0].ScalingConfigurationName)

	if configList[0].KeyPairName != "" {
		actual.SSHKey = &SSHKey{
			Name: fi.String(configList[0].KeyPairName),
		}
	}

	if configList[0].RamRoleName != "" {
		actual.RAMRole = &RAMRole{
			Name: fi.String(configList[0].RamRoleName),
		}
	}

	if configList[0].UserData != "" {
		userData, err := base64.StdEncoding.DecodeString(configList[0].UserData)
		if err != nil {
			return nil, fmt.Errorf("error decoding UserData: %v", err)
		}
		actual.UserData = fi.WrapResource(fi.NewStringResource(string(userData)))
	}

	actual.ScalingGroup = &ScalingGroup{
		ScalingGroupId: fi.String(configList[0].ScalingGroupId),
	}
	actual.SecurityGroup = &SecurityGroup{
		SecurityGroupId: fi.String(configList[0].SecurityGroupId),
	}

	if len(configList[0].Tags.Tag) != 0 {
		for _, tag := range configList[0].Tags.Tag {
			actual.Tags[tag.Key] = tag.Value
		}
	}

	// Ignore "system" fields
	actual.Lifecycle = l.Lifecycle
	return actual, nil
}

func (l *LaunchConfiguration) Run(c *fi.Context) error {
	c.Cloud.(aliup.ALICloud).AddClusterTags(l.Tags)
	return fi.DefaultDeltaRunMethod(l, c)
}

func (_ *LaunchConfiguration) CheckChanges(a, e, changes *LaunchConfiguration) error {
	//Configuration can not be modified, we need to create a new one

	if e.Name == nil {
		return fi.RequiredField("Name")
	}

	if e.ImageId == nil {
		return fi.RequiredField("ImageId")
	}
	if e.InstanceType == nil {
		return fi.RequiredField("InstanceType")
	}

	return nil
}

func (_ *LaunchConfiguration) RenderALI(t *aliup.ALIAPITarget, a, e, changes *LaunchConfiguration) error {

	glog.V(2).Infof("Creating LaunchConfiguration for ScalingGroup:%q", fi.StringValue(e.ScalingGroup.ScalingGroupId))

	createScalingConfiguration := &ess.CreateScalingConfigurationArgs{
		ScalingGroupId:      fi.StringValue(e.ScalingGroup.ScalingGroupId),
		ImageId:             fi.StringValue(e.ImageId),
		InstanceType:        fi.StringValue(e.InstanceType),
		SecurityGroupId:     fi.StringValue(e.SecurityGroup.SecurityGroupId),
		SystemDisk_Size:     common.UnderlineString(strconv.Itoa(fi.IntValue(e.SystemDiskSize))),
		SystemDisk_Category: common.UnderlineString(fi.StringValue(e.SystemDiskCategory)),
	}

	if e.RAMRole != nil && e.RAMRole.Name != nil {
		createScalingConfiguration.RamRoleName = fi.StringValue(e.RAMRole.Name)
	}

	if e.UserData != nil {
		userData, err := e.UserData.AsString()
		if err != nil {
			return fmt.Errorf("error rendering ScalingLaunchConfiguration UserData: %v", err)
		}
		createScalingConfiguration.UserData = userData
	}

	if e.SSHKey != nil && e.SSHKey.Name != nil {
		createScalingConfiguration.KeyPairName = fi.StringValue(e.SSHKey.Name)
	}

	if e.Tags != nil {
		tagItem, err := json.Marshal(e.Tags)
		if err != nil {
			return fmt.Errorf("error rendering ScalingLaunchConfiguration Tags: %v", err)
		}
		createScalingConfiguration.Tags = string(tagItem)
	}

	createScalingConfigurationResponse, err := t.Cloud.EssClient().CreateScalingConfiguration(createScalingConfiguration)
	if err != nil {
		return fmt.Errorf("error creating scalingConfiguration: %v", err)
	}
	e.ConfigurationId = fi.String(createScalingConfigurationResponse.ScalingConfigurationId)

	// Disable ScalingGroup, used to bind scalingConfig, we should execute EnableScalingGroup in the task LaunchConfiguration
	// If the ScalingGroup is active, we can not execute EnableScalingGroup.
	if e.ScalingGroup.Active != nil && fi.BoolValue(e.ScalingGroup.Active) {

		glog.V(2).Infof("Disabling LoadBalancer with id:%q", fi.StringValue(e.ScalingGroup.ScalingGroupId))

		disableScalingGroupArgs := &ess.DisableScalingGroupArgs{
			ScalingGroupId: fi.StringValue(e.ScalingGroup.ScalingGroupId),
		}
		_, err := t.Cloud.EssClient().DisableScalingGroup(disableScalingGroupArgs)
		if err != nil {
			return fmt.Errorf("error disabling scalingGroup: %v", err)
		}
	}

	//Enable this configuration
	enableScalingGroupArgs := &ess.EnableScalingGroupArgs{
		ScalingGroupId:               fi.StringValue(e.ScalingGroup.ScalingGroupId),
		ActiveScalingConfigurationId: fi.StringValue(e.ConfigurationId),
	}

	glog.V(2).Infof("Enabling new LaunchConfiguration of LoadBalancer with id:%q", fi.StringValue(e.ScalingGroup.ScalingGroupId))

	_, err = t.Cloud.EssClient().EnableScalingGroup(enableScalingGroupArgs)
	if err != nil {
		return fmt.Errorf("error enabling scalingGroup: %v", err)
	}

	return nil
}

type terraformLaunchConfiguration struct {
	ImageID            *string `json:"image_id ,omitempty"`
	InstanceType       *string `json:"instance_type,omitempty"`
	SystemDiskCategory *string `json:"system_disk_category,omitempty"`
	UserData           *string `json:"user_data,omitempty"`

	RAMRole       *terraform.Literal `json:"role_name,omitempty"`
	ScalingGroup  *terraform.Literal `json:"scaling_group_id,omitempty"`
	SSHKey        *terraform.Literal `json:"key_name,omitempty"`
	SecurityGroup *terraform.Literal `json:"security_group_id,omitempty"`
}

func (_ *LaunchConfiguration) RenderTerraform(t *terraform.TerraformTarget, a, e, changes *LaunchConfiguration) error {
	data, err := e.UserData.AsBytes()
	if err != nil {
		return fmt.Errorf("error rendering ScalingLaunchConfiguration UserData: %v", err)
	}

	userData := base64.StdEncoding.EncodeToString(data)

	tf := &terraformLaunchConfiguration{
		ImageID:            e.ImageId,
		InstanceType:       e.InstanceType,
		SystemDiskCategory: e.SystemDiskCategory,
		UserData:           &userData,

		RAMRole:       e.RAMRole.TerraformLink(),
		ScalingGroup:  e.ScalingGroup.TerraformLink(),
		SSHKey:        e.SSHKey.TerraformLink(),
		SecurityGroup: e.SecurityGroup.TerraformLink(),
	}

	return t.RenderResource("alicloud_ess_scaling_configuration", *e.Name, tf)
}

func (l *LaunchConfiguration) TerraformLink() *terraform.Literal {
	return terraform.LiteralProperty("alicloud_ess_scaling_configuration", *l.Name, "id")
}
