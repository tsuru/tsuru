// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/flaviamissi/go-elb/aws"
	"github.com/flaviamissi/go-elb/elb"
	"github.com/flaviamissi/go-elb/elb/elbtest"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/queue"
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"sort"
)

type ELBSuite struct {
	server      *elbtest.Server
	client      *elb.ELB
	conn        *db.Storage
	cName       string
	provisioner *testing.FakeProvisioner
}

var _ = gocheck.Suite(&ELBSuite{})

func (s *ELBSuite) SetUpSuite(c *gocheck.C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "juju_elb_tests")
	s.conn, err = db.Conn()
	s.server, err = elbtest.NewServer()
	c.Assert(err, gocheck.IsNil)
	config.Set("juju:elb-endpoint", s.server.URL())
	config.Set("juju:use-elb", true)
	region := aws.SAEast
	region.ELBEndpoint = s.server.URL()
	s.client = elb.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	c.Assert(err, gocheck.IsNil)
	s.cName = "juju_test_elbs"
	config.Set("juju:elb-collection", s.cName)
	config.Set("juju:elb-avail-zones", []interface{}{"my-zone-1a", "my-zone-1b"})
	config.Set("aws:access-key-id", "access")
	config.Set("aws:secret-access-key", "s3cr3t")
	config.Set("git:ro-host", "git.tsuru.io")
	config.Set("queue", "fake")
	config.Set("juju:units-collection", "juju_units_test_elb")
	s.provisioner = testing.NewFakeProvisioner()
	app.Provisioner = s.provisioner
}

func (s *ELBSuite) TearDownSuite(c *gocheck.C) {
	config.Unset("juju:use-elb")
	s.conn.Collection("juju_units_test_elb").Database.DropDatabase()
	s.server.Quit()
	queue.Preempt()
}

func (s *ELBSuite) TestGetCollection(c *gocheck.C) {
	manager := ELBManager{}
	conn, coll := manager.collection()
	defer conn.Close()
	other := s.conn.Collection(s.cName)
	c.Assert(coll.FullName, gocheck.Equals, other.FullName)
}

func (s *ELBSuite) TestGetELBClient(c *gocheck.C) {
	manager := ELBManager{}
	elb := manager.elb()
	c.Assert(elb.ELBEndpoint, gocheck.Equals, s.server.URL())
}

func (s *ELBSuite) TestCreateELB(c *gocheck.C) {
	app := testing.NewFakeApp("together", "gotthard", 1)
	manager := ELBManager{}
	manager.e = s.client
	err := manager.Create(app)
	c.Assert(err, gocheck.IsNil)
	defer s.client.DeleteLoadBalancer(app.GetName())
	conn, coll := manager.collection()
	defer conn.Close()
	defer coll.Remove(bson.M{"name": app.GetName()})
	resp, err := s.client.DescribeLoadBalancers("together")
	c.Assert(err, gocheck.IsNil)
	c.Assert(resp.LoadBalancerDescriptions, gocheck.HasLen, 1)
	c.Assert(resp.LoadBalancerDescriptions[0].ListenerDescriptions, gocheck.HasLen, 1)
	listener := resp.LoadBalancerDescriptions[0].ListenerDescriptions[0].Listener
	c.Assert(listener.InstancePort, gocheck.Equals, 80)
	c.Assert(listener.LoadBalancerPort, gocheck.Equals, 80)
	c.Assert(listener.InstanceProtocol, gocheck.Equals, "HTTP")
	c.Assert(listener.Protocol, gocheck.Equals, "HTTP")
	c.Assert(listener.SSLCertificateId, gocheck.Equals, "")
	dnsName := resp.LoadBalancerDescriptions[0].DNSName
	var lb loadBalancer
	err = s.conn.Collection(s.cName).Find(bson.M{"name": app.GetName()}).One(&lb)
	c.Assert(err, gocheck.IsNil)
	c.Assert(lb.DNSName, gocheck.Equals, dnsName)
}

func (s *ELBSuite) TestCreateELBUsingVPC(c *gocheck.C) {
	old, _ := config.Get("juju:elb-avail-zones")
	config.Unset("juju:elb-avail-zones")
	config.Set("juju:elb-use-vpc", true)
	config.Set("juju:elb-vpc-subnets", []string{"subnet-a4a3a2a1", "subnet-002200"})
	config.Set("juju:elb-vpc-secgroups", []string{"sg-0900"})
	defer func() {
		config.Set("juju:elb-avail-zones", old)
		config.Unset("juju:elb-use-vpc")
		config.Unset("juju:elb-vpc-subnets")
		config.Unset("juju:elb-vpc-secgroups")
	}()
	app := testing.NewFakeApp("relax", "who", 1)
	manager := ELBManager{}
	err := manager.Create(app)
	c.Assert(err, gocheck.IsNil)
	defer s.client.DeleteLoadBalancer(app.GetName())
	conn, coll := manager.collection()
	defer conn.Close()
	defer coll.Remove(bson.M{"name": app.GetName()})
	resp, err := s.client.DescribeLoadBalancers(app.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(resp.LoadBalancerDescriptions, gocheck.HasLen, 1)
	lbd := resp.LoadBalancerDescriptions[0]
	c.Assert(lbd.Subnets, gocheck.DeepEquals, []string{"subnet-a4a3a2a1", "subnet-002200"})
	c.Assert(lbd.SecurityGroups, gocheck.DeepEquals, []string{"sg-0900"})
	c.Assert(lbd.Scheme, gocheck.Equals, "internal")
	c.Assert(lbd.AvailZones, gocheck.HasLen, 0)
}

func (s *ELBSuite) TestDestroyELB(c *gocheck.C) {
	app := testing.NewFakeApp("blue", "who", 1)
	manager := ELBManager{}
	manager.e = s.client
	err := manager.Create(app)
	c.Assert(err, gocheck.IsNil)
	defer s.client.DeleteLoadBalancer(app.GetName()) // sanity
	conn, coll := manager.collection()
	defer conn.Close()
	defer coll.Remove(bson.M{"name": app.GetName()}) // sanity
	err = manager.Destroy(app)
	c.Assert(err, gocheck.IsNil)
	_, err = s.client.DescribeLoadBalancers(app.GetName())
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, `^.*\(LoadBalancerNotFound\)$`)
	n, err := coll.Find(bson.M{"name": app.GetName()}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 0)
}

func (s *ELBSuite) TestRegisterUnit(c *gocheck.C) {
	id1 := s.server.NewInstance()
	defer s.server.RemoveInstance(id1)
	id2 := s.server.NewInstance()
	defer s.server.RemoveInstance(id2)
	app := testing.NewFakeApp("fooled", "who", 1)
	manager := ELBManager{}
	manager.e = s.client
	err := manager.Create(app)
	c.Assert(err, gocheck.IsNil)
	defer manager.Destroy(app)
	err = manager.Register(app, provision.Unit{InstanceId: id1}, provision.Unit{InstanceId: id2})
	c.Assert(err, gocheck.IsNil)
	resp, err := s.client.DescribeLoadBalancers(app.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(resp.LoadBalancerDescriptions, gocheck.HasLen, 1)
	c.Assert(resp.LoadBalancerDescriptions[0].Instances, gocheck.HasLen, 2)
	instances := resp.LoadBalancerDescriptions[0].Instances
	ids := []string{instances[0].InstanceId, instances[1].InstanceId}
	sort.Strings(ids)
	expected := []string{id1, id2}
	sort.Strings(expected)
	c.Assert(ids, gocheck.DeepEquals, expected)
}

func (s *ELBSuite) TestDeregisterUnit(c *gocheck.C) {
	id1 := s.server.NewInstance()
	defer s.server.RemoveInstance(id1)
	id2 := s.server.NewInstance()
	defer s.server.RemoveInstance(id2)
	unit1 := provision.Unit{InstanceId: id1}
	unit2 := provision.Unit{InstanceId: id2}
	app := testing.NewFakeApp("dirty", "who", 1)
	manager := ELBManager{}
	manager.e = s.client
	err := manager.Create(app)
	c.Assert(err, gocheck.IsNil)
	defer manager.Destroy(app)
	err = manager.Register(app, unit1, unit2)
	c.Assert(err, gocheck.IsNil)
	err = manager.Deregister(app, unit1, unit2)
	c.Assert(err, gocheck.IsNil)
	resp, err := s.client.DescribeLoadBalancers(app.GetName())
	c.Assert(err, gocheck.IsNil)
	c.Assert(resp.LoadBalancerDescriptions, gocheck.HasLen, 1)
	c.Assert(resp.LoadBalancerDescriptions[0].Instances, gocheck.HasLen, 0)
}

func (s *ELBSuite) TestAddr(c *gocheck.C) {
	app := testing.NewFakeApp("enough", "who", 1)
	manager := ELBManager{}
	manager.e = s.client
	err := manager.Create(app)
	c.Assert(err, gocheck.IsNil)
	defer manager.Destroy(app)
	var lb loadBalancer
	conn, coll := manager.collection()
	defer conn.Close()
	err = coll.Find(bson.M{"name": app.GetName()}).One(&lb)
	c.Assert(err, gocheck.IsNil)
	addr, err := manager.Addr(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, lb.DNSName)
}

func (s *ELBSuite) TestAddrUnknownLoadBalancer(c *gocheck.C) {
	app := testing.NewFakeApp("five", "who", 1)
	manager := ELBManager{}
	addr, err := manager.Addr(app)
	c.Assert(addr, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "not found")
}

func (s *ELBSuite) TestELBInstanceHealer(c *gocheck.C) {
	lb := "elbtest"
	instance := s.server.NewInstance()
	defer s.server.RemoveInstance(instance)
	s.server.NewLoadBalancer(lb)
	defer s.server.RemoveLoadBalancer(lb)
	s.server.RegisterInstance(instance, lb)
	defer s.server.DeregisterInstance(instance, lb)
	a := app.App{
		Name:  "elbtest",
		Units: []app.Unit{{InstanceId: instance, State: "error", Name: "elbtest/0"}},
	}
	storage, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	err = storage.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer storage.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	state := elb.InstanceState{
		Description: "Instance has failed at least the UnhealthyThreshold number of health checks consecutively.",
		State:       "OutOfService",
		ReasonCode:  "Instance",
		InstanceId:  instance,
	}
	s.server.ChangeInstanceState(lb, state)
	healer := elbInstanceHealer{}
	err = healer.Heal()
	c.Assert(err, gocheck.IsNil)
	err = a.Get()
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.Units, gocheck.HasLen, 1)
	c.Assert(a.Units[0].InstanceId, gocheck.Not(gocheck.Equals), instance)
	queue.Preempt()
}

func (s *ELBSuite) TestELBInstanceHealerInstallingUnit(c *gocheck.C) {
	lb := "elbtest"
	instance := s.server.NewInstance()
	defer s.server.RemoveInstance(instance)
	s.server.NewLoadBalancer(lb)
	defer s.server.RemoveLoadBalancer(lb)
	s.server.RegisterInstance(instance, lb)
	defer s.server.DeregisterInstance(instance, lb)
	a := app.App{
		Name:  "elbtest",
		Units: []app.Unit{{InstanceId: instance, State: "installing", Name: "elbtest/0"}},
	}
	storage, err := db.Conn()
	c.Assert(err, gocheck.IsNil)
	err = storage.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer storage.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	state := elb.InstanceState{
		Description: "Instance has failed at least the UnhealthyThreshold number of health checks consecutively.",
		State:       "OutOfService",
		ReasonCode:  "Instance",
		InstanceId:  instance,
	}
	s.server.ChangeInstanceState(lb, state)
	healer := elbInstanceHealer{}
	err = healer.Heal()
	c.Assert(err, gocheck.IsNil)
	err = a.Get()
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.Units, gocheck.HasLen, 1)
	c.Assert(a.Units[0].InstanceId, gocheck.Equals, instance)
}
