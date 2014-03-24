// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package collector

import (
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	ttesting "github.com/globocom/tsuru/testing"
	"github.com/tsuru/config"
	"launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct {
	conn        *db.Storage
	provisioner *ttesting.FakeProvisioner
}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_collector_test")
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
	s.provisioner = ttesting.NewFakeProvisioner()
	app.Provisioner = s.provisioner
	config.Set("queue-server", "127.0.0.1:0")
}

func (s *S) TearDownSuite(c *gocheck.C) {
	s.conn.Apps().Database.DropDatabase()
}

func (s *S) TearDownTest(c *gocheck.C) {
	_, err := s.conn.Apps().RemoveAll(nil)
	c.Assert(err, gocheck.IsNil)
	s.provisioner.Reset()
}
