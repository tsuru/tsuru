// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/config"
	"launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct {
	collName string
	conn     *db.Storage
}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	var err error
	s.collName = "juju_units_test"
	config.Set("git:ro-host", "tsuruhost.com")
	config.Set("juju:units-collection", s.collName)
	config.Set("juju:bootstrap-collection", "juju_bootstrap_test")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "juju_provision_tests_s")
	config.Set("queue", "fake")
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TearDownTest(c *gocheck.C) {
	s.conn.Collection("juju_bootstrap_test").Remove(nil)
}

func (s *S) TearDownSuite(c *gocheck.C) {
	queue.Preempt()
	s.conn.Collection(s.collName).Database.DropDatabase()
	s.conn.Collection("juju_bootstrap_test").Database.DropDatabase()
}
