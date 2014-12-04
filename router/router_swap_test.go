// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router_test

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/router"
	_ "github.com/tsuru/tsuru/router/hipache"
	_ "github.com/tsuru/tsuru/router/testing"
	ttesting "github.com/tsuru/tsuru/testing"
	"launchpad.net/gocheck"
)

type SwapSuite struct {
	conn *db.Storage
}

var _ = gocheck.Suite(&SwapSuite{})

func (s *SwapSuite) SetUpSuite(c *gocheck.C) {
	config.Set("hipache:domain", "swaptest.org")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "router_swap_tests")
}

func (s *SwapSuite) SetUpTest(c *gocheck.C) {
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
	ttesting.ClearAllCollections(s.conn.Collection("router").Database)
}

func (s *SwapSuite) TestSwap(c *gocheck.C) {
	backend1 := "b1"
	backend2 := "b2"
	r, err := router.Get("fake")
	c.Assert(err, gocheck.IsNil)
	r.AddBackend(backend1)
	r.AddRoute(backend1, "http://127.0.0.1")
	r.AddBackend(backend2)
	r.AddRoute(backend2, "http://10.10.10.10")
	err = router.Swap(r, backend1, backend2)
	c.Assert(err, gocheck.IsNil)
	routes1, err := r.Routes(backend1)
	c.Assert(err, gocheck.IsNil)
	c.Assert(routes1, gocheck.DeepEquals, []string{"http://127.0.0.1"})
	routes2, err := r.Routes(backend2)
	c.Assert(err, gocheck.IsNil)
	c.Assert(routes2, gocheck.DeepEquals, []string{"http://10.10.10.10"})
	name1, err := router.Retrieve(backend1)
	c.Assert(err, gocheck.IsNil)
	c.Assert(name1, gocheck.Equals, backend2)
	name2, err := router.Retrieve(backend2)
	c.Assert(err, gocheck.IsNil)
	c.Assert(name2, gocheck.Equals, backend1)
}

func (s *SwapSuite) TestSwapWithDifferentRouterKinds(c *gocheck.C) {
	backend1 := "bb1"
	backend2 := "bb2"
	r1, err := router.Get("fake")
	c.Assert(err, gocheck.IsNil)
	r2, err := router.Get("hipache")
	c.Assert(err, gocheck.IsNil)
	err = r1.AddBackend(backend1)
	c.Assert(err, gocheck.IsNil)
	err = r1.AddRoute(backend1, "http://127.0.0.1")
	c.Assert(err, gocheck.IsNil)
	err = r2.AddBackend(backend2)
	c.Assert(err, gocheck.IsNil)
	err = r2.AddRoute(backend2, "http://10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	err = router.Swap(r1, backend1, backend2)
	c.Assert(err, gocheck.ErrorMatches, `swap is only allowed between routers of the same kind. "bb1" uses "fake", "bb2" uses "hipache"`)
	err = router.Swap(r2, backend1, backend2)
	c.Assert(err, gocheck.ErrorMatches, `swap is only allowed between routers of the same kind. "bb1" uses "fake", "bb2" uses "hipache"`)
}
