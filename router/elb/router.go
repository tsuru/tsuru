// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package elb

import (
	"github.com/flaviamissi/go-elb/aws"
	"github.com/flaviamissi/go-elb/elb"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/router"
)

func init() {
	router.Register("elb", elbRouter{})
}

type elbRouter struct {
	e *elb.ELB
}

func (r *elbRouter) elb() *elb.ELB {
	if r.e == nil {
		r.e = getELBEndpoint()
	}
	return r.e
}

func getELBEndpoint() *elb.ELB {
	access, err := config.GetString("aws:access-key-id")
	if err != nil {
		log.Error(err.Error())
	}
	secret, err := config.GetString("aws:secret-access-key")
	if err != nil {
		log.Error(err.Error())
	}
	endpoint, err := config.GetString("juju:elb-endpoint")
	if err != nil {
		log.Error(err.Error())
	}
	auth := aws.Auth{AccessKey: access, SecretKey: secret}
	region := aws.Region{ELBEndpoint: endpoint}
	return elb.New(auth, region)
}

func (r elbRouter) AddBackend(name string) error {
	var err error
	options := elb.CreateLoadBalancer{
		Name: name,
		Listeners: []elb.Listener{
			{
				InstancePort:     80,
				InstanceProtocol: "HTTP",
				LoadBalancerPort: 80,
				Protocol:         "HTTP",
			},
		},
	}
	vpc, _ := config.GetBool("juju:elb-use-vpc")
	if vpc {
		options.Subnets, err = config.GetList("juju:elb-vpc-subnets")
		if err != nil {
			return err
		}
		options.SecurityGroups, err = config.GetList("juju:elb-vpc-secgroups")
		if err != nil {
			return err
		}
		options.Scheme = "internal"
	} else {
		options.AvailZones, err = config.GetList("juju:elb-avail-zones")
		if err != nil {
			return err
		}
	}
	_, err = r.elb().CreateLoadBalancer(&options)
	return router.Store(name, name)
}

func (r elbRouter) RemoveBackend(name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	_, err = r.elb().DeleteLoadBalancer(backendName)
	if err != nil {
		return err
	}
	return router.Remove(backendName)
}

func (r elbRouter) AddRoute(name, address string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	_, err = r.elb().RegisterInstancesWithLoadBalancer([]string{address}, backendName)
	return err
}

func (r elbRouter) RemoveRoute(name, address string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	_, err = r.elb().DeregisterInstancesFromLoadBalancer([]string{address}, backendName)
	return err
}

func (elbRouter) SetCName(cname, name string) error {
	return nil
}

func (elbRouter) UnsetCName(cname, name string) error {
	return nil
}

func (r elbRouter) Routes(name string) ([]string, error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return nil, err
	}
	var routes []string
	resp, err := r.elb().DescribeLoadBalancers(backendName)
	if err != nil {
		return nil, err
	}
	for _, instance := range resp.LoadBalancerDescriptions[0].Instances {
		routes = append(routes, instance.InstanceId)
	}
	return routes, nil
}

func (r elbRouter) Addr(name string) (string, error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return "", err
	}
	resp, err := r.elb().DescribeLoadBalancers(backendName)
	if err != nil {
		return "", err
	}
	return resp.LoadBalancerDescriptions[0].DNSName, nil
}

func (r elbRouter) Swap(backend1, backend2 string) error {
	return router.Swap(r, backend1, backend2)
}
