/*
Copyright 2019 The Kubernetes Authors.

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

package openstack

import (
	"fmt"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kops/pkg/cloudinstances"
	"k8s.io/kops/util/pkg/vfs"
)

func (c *openstackCloud) CreateInstance(opt servers.CreateOptsBuilder) (*servers.Server, error) {
	var server *servers.Server

	done, err := vfs.RetryWithBackoff(writeBackoff, func() (bool, error) {
		v, err := servers.Create(c.novaClient, opt).Extract()
		if err != nil {
			return false, fmt.Errorf("error creating server %v: %v", opt, err)
		}
		server = v
		return true, nil
	})
	if err != nil {
		return server, err
	} else if done {
		return server, nil
	} else {
		return server, wait.ErrWaitTimeout
	}
}

func (c *openstackCloud) DeleteInstance(i *cloudinstances.CloudInstanceGroupMember) error {
	return fmt.Errorf("openstackCloud::DeleteInstance not implemented")
}

func (c *openstackCloud) DeleteInstanceWithID(instanceID string) error {
	return servers.Delete(c.novaClient, instanceID).ExtractErr()
}

func (c *openstackCloud) GetInstance(id string) (*servers.Server, error) {
	var server *servers.Server

	done, err := vfs.RetryWithBackoff(readBackoff, func() (bool, error) {
		instance, err := servers.Get(c.novaClient, id).Extract()
		if err != nil {
			return false, err
		}
		server = instance
		return true, nil
	})
	if err != nil {
		return server, err
	} else if done {
		return server, nil
	} else {
		return server, wait.ErrWaitTimeout
	}
}

func (c *openstackCloud) ListInstances(opt servers.ListOptsBuilder) ([]servers.Server, error) {
	var instances []servers.Server

	done, err := vfs.RetryWithBackoff(readBackoff, func() (bool, error) {
		allPages, err := servers.List(c.novaClient, opt).AllPages()
		if err != nil {
			return false, fmt.Errorf("error listing servers %v: %v", opt, err)
		}

		ss, err := servers.ExtractServers(allPages)
		if err != nil {
			return false, fmt.Errorf("error extracting servers from pages: %v", err)
		}
		instances = ss
		return true, nil
	})
	if err != nil {
		return instances, err
	} else if done {
		return instances, nil
	} else {
		return instances, wait.ErrWaitTimeout
	}
}
