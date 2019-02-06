// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router_test

import (
	"net/url"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/router"
	_ "github.com/tsuru/tsuru/router/hipache"
	"github.com/tsuru/tsuru/router/routertest"
	check "gopkg.in/check.v1"
)

type ExternalSuite struct {
	conn *db.Storage
}

var _ = check.Suite(&ExternalSuite{})

func (s *ExternalSuite) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("hipache:domain", "swaptest.org")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "router_swap_tests")
	config.Set("routers:fake:type", "fake")
}

func (s *ExternalSuite) SetUpTest(c *check.C) {
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	dbtest.ClearAllCollections(s.conn.Collection("router").Database)
}

func (s *ExternalSuite) TearDownTest(c *check.C) {
	s.conn.Close()
}

func (s *ExternalSuite) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Apps().Database.DropDatabase()
}

func (s *ExternalSuite) TestSwapCnameOnly(c *check.C) {
	backend1 := "bx1"
	backend2 := "bx2"
	r, err := router.Get("fake")
	c.Assert(err, check.IsNil)
	cnameRouter, ok := r.(router.CNameRouter)
	c.Assert(ok, check.Equals, true)
	err = r.AddBackend(routertest.FakeApp{Name: backend1})
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://127.0.0.1")
	c.Assert(err, check.IsNil)
	r.AddRoutes(backend1, []*url.URL{addr1})
	err = cnameRouter.SetCName("cname.com", backend1)
	c.Assert(err, check.IsNil)
	err = r.AddBackend(routertest.FakeApp{Name: backend2})
	c.Assert(err, check.IsNil)
	addr2, err := url.Parse("http://10.10.10.10")
	c.Assert(err, check.IsNil)
	r.AddRoutes(backend2, []*url.URL{addr2})
	err = router.Swap(r, backend1, backend2, true)
	c.Assert(err, check.IsNil)
	routes1, err := r.Routes(backend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes1, check.DeepEquals, []*url.URL{addr1})
	routes2, err := r.Routes(backend2)
	c.Assert(err, check.IsNil)
	c.Assert(routes2, check.DeepEquals, []*url.URL{addr2})
	name1, err := router.Retrieve(backend1)
	c.Assert(err, check.IsNil)
	c.Assert(name1, check.Equals, backend1)
	name2, err := router.Retrieve(backend2)
	c.Assert(err, check.IsNil)
	c.Assert(name2, check.Equals, backend2)
	cnames, err := cnameRouter.CNames(backend1)
	c.Assert(err, check.IsNil)
	c.Assert(cnames, check.HasLen, 0)
	expected := []*url.URL{{Host: "cname.com"}}
	cnames, err = cnameRouter.CNames(backend2)
	c.Assert(err, check.IsNil)
	c.Assert(expected, check.DeepEquals, cnames)
	err = router.Swap(r, backend1, backend2, true)
	c.Assert(err, check.IsNil)
	cnames, err = cnameRouter.CNames(backend1)
	c.Assert(err, check.IsNil)
	c.Assert(expected, check.DeepEquals, cnames)
	cnames, err = cnameRouter.CNames(backend2)
	c.Assert(err, check.IsNil)
	c.Assert(cnames, check.HasLen, 0)
}

func (s *ExternalSuite) TestSwap(c *check.C) {
	backend1 := "b1"
	backend2 := "b2"
	r, err := router.Get("fake")
	c.Assert(err, check.IsNil)
	r.AddBackend(routertest.FakeApp{Name: backend1})
	addr1, _ := url.Parse("http://127.0.0.1")
	r.AddRoutes(backend1, []*url.URL{addr1})
	r.AddBackend(routertest.FakeApp{Name: backend2})
	addr2, _ := url.Parse("http://10.10.10.10")
	r.AddRoutes(backend2, []*url.URL{addr2})
	err = router.Swap(r, backend1, backend2, false)
	c.Assert(err, check.IsNil)
	routes1, err := r.Routes(backend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes1, check.DeepEquals, []*url.URL{addr1})
	routes2, err := r.Routes(backend2)
	c.Assert(err, check.IsNil)
	c.Assert(routes2, check.DeepEquals, []*url.URL{addr2})
	name1, err := router.Retrieve(backend1)
	c.Assert(err, check.IsNil)
	c.Assert(name1, check.Equals, backend2)
	name2, err := router.Retrieve(backend2)
	c.Assert(err, check.IsNil)
	c.Assert(name2, check.Equals, backend1)
}

func (s *ExternalSuite) TestSwapWithDifferentRouterKinds(c *check.C) {
	config.Set("hipache:redis-server", "127.0.0.1:6379")
	config.Set("hipache:redis-db", 5)
	backend1 := "bb1"
	backend2 := "bb2"
	r1, err := router.Get("fake")
	c.Assert(err, check.IsNil)
	r2, err := router.Get("hipache")
	c.Assert(err, check.IsNil)
	err = r1.AddBackend(routertest.FakeApp{Name: backend1})
	c.Assert(err, check.IsNil)
	defer r1.RemoveBackend(backend1)
	addr1, _ := url.Parse("http://127.0.0.1")
	err = r1.AddRoutes(backend1, []*url.URL{addr1})
	c.Assert(err, check.IsNil)
	defer r1.RemoveRoutes(backend1, []*url.URL{addr1})
	err = r2.AddBackend(routertest.FakeApp{Name: backend2})
	c.Assert(err, check.IsNil)
	defer r2.RemoveBackend(backend2)
	addr2, _ := url.Parse("http://10.10.10.10")
	err = r2.AddRoutes(backend2, []*url.URL{addr2})
	c.Assert(err, check.IsNil)
	defer r2.RemoveRoutes(backend2, []*url.URL{addr2})
	err = router.Swap(r1, backend1, backend2, false)
	c.Assert(err, check.ErrorMatches, `swap is only allowed between routers of the same kind. "bb1" uses "fake", "bb2" uses "hipache"`)
	err = router.Swap(r2, backend1, backend2, false)
	c.Assert(err, check.ErrorMatches, `swap is only allowed between routers of the same kind. "bb1" uses "fake", "bb2" uses "hipache"`)
}
