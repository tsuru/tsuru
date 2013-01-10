// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	collName string
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	s.collName = "juju_units_test"
	config.Set("git:host", "tsuruhost.com")
	config.Set("juju:units-collection", s.collName)
	err := handler.DryRun()
	c.Assert(err, IsNil)
	db.Session, err = db.Open("127.0.0.1:27017", "juju_provision_tests_s")
	c.Assert(err, IsNil)
}

func (s *S) TearDownSuite(c *C) {
	handler.Stop()
	handler.Wait()
	db.Session.Collection(s.collName).Database.DropDatabase()
	db.Session.Close()
}
