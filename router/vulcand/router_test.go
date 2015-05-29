// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vulcand

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/router"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type S struct {
	conn *db.Storage
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("routers:vulcand:domain", "vulcand.example.com")
	config.Set("routers:vulcand:type", "vulcand")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "router_vulcand_tests")

	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) SetUpTest(c *check.C) {
	dbtest.ClearAllCollections(s.conn.Collection("router_vulcand_tests").Database)
}

func (s *S) TestShouldBeRegistered(c *check.C) {
	got, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)
	_, ok := got.(*vulcandRouter)
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestShouldBeRegisteredAllowingPrefixes(c *check.C) {
}

func (s *S) TestAddBackend(c *check.C) {
}

func (s *S) TestRemoveBackend(c *check.C) {
}

func (s *S) TestAddRoute(c *check.C) {
}

func (s *S) TestRemoveRoute(c *check.C) {
}

func (s *S) TestSetCName(c *check.C) {
}

func (s *S) TestUnsetCName(c *check.C) {
}

func (s *S) TestAddr(c *check.C) {
}

func (s *S) TestSwap(c *check.C) {
}

func (s *S) TestRoutes(c *check.C) {
}

func (s *S) TestStartupMessage(c *check.C) {
}
