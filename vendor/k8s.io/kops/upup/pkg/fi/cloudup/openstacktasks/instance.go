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

package openstacktasks

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/keypairs"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/schedulerhints"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/openstack"
)

//go:generate fitask -type=Instance
type Instance struct {
	ID          *string
	Name        *string
	Port        *Port
	Region      *string
	Flavor      *string
	Image       *string
	SSHKey      *string
	ServerGroup *ServerGroup
	Tags        []string
	Role        *string
	UserData    *string
	Metadata    map[string]string

	Lifecycle *fi.Lifecycle
}

// GetDependencies returns the dependencies of the Instance task
func (e *Instance) GetDependencies(tasks map[string]fi.Task) []fi.Task {
	var deps []fi.Task
	for _, task := range tasks {
		if _, ok := task.(*ServerGroup); ok {
			deps = append(deps, task)
		}
		if _, ok := task.(*Port); ok {
			deps = append(deps, task)
		}
	}
	return deps
}

var _ fi.CompareWithID = &Instance{}

func (e *Instance) WaitForStatusActive(t *openstack.OpenstackAPITarget) error {
	return servers.WaitForStatus(t.Cloud.ComputeClient(), *e.ID, "ACTIVE", 120)
}

func (e *Instance) CompareWithID() *string {
	return e.ID
}

func (e *Instance) Find(c *fi.Context) (*Instance, error) {
	if e == nil || e.Name == nil {
		return nil, nil
	}
	serverPage, err := servers.List(c.Cloud.(openstack.OpenstackCloud).ComputeClient(), servers.ListOpts{
		Name: fi.StringValue(e.Name),
	}).AllPages()
	if err != nil {
		return nil, fmt.Errorf("error finding server with name %s: %v", fi.StringValue(e.Name), err)
	}
	serverList, err := servers.ExtractServers(serverPage)
	if err != nil {
		return nil, fmt.Errorf("error extracting server page: %v", err)
	}
	if len(serverList) == 0 {
		return nil, nil
	}
	if len(serverList) > 1 {
		return nil, fmt.Errorf("Multiple servers found with name %s", fi.StringValue(e.Name))
	}

	server := serverList[0]
	actual := &Instance{
		ID:        fi.String(server.ID),
		Name:      fi.String(server.Name),
		SSHKey:    fi.String(server.KeyName),
		Lifecycle: e.Lifecycle,
	}
	e.ID = actual.ID

	return actual, nil
}

func (e *Instance) Run(c *fi.Context) error {
	return fi.DefaultDeltaRunMethod(e, c)
}

func (_ *Instance) CheckChanges(a, e, changes *Instance) error {
	if a == nil {
		if e.Name == nil {
			return fi.RequiredField("Name")
		}
	} else {
		if changes.ID != nil {
			return fi.CannotChangeField("ID")
		}
		if changes.Name != nil {
			return fi.CannotChangeField("Name")
		}
	}
	return nil
}

func (_ *Instance) ShouldCreate(a, e, changes *Instance) (bool, error) {
	return a == nil, nil
}

func (_ *Instance) RenderOpenstack(t *openstack.OpenstackAPITarget, a, e, changes *Instance) error {
	if a == nil {
		glog.V(2).Infof("Creating Instance with name: %q", fi.StringValue(e.Name))

		opt := servers.CreateOpts{
			Name:       fi.StringValue(e.Name),
			ImageName:  fi.StringValue(e.Image),
			FlavorName: fi.StringValue(e.Flavor),
			Networks: []servers.Network{
				{
					Port: fi.StringValue(e.Port.ID),
				},
			},
			Metadata:      e.Metadata,
			ServiceClient: t.Cloud.ComputeClient(),
		}
		if e.UserData != nil {
			opt.UserData = []byte(*e.UserData)
		}
		keyext := keypairs.CreateOptsExt{
			CreateOptsBuilder: opt,
			KeyName:           openstackKeyPairName(fi.StringValue(e.SSHKey)),
		}

		sgext := schedulerhints.CreateOptsExt{
			CreateOptsBuilder: keyext,
			SchedulerHints: &schedulerhints.SchedulerHints{
				Group: *e.ServerGroup.ID,
			},
		}
		v, err := t.Cloud.CreateInstance(sgext)
		if err != nil {
			return fmt.Errorf("Error creating instance: %v", err)
		}
		e.ID = fi.String(v.ID)
		e.ServerGroup.Members = append(e.ServerGroup.Members, fi.StringValue(e.ID))

		glog.V(2).Infof("Creating a new Openstack instance, id=%s", v.ID)

		return nil
	}

	glog.V(2).Infof("Openstack task Instance::RenderOpenstack did nothing")
	return nil
}
