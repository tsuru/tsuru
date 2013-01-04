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
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
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
}

func (s *ELBSuite) TearDownSuite(c *C) {
	db.Session.Close()
	s.server.Quit()
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
	c.Assert(manager.zones, DeepEquals, []string{"my-zone-1a", "my-zone-1b"})
}

func (s *ELBSuite) TestCreateELB(c *C) {
	app := testing.NewFakeApp("together", "gotthard", 1)
	defer s.client.DeleteLoadBalancer("together")
	manager := ELBManager{}
	manager.e = s.client
	manager.zones = []string{"my-zone-1a"}
	err := manager.Create(app)
	c.Assert(err, IsNil)
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
	var lb LoadBalancer
	err = db.Session.Collection(s.cName).Find(bson.M{"name": app.GetName()}).One(&lb)
	c.Assert(err, IsNil)
	c.Assert(lb.DNSName, Equals, dnsName)
}
