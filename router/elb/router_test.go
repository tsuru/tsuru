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
	"github.com/globocom/tsuru/router"
	"github.com/globocom/tsuru/testing"
	"launchpad.net/gocheck"
	goTesting "testing"
)

func Test(t *goTesting.T) {
	gocheck.TestingT(t)
}

type S struct {
	server      *elbtest.Server
	client      *elb.ELB
	provisioner *testing.FakeProvisioner
}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	var err error
	s.server, err = elbtest.NewServer()
	c.Assert(err, gocheck.IsNil)
	config.Set("juju:elb-endpoint", s.server.URL())
	config.Set("juju:use-elb", true)
	region := aws.SAEast
	region.ELBEndpoint = s.server.URL()
	s.client = elb.New(aws.Auth{AccessKey: "some", SecretKey: "thing"}, region)
	c.Assert(err, gocheck.IsNil)
	config.Set("juju:elb-avail-zones", []interface{}{"my-zone-1a", "my-zone-1b"})
	config.Set("aws:access-key-id", "access")
	config.Set("aws:secret-access-key", "s3cr3t")
	s.provisioner = testing.NewFakeProvisioner()
	app.Provisioner = s.provisioner
}

func (s *S) TearDownSuite(c *gocheck.C) {
	s.server.Quit()
}

func (s *S) TestShouldBeRegistered(c *gocheck.C) {
	r, err := router.Get("elb")
	c.Assert(err, gocheck.IsNil)
	_, ok := r.(elbRouter)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestAddBackend(c *gocheck.C) {
	router := elbRouter{}
	err := router.AddBackend("tip")
	c.Assert(err, gocheck.IsNil)
	defer router.RemoveBackend("tip")
	resp, err := s.client.DescribeLoadBalancers("tip")
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

func (s *S) TestRemoveBackend(c *gocheck.C) {
	router := elbRouter{}
	err := router.AddBackend("tip")
	c.Assert(err, gocheck.IsNil)
	err = router.RemoveBackend("tip")
	c.Assert(err, gocheck.IsNil)
	_, err = s.client.DescribeLoadBalancers("tip")
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestAddRoute(c *gocheck.C) {
	instanceId := s.server.NewInstance()
	defer s.server.RemoveInstance(instanceId)
	router := elbRouter{}
	err := router.AddBackend("tip")
	c.Assert(err, gocheck.IsNil)
	defer router.RemoveBackend("tip")
	err = router.AddRoute("tip", instanceId)
	c.Assert(err, gocheck.IsNil)
	resp, err := s.client.DescribeLoadBalancers("tip")
	c.Assert(err, gocheck.IsNil)
	c.Assert(resp.LoadBalancerDescriptions[0].Instances, gocheck.HasLen, 1)
	instance := resp.LoadBalancerDescriptions[0].Instances[0]
	c.Assert(instance.InstanceId, gocheck.DeepEquals, instanceId)
}

func (s *S) TestRemoveRoute(c *gocheck.C) {
	instanceId := s.server.NewInstance()
	defer s.server.RemoveInstance(instanceId)
	router := elbRouter{}
	err := router.AddBackend("tip")
	c.Assert(err, gocheck.IsNil)
	defer router.RemoveBackend("tip")
	err = router.AddRoute("tip", instanceId)
	c.Assert(err, gocheck.IsNil)
	err = router.RemoveRoute("tip", instanceId)
	resp, err := s.client.DescribeLoadBalancers("tip")
	c.Assert(err, gocheck.IsNil)
	c.Assert(resp.LoadBalancerDescriptions[0].Instances, gocheck.HasLen, 0)
}
