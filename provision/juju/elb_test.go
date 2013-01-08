// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/flaviamissi/go-elb/aws"
	"github.com/flaviamissi/go-elb/elb"
	"github.com/flaviamissi/go-elb/elb/elbtest"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/queue"
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"sort"
)

type ELBSuite struct {
	server *elbtest.Server
	client *elb.ELB
	cName  string
}

var _ = Suite(&ELBSuite{})

func (s *ELBSuite) SetUpSuite(c *C) {
	var err error
	db.Session, err = db.Open("127.0.0.1:27017", "juju_tests")
	c.Assert(err, IsNil)
	s.server, err = elbtest.NewServer()
	c.Assert(err, IsNil)
	config.Set("juju:elb-endpoint", s.server.URL())
	config.Set("juju:use-elb", true)
	region := aws.SAEast
	region.ELBEndpoint = s.server.URL()
	s.client = elb.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	c.Assert(err, IsNil)
	s.cName = "juju_test_elbs"
	config.Set("juju:elb-collection", s.cName)
	config.Set("juju:elb-avail-zones", []interface{}{"my-zone-1a", "my-zone-1b"})
	config.Set("aws:access-key-id", "access")
	config.Set("aws:secret-access-key", "s3cr3t")
	config.Set("git:host", "git.tsuru.io")
	config.Set("queue-server", "127.0.0.1:11300")
	cleanQueue()
	err = handler.DryRun()
	c.Assert(err, IsNil)
}

func (s *ELBSuite) TearDownSuite(c *C) {
	config.Unset("juju:use-elb")
	db.Session.Close()
	s.server.Quit()
	handler.Stop()
	handler.Wait()
}

func (s *ELBSuite) TestGetCollection(c *C) {
	manager := ELBManager{}
	coll := manager.collection()
	other := db.Session.Collection(s.cName)
	c.Assert(coll, DeepEquals, other)
}

func (s *ELBSuite) TestGetELBClient(c *C) {
	manager := ELBManager{}
	elb := manager.elb()
	c.Assert(elb.ELBEndpoint, Equals, s.server.URL())
}

func (s *ELBSuite) TestCreateELB(c *C) {
	app := testing.NewFakeApp("together", "gotthard", 1)
	manager := ELBManager{}
	manager.e = s.client
	err := manager.Create(app)
	c.Assert(err, IsNil)
	defer s.client.DeleteLoadBalancer(app.GetName())
	defer manager.collection().Remove(bson.M{"name": app.GetName()})
	resp, err := s.client.DescribeLoadBalancers("together")
	c.Assert(err, IsNil)
	c.Assert(resp.LoadBalancerDescriptions, HasLen, 1)
	c.Assert(resp.LoadBalancerDescriptions[0].ListenerDescriptions, HasLen, 1)
	listener := resp.LoadBalancerDescriptions[0].ListenerDescriptions[0].Listener
	c.Assert(listener.InstancePort, Equals, 80)
	c.Assert(listener.LoadBalancerPort, Equals, 80)
	c.Assert(listener.InstanceProtocol, Equals, "HTTP")
	c.Assert(listener.Protocol, Equals, "HTTP")
	c.Assert(listener.SSLCertificateId, Equals, "")
	dnsName := resp.LoadBalancerDescriptions[0].DNSName
	var lb loadBalancer
	err = db.Session.Collection(s.cName).Find(bson.M{"name": app.GetName()}).One(&lb)
	c.Assert(err, IsNil)
	c.Assert(lb.DNSName, Equals, dnsName)
}

func (s *ELBSuite) TestCreateELBUsingVPC(c *C) {
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
	c.Assert(err, IsNil)
	defer s.client.DeleteLoadBalancer(app.GetName())
	defer manager.collection().Remove(bson.M{"name": app.GetName()})
	resp, err := s.client.DescribeLoadBalancers(app.GetName())
	c.Assert(err, IsNil)
	c.Assert(resp.LoadBalancerDescriptions, HasLen, 1)
	lbd := resp.LoadBalancerDescriptions[0]
	c.Assert(lbd.Subnets, DeepEquals, []string{"subnet-a4a3a2a1", "subnet-002200"})
	c.Assert(lbd.SecurityGroups, DeepEquals, []string{"sg-0900"})
	c.Assert(lbd.Scheme, Equals, "internal")
	c.Assert(lbd.AvailZones, HasLen, 0)
}

func (s *ELBSuite) TestDestroyELB(c *C) {
	app := testing.NewFakeApp("blue", "who", 1)
	manager := ELBManager{}
	manager.e = s.client
	err := manager.Create(app)
	c.Assert(err, IsNil)
	defer s.client.DeleteLoadBalancer(app.GetName())                 // sanity
	defer manager.collection().Remove(bson.M{"name": app.GetName()}) // sanity
	err = manager.Destroy(app)
	c.Assert(err, IsNil)
	_, err = s.client.DescribeLoadBalancers(app.GetName())
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, `^.*\(LoadBalancerNotFound\)$`)
	n, err := manager.collection().Find(bson.M{"name": app.GetName()}).Count()
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 0)
}

func (s *ELBSuite) TestRegisterUnit(c *C) {
	id1 := s.server.NewInstance()
	defer s.server.RemoveInstance(id1)
	id2 := s.server.NewInstance()
	defer s.server.RemoveInstance(id2)
	app := testing.NewFakeApp("fooled", "who", 1)
	manager := ELBManager{}
	manager.e = s.client
	err := manager.Create(app)
	c.Assert(err, IsNil)
	defer manager.Destroy(app)
	err = manager.Register(app, provision.Unit{InstanceId: id1}, provision.Unit{InstanceId: id2})
	c.Assert(err, IsNil)
	resp, err := s.client.DescribeLoadBalancers(app.GetName())
	c.Assert(err, IsNil)
	c.Assert(resp.LoadBalancerDescriptions, HasLen, 1)
	c.Assert(resp.LoadBalancerDescriptions[0].Instances, HasLen, 2)
	instances := resp.LoadBalancerDescriptions[0].Instances
	ids := []string{instances[0].InstanceId, instances[1].InstanceId}
	sort.Strings(ids)
	expected := []string{id1, id2}
	sort.Strings(expected)
	c.Assert(ids, DeepEquals, expected)
}

func (s *ELBSuite) TestDeregisterUnit(c *C) {
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
	c.Assert(err, IsNil)
	defer manager.Destroy(app)
	err = manager.Register(app, unit1, unit2)
	c.Assert(err, IsNil)
	err = manager.Deregister(app, unit1, unit2)
	c.Assert(err, IsNil)
	resp, err := s.client.DescribeLoadBalancers(app.GetName())
	c.Assert(err, IsNil)
	c.Assert(resp.LoadBalancerDescriptions, HasLen, 1)
	c.Assert(resp.LoadBalancerDescriptions[0].Instances, HasLen, 0)
}

func (s *ELBSuite) TestAddr(c *C) {
	app := testing.NewFakeApp("enough", "who", 1)
	manager := ELBManager{}
	manager.e = s.client
	err := manager.Create(app)
	c.Assert(err, IsNil)
	defer manager.Destroy(app)
	var lb loadBalancer
	err = manager.collection().Find(bson.M{"name": app.GetName()}).One(&lb)
	c.Assert(err, IsNil)
	addr, err := manager.Addr(app)
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, lb.DNSName)
}

func (s *ELBSuite) TestAddrUnknownLoadBalancer(c *C) {
	app := testing.NewFakeApp("five", "who", 1)
	manager := ELBManager{}
	addr, err := manager.Addr(app)
	c.Assert(addr, Equals, "")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "not found")
}

func cleanQueue() {
	var (
		err error
		msg *queue.Message
	)
	for err == nil {
		if msg, err = queue.Get(queueName, 1e6); err == nil {
			err = msg.Delete()
		}
	}
}
