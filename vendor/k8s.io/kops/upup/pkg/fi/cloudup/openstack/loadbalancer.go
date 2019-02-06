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
	"github.com/gophercloud/gophercloud/openstack/loadbalancer/v2/listeners"
	"github.com/gophercloud/gophercloud/openstack/loadbalancer/v2/loadbalancers"
	v2pools "github.com/gophercloud/gophercloud/openstack/loadbalancer/v2/pools"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kops/util/pkg/vfs"
)

func (c *openstackCloud) DeletePool(poolID string) error {
	done, err := vfs.RetryWithBackoff(writeBackoff, func() (bool, error) {
		err := v2pools.Delete(c.lbClient, poolID).ExtractErr()
		if err != nil && !isNotFound(err) {
			return false, fmt.Errorf("error deleting pool: %v", err)
		}
		return true, nil
	})
	if err != nil {
		return err
	} else if done {
		return nil
	} else {
		return wait.ErrWaitTimeout
	}
}

func (c *openstackCloud) DeleteListener(listenerID string) error {
	done, err := vfs.RetryWithBackoff(writeBackoff, func() (bool, error) {
		err := listeners.Delete(c.lbClient, listenerID).ExtractErr()
		if err != nil && !isNotFound(err) {
			return false, fmt.Errorf("error deleting listener: %v", err)
		}
		return true, nil
	})
	if err != nil {
		return err
	} else if done {
		return nil
	} else {
		return wait.ErrWaitTimeout
	}
}

func (c *openstackCloud) DeleteLB(lbID string, opts loadbalancers.DeleteOpts) error {
	done, err := vfs.RetryWithBackoff(writeBackoff, func() (bool, error) {
		err := loadbalancers.Delete(c.lbClient, lbID, opts).ExtractErr()
		if err != nil && !isNotFound(err) {
			return false, fmt.Errorf("error deleting loadbalancer: %v", err)
		}
		return true, nil
	})
	if err != nil {
		return err
	} else if done {
		return nil
	} else {
		return wait.ErrWaitTimeout
	}
}

func (c *openstackCloud) CreateLB(opt loadbalancers.CreateOptsBuilder) (*loadbalancers.LoadBalancer, error) {
	var i *loadbalancers.LoadBalancer

	done, err := vfs.RetryWithBackoff(writeBackoff, func() (bool, error) {
		v, err := loadbalancers.Create(c.lbClient, opt).Extract()
		if err != nil {
			return false, fmt.Errorf("error creating loadbalancer: %v", err)
		}
		i = v
		return true, nil
	})
	if err != nil {
		return i, err
	} else if done {
		return i, nil
	} else {
		return i, wait.ErrWaitTimeout
	}
}

func (c *openstackCloud) GetLB(loadbalancerID string) (lb *loadbalancers.LoadBalancer, err error) {

	done, err := vfs.RetryWithBackoff(readBackoff, func() (bool, error) {
		lb, err = loadbalancers.Get(c.neutronClient, loadbalancerID).Extract()
		if err != nil {
			return false, err
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return lb, err
	}
	return lb, nil
}

// ListLBs will list load balancers
func (c *openstackCloud) ListLBs(opt loadbalancers.ListOptsBuilder) (lbs []loadbalancers.LoadBalancer, err error) {

	done, err := vfs.RetryWithBackoff(readBackoff, func() (bool, error) {
		allPages, err := loadbalancers.List(c.lbClient, opt).AllPages()
		if err != nil {
			return false, fmt.Errorf("failed to list loadbalancers: %s", err)
		}
		lbs, err = loadbalancers.ExtractLoadBalancers(allPages)
		if err != nil {
			return false, fmt.Errorf("failed to extract loadbalancer pages: %s", err)
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return lbs, err
	}
	return lbs, nil
}

func (c *openstackCloud) GetPool(poolID string, memberID string) (member *v2pools.Member, err error) {
	done, err := vfs.RetryWithBackoff(readBackoff, func() (bool, error) {
		member, err = v2pools.GetMember(c.neutronClient, poolID, memberID).Extract()
		if err != nil {
			return false, err
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return member, err
	}
	return member, nil
}

func (c *openstackCloud) AssociateToPool(server *servers.Server, poolID string, opts v2pools.CreateMemberOpts) (association *v2pools.Member, err error) {

	done, err := vfs.RetryWithBackoff(writeBackoff, func() (bool, error) {
		association, err = v2pools.GetMember(c.NetworkingClient(), poolID, server.ID).Extract()
		if err != nil || association == nil {
			// Pool association does not exist.  Create it
			association, err = v2pools.CreateMember(c.NetworkingClient(), poolID, opts).Extract()
			if err != nil {
				return false, fmt.Errorf("Failed to create pool association: %v", err)
			}
			return true, nil
		}
		//NOOP
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return association, err
	}
	return association, nil
}

func (c *openstackCloud) CreatePool(opts v2pools.CreateOpts) (pool *v2pools.Pool, err error) {
	done, err := vfs.RetryWithBackoff(writeBackoff, func() (bool, error) {
		pool, err = v2pools.Create(c.LoadBalancerClient(), opts).Extract()
		if err != nil {
			return false, fmt.Errorf("Failed to create pool: %v", err)
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return pool, err
	}
	return pool, nil
}

func (c *openstackCloud) ListPools(opts v2pools.ListOpts) (poolList []v2pools.Pool, err error) {
	done, err := vfs.RetryWithBackoff(readBackoff, func() (bool, error) {
		poolPage, err := v2pools.List(c.LoadBalancerClient(), opts).AllPages()
		if err != nil {
			return false, fmt.Errorf("Failed to list pools: %v", err)
		}
		poolList, err = v2pools.ExtractPools(poolPage)
		if err != nil {
			return false, fmt.Errorf("Failed to extract pools: %v", err)
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return poolList, err
	}
	return poolList, nil
}

func (c *openstackCloud) ListListeners(opts listeners.ListOpts) (listenerList []listeners.Listener, err error) {
	done, err := vfs.RetryWithBackoff(readBackoff, func() (bool, error) {
		listenerPage, err := listeners.List(c.LoadBalancerClient(), opts).AllPages()
		if err != nil {
			return false, fmt.Errorf("Failed to list listeners: %v", err)
		}
		listenerList, err = listeners.ExtractListeners(listenerPage)
		if err != nil {
			return false, fmt.Errorf("Failed to extract listeners: %v", err)
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return listenerList, err
	}
	return listenerList, nil
}

func (c *openstackCloud) CreateListener(opts listeners.CreateOpts) (listener *listeners.Listener, err error) {
	done, err := vfs.RetryWithBackoff(readBackoff, func() (bool, error) {
		listener, err = listeners.Create(c.LoadBalancerClient(), opts).Extract()
		if err != nil {
			return false, fmt.Errorf("Unabled to create listener: %v", err)
		}
		return true, nil
	})
	if !done {
		if err == nil {
			err = wait.ErrWaitTimeout
		}
		return listener, err
	}
	return listener, nil
}
