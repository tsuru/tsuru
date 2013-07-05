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

type elbRouter struct{}

func getELBEndpoint() *elb.ELB {
	access, err := config.GetString("aws:access-key-id")
	if err != nil {
		log.Fatal(err)
	}
	secret, err := config.GetString("aws:secret-access-key")
	if err != nil {
		log.Fatal(err)
	}
	endpoint, err := config.GetString("juju:elb-endpoint")
	if err != nil {
		log.Fatal(err)
	}
	auth := aws.Auth{AccessKey: access, SecretKey: secret}
	region := aws.Region{ELBEndpoint: endpoint}
	return elb.New(auth, region)
}

func (elbRouter) AddBackend(name string) error {
	zone, err := config.GetList("juju:elb-avail-zones")
	if err != nil {
		return err
	}
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
		AvailZones: zone,
	}
	_, err = getELBEndpoint().CreateLoadBalancer(&options)
	return err
}

func (elbRouter) RemoveBackend(name string) error {
	_, err := getELBEndpoint().DeleteLoadBalancer(name)
	return err
}

func (elbRouter) AddRoute(name, address string) error {
	_, err := getELBEndpoint().RegisterInstancesWithLoadBalancer([]string{address}, name)
	return err
}

func (elbRouter) RemoveRoute(name, address string) error {
	_, err := getELBEndpoint().DeregisterInstancesFromLoadBalancer([]string{address}, name)
	return err
}

func (elbRouter) SetCName(cname, name string) error {
	return nil
}

func (elbRouter) UnsetCName(cname, name string) error {
	return nil
}

func (elbRouter) Addr(name string) (string, error) {
	return "", nil
}
