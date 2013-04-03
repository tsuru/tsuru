// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
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
	s.collName = "collName"
	config.Set("docker:binary", "docker")
	config.Set("docker:collection", s.collName)
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "juju_provision_tests_s")
	config.Set("docker:authorized-key-path", "somepath")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TearDownSuite(c *gocheck.C) {
	s.conn.Collection(s.collName).Database.DropDatabase()
}
