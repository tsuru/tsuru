// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/flaviamissi/go-elb/aws"
	"github.com/flaviamissi/go-elb/elb"
	"github.com/flaviamissi/go-elb/elb/elbtest"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/testing"
	"launchpad.net/gocheck"
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
