/*
Copyright 2017 The Kubernetes Authors.

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
	"github.com/gophercloud/gophercloud/openstack/loadbalancer/v2/loadbalancers"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/subnets"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/openstack"
)

//go:generate fitask -type=LB
type LB struct {
	ID        *string
	Name      *string
	Subnet    *string
	VipSubnet *string
	Lifecycle *fi.Lifecycle
	PortID    *string
}

// GetDependencies returns the dependencies of the Instance task
func (e *LB) GetDependencies(tasks map[string]fi.Task) []fi.Task {
	var deps []fi.Task
	for _, task := range tasks {
		if _, ok := task.(*Subnet); ok {
			deps = append(deps, task)
		}
		if _, ok := task.(*ServerGroup); ok {
			deps = append(deps, task)
		}
		if _, ok := task.(*Instance); ok {
			deps = append(deps, task)
		}
	}
	return deps
}

var _ fi.CompareWithID = &LB{}

func (s *LB) CompareWithID() *string {
	return s.ID
}

func NewLBTaskFromCloud(cloud openstack.OpenstackCloud, lifecycle *fi.Lifecycle, lb *loadbalancers.LoadBalancer, find *LB) (*LB, error) {
	osCloud := cloud.(openstack.OpenstackCloud)
	sub, err := subnets.Get(osCloud.NetworkingClient(), lb.VipSubnetID).Extract()
	if err != nil {
		return nil, err
	}

	actual := &LB{
		ID:        fi.String(lb.ID),
		Name:      fi.String(lb.Name),
		Lifecycle: lifecycle,
		PortID:    fi.String(lb.VipPortID),
		Subnet:    fi.String(sub.Name),
	}

	if find != nil {
		find.ID = actual.ID
		find.PortID = fi.String(lb.VipPortID)
	}
	return actual, nil
}

func (s *LB) Find(context *fi.Context) (*LB, error) {
	if s.Name == nil {
		return nil, nil
	}

	cloud := context.Cloud.(openstack.OpenstackCloud)
	lbPage, err := loadbalancers.List(cloud.LoadBalancerClient(), loadbalancers.ListOpts{
		Name: fi.StringValue(s.Name),
	}).AllPages()
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve loadbalancers for name %s: %v", fi.StringValue(s.Name), err)
	}
	lbs, err := loadbalancers.ExtractLoadBalancers(lbPage)
	if err != nil {
		return nil, fmt.Errorf("Failed to extract loadbalancers : %v", err)
	}
	if len(lbs) == 0 {
		return nil, nil
	}
	if len(lbs) > 1 {
		return nil, fmt.Errorf("Multiple load balancers for name %s", fi.StringValue(s.Name))
	}

	return NewLBTaskFromCloud(cloud, s.Lifecycle, &lbs[0], s)
}

func (s *LB) Run(context *fi.Context) error {
	return fi.DefaultDeltaRunMethod(s, context)
}

func (_ *LB) CheckChanges(a, e, changes *LB) error {
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

func (_ *LB) RenderOpenstack(t *openstack.OpenstackAPITarget, a, e, changes *LB) error {
	if a == nil {
		glog.V(2).Infof("Creating LB with Name: %q", fi.StringValue(e.Name))

		subnets, err := t.Cloud.ListSubnets(subnets.ListOpts{
			Name: fi.StringValue(e.Subnet),
		})
		if err != nil {
			return fmt.Errorf("Failed to retrieve subnet `%s` in loadbalancer creation: %v", fi.StringValue(e.Subnet), err)
		}
		if len(subnets) != 1 {
			return fmt.Errorf("Unexpected desired subnets for `%s`.  Expected 1, got %d", fi.StringValue(e.Subnet), len(subnets))
		}

		lbopts := loadbalancers.CreateOpts{
			Name:        fi.StringValue(e.Name),
			VipSubnetID: subnets[0].ID,
		}
		lb, err := t.Cloud.CreateLB(lbopts)
		if err != nil {
			return fmt.Errorf("error creating LB: %v", err)
		}
		e.ID = fi.String(lb.ID)
		e.PortID = fi.String(lb.VipPortID)
		e.VipSubnet = fi.String(lb.VipSubnetID)

		return nil
	}

	glog.V(2).Infof("Openstack task LB::RenderOpenstack did nothing")
	return nil
}
