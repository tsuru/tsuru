// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router_test

import (
	"context"
	"net/url"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/routertest"
	check "gopkg.in/check.v1"
)

type ExternalSuite struct {
	conn *db.Storage
}

var _ = check.Suite(&ExternalSuite{})

func (s *ExternalSuite) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
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
	dbtest.ClearAllCollections(conn.Apps().Database)
}

func (s *ExternalSuite) TestSwapCnameOnly(c *check.C) {
	backend1 := routertest.FakeApp{Name: "bx1"}
	backend2 := routertest.FakeApp{Name: "bx2"}
	r, err := router.Get(context.TODO(), "fake")
	c.Assert(err, check.IsNil)
	cnameRouter, ok := r.(router.CNameRouter)
	c.Assert(ok, check.Equals, true)
	err = r.AddBackend(context.TODO(), backend1)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://127.0.0.1")
	c.Assert(err, check.IsNil)
	r.AddRoutes(context.TODO(), backend1, []*url.URL{addr1})
	err = cnameRouter.SetCName(context.TODO(), "cname.com", backend1)
	c.Assert(err, check.IsNil)
	err = r.AddBackend(context.TODO(), backend2)
	c.Assert(err, check.IsNil)
	addr2, err := url.Parse("http://10.10.10.10")
	c.Assert(err, check.IsNil)
	r.AddRoutes(context.TODO(), backend2, []*url.URL{addr2})
	err = router.Swap(context.TODO(), r, backend1, backend2, true)
	c.Assert(err, check.IsNil)
	routes1, err := r.Routes(context.TODO(), backend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes1, check.DeepEquals, []*url.URL{addr1})
	routes2, err := r.Routes(context.TODO(), backend2)
	c.Assert(err, check.IsNil)
	c.Assert(routes2, check.DeepEquals, []*url.URL{addr2})
	name1, err := router.Retrieve(backend1.GetName())
	c.Assert(err, check.IsNil)
	c.Assert(name1, check.Equals, backend1.GetName())
	name2, err := router.Retrieve(backend2.GetName())
	c.Assert(err, check.IsNil)
	c.Assert(name2, check.Equals, backend2.GetName())
	cnames, err := cnameRouter.CNames(context.TODO(), backend1)
	c.Assert(err, check.IsNil)
	c.Assert(cnames, check.HasLen, 0)
	expected := []*url.URL{{Host: "cname.com"}}
	cnames, err = cnameRouter.CNames(context.TODO(), backend2)
	c.Assert(err, check.IsNil)
	c.Assert(expected, check.DeepEquals, cnames)
	err = router.Swap(context.TODO(), r, backend1, backend2, true)
	c.Assert(err, check.IsNil)
	cnames, err = cnameRouter.CNames(context.TODO(), backend1)
	c.Assert(err, check.IsNil)
	c.Assert(expected, check.DeepEquals, cnames)
	cnames, err = cnameRouter.CNames(context.TODO(), backend2)
	c.Assert(err, check.IsNil)
	c.Assert(cnames, check.HasLen, 0)
}

func (s *ExternalSuite) TestSwap(c *check.C) {
	backend1 := routertest.FakeApp{Name: "b1"}
	backend2 := routertest.FakeApp{Name: "b2"}
	r, err := router.Get(context.TODO(), "fake")
	c.Assert(err, check.IsNil)
	r.AddBackend(context.TODO(), backend1)
	addr1, _ := url.Parse("http://127.0.0.1")
	r.AddRoutes(context.TODO(), backend1, []*url.URL{addr1})
	r.AddBackend(context.TODO(), backend2)
	addr2, _ := url.Parse("http://10.10.10.10")
	r.AddRoutes(context.TODO(), backend2, []*url.URL{addr2})
	err = router.Swap(context.TODO(), r, backend1, backend2, false)
	c.Assert(err, check.IsNil)
	routes1, err := r.Routes(context.TODO(), backend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes1, check.DeepEquals, []*url.URL{addr1})
	routes2, err := r.Routes(context.TODO(), backend2)
	c.Assert(err, check.IsNil)
	c.Assert(routes2, check.DeepEquals, []*url.URL{addr2})
	name1, err := router.Retrieve(backend1.GetName())
	c.Assert(err, check.IsNil)
	c.Assert(name1, check.Equals, backend2.GetName())
	name2, err := router.Retrieve(backend2.GetName())
	c.Assert(err, check.IsNil)
	c.Assert(name2, check.Equals, backend1.GetName())
}
