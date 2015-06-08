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
	c.Assert(r.domain, check.Equals, "vulcand.example.com")
}

func (s *S) TestShouldBeRegisteredAllowingPrefixes(c *check.C) {
	config.Set("routers:inst1:type", "vulcand")
	config.Set("routers:inst1:api-url", "http://localhost:1")
	config.Set("routers:inst1:domain", "inst1.example.com")
	config.Set("routers:inst2:type", "vulcand")
	config.Set("routers:inst2:api-url", "http://localhost:2")
	config.Set("routers:inst2:domain", "inst2.example.com")
	defer config.Unset("routers:inst1:type")
	defer config.Unset("routers:inst1:api-url")
	defer config.Unset("routers:inst1:domain")
	defer config.Unset("routers:inst2:type")
	defer config.Unset("routers:inst2:api-url")
	defer config.Unset("routers:inst2:domain")

	got1, err := router.Get("inst1")
	c.Assert(err, check.IsNil)
	got2, err := router.Get("inst2")
	c.Assert(err, check.IsNil)

	r1, ok := got1.(*vulcandRouter)
	c.Assert(ok, check.Equals, true)
	c.Assert(r1.client.Addr, check.Equals, "http://localhost:1")
	c.Assert(r1.domain, check.Equals, "inst1.example.com")
	c.Assert(r1.prefix, check.Equals, "routers:inst1")
	r2, ok := got2.(*vulcandRouter)
	c.Assert(ok, check.Equals, true)
	c.Assert(r2.client.Addr, check.Equals, "http://localhost:2")
	c.Assert(r2.domain, check.Equals, "inst2.example.com")
	c.Assert(r2.prefix, check.Equals, "routers:inst2")
}

func (s *S) TestAddBackend(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)

	err = vRouter.AddBackend("myapp")
	c.Assert(err, check.IsNil)

	backendKey := engine.BackendKey{Id: "tsuru_myapp"}
	frontendKey := engine.FrontendKey{Id: "tsuru_myapp.vulcand.example.com"}

	backend, err := s.engine.GetBackend(backendKey)
	c.Assert(err, check.IsNil)
	c.Assert(backend.Id, check.Equals, backendKey.String())
	c.Assert(backend.Type, check.Equals, "http")

	frontend, err := s.engine.GetFrontend(frontendKey)
	c.Assert(err, check.IsNil)
	c.Assert(frontend.Id, check.Equals, frontendKey.String())
	c.Assert(frontend.Route, check.Equals, `Host("myapp.vulcand.example.com") && PathRegexp("/")`)
	c.Assert(frontend.Type, check.Equals, "http")
	c.Assert(frontend.BackendId, check.Equals, backendKey.String())
}

func (s *S) TestRemoveBackend(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)

	err = vRouter.AddBackend("myapp")
	c.Assert(err, check.IsNil)
	backends, err := s.engine.GetBackends()
	c.Assert(err, check.IsNil)
	c.Assert(backends, check.HasLen, 1)
	frontends, err := s.engine.GetFrontends()
	c.Assert(err, check.IsNil)
	c.Assert(frontends, check.HasLen, 1)

	err = vRouter.RemoveBackend("myapp")
	c.Assert(err, check.IsNil)
	backends, err = s.engine.GetBackends()
	c.Assert(err, check.IsNil)
	c.Assert(backends, check.HasLen, 0)
	frontends, err = s.engine.GetFrontends()
	c.Assert(err, check.IsNil)
	c.Assert(frontends, check.HasLen, 0)
}

func (s *S) TestAddRoute(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)

	err = vRouter.AddBackend("myapp")
	c.Assert(err, check.IsNil)
	err = vRouter.AddRoute("myapp", "http://1.1.1.1:111")
	c.Assert(err, check.IsNil)
	err = vRouter.AddRoute("myapp", "http://2.2.2.2:222")
	c.Assert(err, check.IsNil)

	servers, err := s.engine.GetServers(engine.BackendKey{Id: "tsuru_myapp"})
	c.Assert(err, check.IsNil)
	c.Assert(servers, check.HasLen, 2)
	c.Assert(servers[0].URL, check.Equals, "http://1.1.1.1:111")
	c.Assert(servers[1].URL, check.Equals, "http://2.2.2.2:222")
}

func (s *S) TestRemoveRoute(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)

	err = vRouter.AddBackend("myapp")
	c.Assert(err, check.IsNil)
	err = vRouter.AddRoute("myapp", "http://1.1.1.1:111")
	c.Assert(err, check.IsNil)
	err = vRouter.AddRoute("myapp", "http://2.2.2.2:222")
	c.Assert(err, check.IsNil)

	servers, err := s.engine.GetServers(engine.BackendKey{Id: "tsuru_myapp"})
	c.Assert(err, check.IsNil)
	c.Assert(servers, check.HasLen, 2)

	err = vRouter.RemoveRoute("myapp", "http://1.1.1.1:111")
	c.Assert(err, check.IsNil)

	servers, err = s.engine.GetServers(engine.BackendKey{Id: "tsuru_myapp"})
	c.Assert(err, check.IsNil)
	c.Assert(servers, check.HasLen, 1)
	c.Assert(servers[0].URL, check.Equals, "http://2.2.2.2:222")
}

func (s *S) TestSetCName(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)

	err = vRouter.AddBackend("myapp")
	c.Assert(err, check.IsNil)
	err = vRouter.AddRoute("myapp", "http://1.1.1.1:111")
	c.Assert(err, check.IsNil)
	err = vRouter.AddRoute("myapp", "http://2.2.2.2:222")
	c.Assert(err, check.IsNil)
	err = vRouter.SetCName("myapp.cname.example.com", "myapp")
	c.Assert(err, check.IsNil)

	appFrontend, err := s.engine.GetFrontend(engine.FrontendKey{
		Id: "tsuru_myapp.vulcand.example.com",
	})
	c.Assert(err, check.IsNil)
	cnameFrontend, err := s.engine.GetFrontend(engine.FrontendKey{
		Id: "tsuru_myapp.cname.example.com",
	})
	c.Assert(err, check.IsNil)

	c.Assert(cnameFrontend.BackendId, check.DeepEquals, appFrontend.BackendId)
	c.Assert(cnameFrontend.Route, check.Equals, `Host("myapp.cname.example.com") && PathRegexp("/")`)
	c.Assert(cnameFrontend.Type, check.Equals, "http")
}

func (s *S) TestUnsetCName(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)

	err = vRouter.AddBackend("myapp")
	c.Assert(err, check.IsNil)
	err = vRouter.AddRoute("myapp", "http://1.1.1.1:111")
	c.Assert(err, check.IsNil)
	err = vRouter.AddRoute("myapp", "http://2.2.2.2:222")
	c.Assert(err, check.IsNil)
	err = vRouter.SetCName("myapp.cname.example.com", "myapp")
	c.Assert(err, check.IsNil)

	frontends, err := s.engine.GetFrontends()
	c.Assert(err, check.IsNil)
	c.Assert(frontends, check.HasLen, 2)

	vRouter.UnsetCName("myapp.cname.example.com", "myapp")
	frontends, err = s.engine.GetFrontends()
	c.Assert(err, check.IsNil)
	c.Assert(frontends, check.HasLen, 1)
	c.Assert(frontends[0].Id, check.Equals, "tsuru_myapp.vulcand.example.com")
}

func (s *S) TestAddr(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)

	err = vRouter.AddBackend("myapp")
	c.Assert(err, check.IsNil)

	addr, err := vRouter.Addr("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(addr, check.Equals, "myapp.vulcand.example.com")
}

func (s *S) TestAddrNotExist(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)

	frontends, err := s.engine.GetFrontends()
	c.Assert(err, check.IsNil)
	c.Assert(frontends, check.HasLen, 0)
	backends, err := s.engine.GetBackends()
	c.Assert(err, check.IsNil)
	c.Assert(backends, check.HasLen, 0)

	addr, err := vRouter.Addr("myapp")
	c.Assert(err, check.ErrorMatches, "Object not found")
	c.Assert(addr, check.Equals, "")
}

func (s *S) TestSwap(c *check.C) {
}

func (s *S) TestRoutes(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)

	err = vRouter.AddBackend("myapp")
	c.Assert(err, check.IsNil)
	err = vRouter.AddRoute("myapp", "http://1.1.1.1:111")
	c.Assert(err, check.IsNil)
	err = vRouter.AddRoute("myapp", "http://2.2.2.2:222")
	c.Assert(err, check.IsNil)

	routes, err := vRouter.Routes("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []string{"http://1.1.1.1:111", "http://2.2.2.2:222"})
}

func (s *S) TestStartupMessage(c *check.C) {
}
