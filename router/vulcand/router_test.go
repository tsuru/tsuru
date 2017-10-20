// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !windows

package vulcand

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/tsurutest"
	"github.com/vulcand/vulcand/api"
	"github.com/vulcand/vulcand/engine"
	"github.com/vulcand/vulcand/engine/memng"
	"github.com/vulcand/vulcand/plugin/registry"
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

func init() {
	base := &S{}
	suite := &routertest.RouterSuite{
		SetUpSuiteFunc:   base.SetUpSuite,
		TearDownTestFunc: base.TearDownTest,
	}
	suite.SetUpTestFunc = func(c *check.C) {
		config.Set("database:name", "router_generic_vulcand_tests")
		base.SetUpTest(c)
		r, err := router.Get("vulcand")
		c.Assert(err, check.IsNil)
		suite.Router = r
	}
	check.Suite(suite)
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("routers:vulcand:domain", "vulcand.example.com")
	config.Set("routers:vulcand:type", "vulcand")
	config.Set("routers:vulcand:api-url", "127.0.0.1:8181")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "router_vulcand_tests")
}

func (s *S) SetUpTest(c *check.C) {
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	dbtest.ClearAllCollections(s.conn.Collection("router_vulcand_tests").Database)
	s.engine = memng.New(registry.GetRegistry())
	router := mux.NewRouter()
	api.InitProxyController(s.engine, nil, router)
	s.vulcandServer = httptest.NewServer(router)
	config.Set("routers:vulcand:api-url", s.vulcandServer.URL)
}

func (s *S) TearDownTest(c *check.C) {
	s.vulcandServer.Close()
	s.conn.Close()
}

func (s *S) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
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
	err = vRouter.AddBackend(routertest.FakeApp{Name: "myapp"})
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
	c.Assert(frontend.Route, check.Equals, `Host("myapp.vulcand.example.com")`)
	c.Assert(frontend.Type, check.Equals, "http")
	c.Assert(frontend.BackendId, check.Equals, backendKey.String())
}

func (s *S) TestAddBackendDuplicate(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)
	err = vRouter.AddBackend(routertest.FakeApp{Name: "myapp"})
	c.Assert(err, check.IsNil)
	err = vRouter.AddBackend(routertest.FakeApp{Name: "myapp"})
	c.Assert(err, check.ErrorMatches, router.ErrBackendExists.Error())
}

func (s *S) TestAddBackendRollbackOnError(c *check.C) {
	s.vulcandServer.Close()
	muxRouter := mux.NewRouter()
	var postRequestCount int
	conditionalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			postRequestCount++
			if postRequestCount > 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		muxRouter.ServeHTTP(w, r)
	})
	api.InitProxyController(s.engine, nil, muxRouter)
	s.vulcandServer = httptest.NewServer(conditionalHandler)
	config.Set("routers:vulcand:api-url", s.vulcandServer.URL)
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)
	err = vRouter.AddBackend(routertest.FakeApp{Name: "myapp"})
	c.Assert(err, check.NotNil)
	backends, err := s.engine.GetBackends()
	c.Assert(err, check.IsNil)
	c.Assert(backends, check.HasLen, 0)
	frontends, err := s.engine.GetFrontends()
	c.Assert(err, check.IsNil)
	c.Assert(frontends, check.HasLen, 0)
}

func (s *S) TestRemoveBackend(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)
	err = vRouter.AddBackend(routertest.FakeApp{Name: "myapp"})
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

func (s *S) TestRemoveBackendNotExist(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)
	frontends, err := s.engine.GetFrontends()
	c.Assert(err, check.IsNil)
	c.Assert(frontends, check.HasLen, 0)
	backends, err := s.engine.GetBackends()
	c.Assert(err, check.IsNil)
	c.Assert(backends, check.HasLen, 0)
	err = vRouter.RemoveBackend("myapp")
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
}

func (s *S) TestAddRoutes(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)
	err = vRouter.AddBackend(routertest.FakeApp{Name: "myapp"})
	c.Assert(err, check.IsNil)
	u1, _ := url.Parse("http://1.1.1.1:111")
	u2, _ := url.Parse("http://2.2.2.2:222")
	err = vRouter.AddRoutes("myapp", []*url.URL{u1})
	c.Assert(err, check.IsNil)
	err = vRouter.AddRoutes("myapp", []*url.URL{u2})
	c.Assert(err, check.IsNil)
	servers, err := s.engine.GetServers(engine.BackendKey{Id: "tsuru_myapp"})
	c.Assert(err, check.IsNil)
	c.Assert(servers, check.HasLen, 2)
	c.Assert(servers[0].URL, check.Equals, u1.String())
	c.Assert(servers[1].URL, check.Equals, u2.String())
}

func (s *S) TestRemoveRoute(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)
	err = vRouter.AddBackend(routertest.FakeApp{Name: "myapp"})
	c.Assert(err, check.IsNil)
	u1, _ := url.Parse("http://1.1.1.1:111")
	u2, _ := url.Parse("http://2.2.2.2:222")
	err = vRouter.AddRoutes("myapp", []*url.URL{u1})
	c.Assert(err, check.IsNil)
	err = vRouter.AddRoutes("myapp", []*url.URL{u2})
	c.Assert(err, check.IsNil)
	servers, err := s.engine.GetServers(engine.BackendKey{Id: "tsuru_myapp"})
	c.Assert(err, check.IsNil)
	c.Assert(servers, check.HasLen, 2)
	err = vRouter.RemoveRoutes("myapp", []*url.URL{u1})
	c.Assert(err, check.IsNil)
	servers, err = s.engine.GetServers(engine.BackendKey{Id: "tsuru_myapp"})
	c.Assert(err, check.IsNil)
	c.Assert(servers, check.HasLen, 1)
	c.Assert(servers[0].URL, check.Equals, u2.String())
}

func (s *S) TestSetCName(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)
	err = vRouter.AddBackend(routertest.FakeApp{Name: "myapp"})
	c.Assert(err, check.IsNil)
	u1, _ := url.Parse("http://1.1.1.1:111")
	u2, _ := url.Parse("http://2.2.2.2:222")
	err = vRouter.AddRoutes("myapp", []*url.URL{u1})
	c.Assert(err, check.IsNil)
	err = vRouter.AddRoutes("myapp", []*url.URL{u2})
	c.Assert(err, check.IsNil)
	cnameRouter, ok := vRouter.(router.CNameRouter)
	c.Assert(ok, check.Equals, true)
	err = cnameRouter.SetCName("myapp.cname.example.com", "myapp")
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
	c.Assert(cnameFrontend.Route, check.Equals, `Host("myapp.cname.example.com")`)
	c.Assert(cnameFrontend.Type, check.Equals, "http")
}

func (s *S) TestSetCNameDuplicate(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)
	err = vRouter.AddBackend(routertest.FakeApp{Name: "myapp"})
	c.Assert(err, check.IsNil)
	u1, _ := url.Parse("http://1.1.1.1:111")
	u2, _ := url.Parse("http://2.2.2.2:222")
	err = vRouter.AddRoutes("myapp", []*url.URL{u1})
	c.Assert(err, check.IsNil)
	err = vRouter.AddRoutes("myapp", []*url.URL{u2})
	c.Assert(err, check.IsNil)
	cnameRouter, ok := vRouter.(router.CNameRouter)
	c.Assert(ok, check.Equals, true)
	err = cnameRouter.SetCName("myapp.cname.example.com", "myapp")
	c.Assert(err, check.IsNil)
	err = cnameRouter.SetCName("myapp.cname.example.com", "myapp")
	c.Assert(err, check.Equals, router.ErrCNameExists)
}

func (s *S) TestUnsetCName(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)
	err = vRouter.AddBackend(routertest.FakeApp{Name: "myapp"})
	c.Assert(err, check.IsNil)
	u1, _ := url.Parse("http://1.1.1.1:111")
	u2, _ := url.Parse("http://2.2.2.2:222")
	err = vRouter.AddRoutes("myapp", []*url.URL{u1})
	c.Assert(err, check.IsNil)
	err = vRouter.AddRoutes("myapp", []*url.URL{u2})
	c.Assert(err, check.IsNil)
	cnameRouter, ok := vRouter.(router.CNameRouter)
	c.Assert(ok, check.Equals, true)
	err = cnameRouter.SetCName("myapp.cname.example.com", "myapp")
	c.Assert(err, check.IsNil)
	frontends, err := s.engine.GetFrontends()
	c.Assert(err, check.IsNil)
	c.Assert(frontends, check.HasLen, 2)
	cnameRouter.UnsetCName("myapp.cname.example.com", "myapp")
	frontends, err = s.engine.GetFrontends()
	c.Assert(err, check.IsNil)
	c.Assert(frontends, check.HasLen, 1)
	c.Assert(frontends[0].Id, check.Equals, "tsuru_myapp.vulcand.example.com")
}

func (s *S) TestUnsetCNameNotExist(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)
	frontends, err := s.engine.GetFrontends()
	c.Assert(err, check.IsNil)
	c.Assert(frontends, check.HasLen, 0)
	cnameRouter, ok := vRouter.(router.CNameRouter)
	c.Assert(ok, check.Equals, true)
	err = cnameRouter.UnsetCName("myapp.cname.example.com", "myapp")
	c.Assert(err, check.Equals, router.ErrCNameNotFound)
}

func (s *S) TestAddr(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)
	err = vRouter.AddBackend(routertest.FakeApp{Name: "myapp"})
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
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
	c.Assert(addr, check.Equals, "")
}

func (s *S) TestSwap(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)
	err = vRouter.AddBackend(routertest.FakeApp{Name: "myapp1"})
	c.Assert(err, check.IsNil)
	u1, _ := url.Parse("http://1.1.1.1:111")
	u2, _ := url.Parse("http://2.2.2.2:222")
	err = vRouter.AddRoutes("myapp1", []*url.URL{u1})
	c.Assert(err, check.IsNil)
	err = vRouter.AddBackend(routertest.FakeApp{Name: "myapp2"})
	c.Assert(err, check.IsNil)
	err = vRouter.AddRoutes("myapp2", []*url.URL{u2})
	c.Assert(err, check.IsNil)
	err = vRouter.Swap("myapp1", "myapp2", false)
	c.Assert(err, check.IsNil)
	servers1, err := s.engine.GetServers(engine.BackendKey{Id: "tsuru_myapp1"})
	c.Assert(err, check.IsNil)
	c.Assert(servers1, check.HasLen, 1)
	c.Assert(servers1[0].URL, check.Equals, u2.String())
	servers2, err := s.engine.GetServers(engine.BackendKey{Id: "tsuru_myapp2"})
	c.Assert(err, check.IsNil)
	c.Assert(servers2, check.HasLen, 1)
	c.Assert(servers2[0].URL, check.Equals, u1.String())
}

func (s *S) TestRoutes(c *check.C) {
	vRouter, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)
	err = vRouter.AddBackend(routertest.FakeApp{Name: "myapp"})
	c.Assert(err, check.IsNil)
	u1, _ := url.Parse("http://1.1.1.1:111")
	u2, _ := url.Parse("http://2.2.2.2:222")
	err = vRouter.AddRoutes("myapp", []*url.URL{u1})
	c.Assert(err, check.IsNil)
	err = vRouter.AddRoutes("myapp", []*url.URL{u2})
	c.Assert(err, check.IsNil)
	routes, err := vRouter.Routes("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []*url.URL{u1, u2})
}

func (s *S) TestStartupMessage(c *check.C) {
	got, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)
	mRouter, ok := got.(router.MessageRouter)
	c.Assert(ok, check.Equals, true)
	message, err := mRouter.StartupMessage()
	c.Assert(err, check.IsNil)
	c.Assert(message, check.Equals,
		fmt.Sprintf(`vulcand router "vulcand.example.com" with API at "%s"`, s.vulcandServer.URL),
	)
}

func (s *S) TestHealthCheck(c *check.C) {
	got, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)
	hcRouter, ok := got.(router.HealthChecker)
	c.Assert(ok, check.Equals, true)
	c.Assert(hcRouter.HealthCheck(), check.IsNil)
}

func (s *S) TestHealthCheckFailure(c *check.C) {
	s.vulcandServer.Close()
	err := tsurutest.WaitCondition(time.Second, func() bool {
		_, err := tsuruNet.Dial5Full60ClientNoKeepAlive.Get(s.vulcandServer.URL)
		return err != nil
	})
	c.Assert(err, check.IsNil)
	got, err := router.Get("vulcand")
	c.Assert(err, check.IsNil)
	hcRouter, ok := got.(router.HealthChecker)
	c.Assert(ok, check.Equals, true)
	c.Assert(hcRouter.HealthCheck(), check.ErrorMatches, ".* connection refused")
}
