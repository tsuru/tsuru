// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package elb

import (
	"github.com/flaviamissi/go-elb/aws"
	"github.com/flaviamissi/go-elb/elb"
	"github.com/flaviamissi/go-elb/elb/elbtest"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/queue"
	"github.com/globocom/tsuru/router"
	"github.com/globocom/tsuru/testing"
	"launchpad.net/gocheck"
	goTesting "testing"
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

func Test(t *goTesting.T) {
	gocheck.TestingT(t)
}

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) TestShouldBeRegistered(c *gocheck.C) {
	r, err := router.Get("elb")
	c.Assert(err, gocheck.IsNil)
	_, ok := r.(elbRouter)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestAddBackend(c *gocheck.C) {
	server, err := elbtest.NewServer()
	c.Assert(err, gocheck.IsNil)
	config.Set("juju:elb-endpoint", server.URL())
	config.Set("juju:elb-avail-zones", []interface{}{"my-zone-1a", "my-zone-1b"})
	region := aws.SAEast
	region.ELBEndpoint = server.URL()
	router := elbRouter{}
	err = router.AddBackend("tip")
	c.Assert(err, gocheck.IsNil)
	client := elb.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	resp, err := client.DescribeLoadBalancers("tip")
	c.Assert(err, gocheck.IsNil)
	c.Assert(resp.LoadBalancerDescriptions, gocheck.HasLen, 1)
	c.Assert(resp.LoadBalancerDescriptions[0].ListenerDescriptions, gocheck.HasLen, 1)
	listener := resp.LoadBalancerDescriptions[0].ListenerDescriptions[0].Listener
	c.Assert(listener.InstancePort, gocheck.Equals, 80)
	c.Assert(listener.LoadBalancerPort, gocheck.Equals, 80)
	c.Assert(listener.InstanceProtocol, gocheck.Equals, "HTTP")
	c.Assert(listener.Protocol, gocheck.Equals, "HTTP")
	c.Assert(listener.SSLCertificateId, gocheck.Equals, "")
}
