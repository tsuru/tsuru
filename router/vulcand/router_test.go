// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vulcand

import (
	"net/http/httptest"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/router"

	"github.com/mailgun/scroll"
	vulcandAPI "github.com/mailgun/vulcand/api"
	"github.com/mailgun/vulcand/engine"
	"github.com/mailgun/vulcand/engine/memng"
	"github.com/mailgun/vulcand/plugin/registry"
	"github.com/mailgun/vulcand/supervisor"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type S struct {
	conn          *db.Storage
	engine        engine.Engine
	vulcandServer *httptest.Server
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("routers:vulcand:domain", "vulcand.example.com")
	config.Set("routers:vulcand:type", "vulcand")
	config.Set("routers:vulcand:api-url", "127.0.0.1:8181")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "router_vulcand_tests")

	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) SetUpTest(c *check.C) {
	dbtest.ClearAllCollections(s.conn.Collection("router_vulcand_tests").Database)

	s.engine = memng.New(registry.GetRegistry())
	scrollApp := scroll.NewApp()
	vulcandAPI.InitProxyController(s.engine, &supervisor.Supervisor{}, scrollApp)
	s.vulcandServer = httptest.NewServer(scrollApp.GetHandler())
	config.Set("routers:vulcand:api-url", s.vulcandServer.URL)
}

func (s *S) TearDownTest(c *check.C) {
	s.vulcandServer.Close()
}

func (s *S) TestShouldBeRegistered(c *check.C) {
	got, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)
	r, ok := got.(*vulcandRouter)
	c.Assert(ok, check.Equals, true)
	c.Assert(r.client.Addr, check.Equals, s.vulcandServer.URL)
}

func (s *S) TestShouldBeRegisteredAllowingPrefixes(c *check.C) {
	config.Set("routers:inst1:type", "vulcand")
	config.Set("routers:inst1:api-url", "http://localhost:1")
	config.Set("routers:inst2:type", "vulcand")
	config.Set("routers:inst2:api-url", "http://localhost:2")
	defer config.Unset("routers:inst1:type")
	defer config.Unset("routers:inst1:api-url")
	defer config.Unset("routers:inst2:type")
	defer config.Unset("routers:inst2:api-url")

	got1, err := router.Get("inst1")
	c.Assert(err, check.IsNil)
	got2, err := router.Get("inst2")
	c.Assert(err, check.IsNil)

	r1, ok := got1.(*vulcandRouter)
	c.Assert(ok, check.Equals, true)
	c.Assert(r1.client.Addr, check.Equals, "http://localhost:1")
	c.Assert(r1.prefix, check.Equals, "routers:inst1")
	r2, ok := got2.(*vulcandRouter)
	c.Assert(ok, check.Equals, true)
	c.Assert(r2.client.Addr, check.Equals, "http://localhost:2")
	c.Assert(r2.prefix, check.Equals, "routers:inst2")
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
