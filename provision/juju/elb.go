// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/flaviamissi/go-elb/aws"
	"github.com/flaviamissi/go-elb/elb"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

// loadBalancer represents an ELB instance.
type loadBalancer struct {
	Name    string
	DNSName string
}

// ELBManager manages load balancers within Amazon Elastic Load Balancer.
//
// If juju:use-elb is true on tsuru.conf, this manager will be used for
// managing load balancers on tsuru.
//
// It uses db package and adds a new collection to tsuru's DB. The name of the
// collection is also defined in the configuration file (juju:elb-collection).
type ELBManager struct {
	e *elb.ELB
}

func (m *ELBManager) collection() *mgo.Collection {
	name, err := config.GetString("juju:elb-collection")
	if err != nil {
		log.Fatal("juju:elb-collection is undefined on config file.")
	}
	return db.Session.Collection(name)
}

func (m *ELBManager) elb() *elb.ELB {
	if m.e == nil {
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
		m.e = elb.New(auth, region)
	}
	return m.e
}

func (m *ELBManager) vpc() bool {
	vpc, _ := config.GetBool("juju:elb-use-vpc")
	return vpc
}

func (m *ELBManager) Create(app provision.Named) error {
	options := elb.CreateLoadBalancer{
		Name: app.GetName(),
		Listeners: []elb.Listener{
			{
				InstancePort:     80,
				InstanceProtocol: "HTTP",
				LoadBalancerPort: 80,
				Protocol:         "HTTP",
			},
		},
	}
	var err error
	if m.vpc() {
		options.Subnets, err = config.GetList("juju:elb-vpc-subnets")
		if err != nil {
			log.Fatal(err)
		}
		options.SecurityGroups, err = config.GetList("juju:elb-vpc-secgroups")
		if err != nil {
			log.Fatal(err)
		}
		options.Scheme = "internal"
	} else {
		options.AvailZones, err = config.GetList("juju:elb-avail-zones")
		if err != nil {
			log.Fatal(err)
		}
	}
	resp, err := m.elb().CreateLoadBalancer(&options)
	if err != nil {
		return err
	}
	lb := loadBalancer{Name: app.GetName(), DNSName: resp.DNSName}
	return m.collection().Insert(lb)
}

func (m *ELBManager) Destroy(app provision.Named) error {
	_, err := m.elb().DeleteLoadBalancer(app.GetName())
	if err != nil {
		return err
	}
	return m.collection().Remove(bson.M{"name": app.GetName()})
}

func (m *ELBManager) Register(app provision.Named, units ...provision.Unit) error {
	ids := make([]string, len(units))
	for i, u := range units {
		ids[i] = u.InstanceId
	}
	_, err := m.elb().RegisterInstancesWithLoadBalancer(ids, app.GetName())
	return err
}

func (m *ELBManager) Deregister(app provision.Named, units ...provision.Unit) error {
	ids := make([]string, len(units))
	for i, u := range units {
		ids[i] = u.InstanceId
	}
	_, err := m.elb().DeregisterInstancesFromLoadBalancer(ids, app.GetName())
	return err
}

func (m *ELBManager) Addr(app provision.Named) (string, error) {
	var lb loadBalancer
	err := m.collection().Find(bson.M{"name": app.GetName()}).One(&lb)
	return lb.DNSName, err
}
