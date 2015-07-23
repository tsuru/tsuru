// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package routertest

import (
	"net/url"
	"sort"

	"github.com/tsuru/tsuru/router"
	"gopkg.in/check.v1"
)

type RouterSuite struct {
	Router            router.Router
	SetUpSuiteFunc    func(c *check.C)
	SetUpTestFunc     func(c *check.C)
	TearDownSuiteFunc func(c *check.C)
	TearDownTestFunc  func(c *check.C)
}

func (s *RouterSuite) SetUpSuite(c *check.C) {
	if s.SetUpSuiteFunc != nil {
		s.SetUpSuiteFunc(c)
	}
}

func (s *RouterSuite) SetUpTest(c *check.C) {
	if s.SetUpTestFunc != nil {
		s.SetUpTestFunc(c)
	}
}

func (s *RouterSuite) TearDownSuite(c *check.C) {
	if s.TearDownSuiteFunc != nil {
		s.TearDownSuiteFunc(c)
	}
}

func (s *RouterSuite) TearDownTest(c *check.C) {
	if s.TearDownTestFunc != nil {
		s.TearDownTestFunc(c)
	}
}

func (s *RouterSuite) TestRouteAddBackendAndRoute(c *check.C) {
	name := "backend1"
	err := s.Router.AddBackend(name)
	c.Assert(err, check.IsNil)
	addr, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(name, addr)
	c.Assert(err, check.IsNil)
	routes, err := s.Router.Routes(name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []*url.URL{addr})
}

func (s *RouterSuite) TestRouteRemoveRouteAndBackend(c *check.C) {
	name := "backend1"
	err := s.Router.AddBackend(name)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(name, addr1)
	addr2, err := url.Parse("http://10.10.10.11:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(name, addr2)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveRoute(name, addr1)
	c.Assert(err, check.IsNil)
	routes, err := s.Router.Routes(name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []*url.URL{addr2})
	err = s.Router.RemoveRoute(name, addr2)
	c.Assert(err, check.IsNil)
	routes, err = s.Router.Routes(name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []*url.URL{})
	err = s.Router.RemoveBackend(name)
	c.Assert(err, check.IsNil)
	_, err = s.Router.Routes(name)
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
}

func (s *RouterSuite) TestRouteRemoveUnknownRoute(c *check.C) {
	name := "backend1"
	err := s.Router.AddBackend(name)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveRoute(name, addr1)
	c.Assert(err, check.Equals, router.ErrRouteNotFound)
}

func (s *RouterSuite) TestRouteAddDupBackend(c *check.C) {
	name := "backend1"
	err := s.Router.AddBackend(name)
	c.Assert(err, check.IsNil)
	err = s.Router.AddBackend(name)
	c.Assert(err, check.Equals, router.ErrBackendExists)
}

func (s *RouterSuite) TestRouteAddDupRoute(c *check.C) {
	name := "backend1"
	err := s.Router.AddBackend(name)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(name, addr1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(name, addr1)
	c.Assert(err, check.Equals, router.ErrRouteExists)
}

func (s *RouterSuite) TestRouteAddRouteInvalidBackend(c *check.C) {
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute("backend1", addr1)
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
}

func (s *RouterSuite) TestSwap(c *check.C) {
	backend1 := "mybackend1"
	backend2 := "mybackend2"
	addr1, _ := url.Parse("http://127.0.0.1")
	addr2, _ := url.Parse("http://10.10.10.10")
	err := s.Router.AddBackend(backend1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(backend1, addr1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddBackend(backend2)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(backend2, addr2)
	c.Assert(err, check.IsNil)
	err = s.Router.Swap(backend1, backend2)
	c.Assert(err, check.IsNil)
	backAddr1, err := s.Router.Addr(backend1)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr1[:len(backend2)], check.Equals, backend2)
	backAddr2, err := s.Router.Addr(backend2)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr2[:len(backend1)], check.Equals, backend1)
	routes, err := s.Router.Routes(backend1)
	c.Assert(err, check.IsNil)
	c.Check(routes, check.DeepEquals, []*url.URL{addr1})
	routes, err = s.Router.Routes(backend2)
	c.Assert(err, check.IsNil)
	c.Check(routes, check.DeepEquals, []*url.URL{addr2})
	addr3, _ := url.Parse("http://127.0.0.2")
	addr4, _ := url.Parse("http://10.10.10.11")
	err = s.Router.AddRoute(backend1, addr3)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(backend2, addr4)
	c.Assert(err, check.IsNil)
	routes, err = s.Router.Routes(backend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 2)
	routesStrs := []string{routes[0].String(), routes[1].String()}
	sort.Strings(routesStrs)
	c.Check(routesStrs, check.DeepEquals, []string{addr1.String(), addr3.String()})
	routes, err = s.Router.Routes(backend2)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 2)
	routesStrs = []string{routes[0].String(), routes[1].String()}
	sort.Strings(routesStrs)
	c.Check(routesStrs, check.DeepEquals, []string{addr2.String(), addr4.String()})
}

func (s *RouterSuite) TestRouteAddDupCName(c *check.C) {
	name := "backend1"
	err := s.Router.AddBackend(name)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(name, addr1)
	c.Assert(err, check.IsNil)
	err = s.Router.SetCName("my.host.com", name)
	c.Assert(err, check.IsNil)
	err = s.Router.SetCName("my.host.com", name)
	c.Assert(err, check.Equals, router.ErrCNameExists)
}

func (s *RouterSuite) TestSetUnsetCName(c *check.C) {
	name := "backend1"
	err := s.Router.AddBackend(name)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(name, addr1)
	c.Assert(err, check.IsNil)
	err = s.Router.SetCName("my.host.com", name)
	c.Assert(err, check.IsNil)
	err = s.Router.UnsetCName("my.host.com", name)
	c.Assert(err, check.IsNil)
	err = s.Router.UnsetCName("my.host.com", name)
	c.Assert(err, check.Equals, router.ErrCNameNotFound)
}

func (s *RouterSuite) TestSetCNameInvalidBackend(c *check.C) {
	err := s.Router.SetCName("my.cname", "backend1")
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
}

func (s *RouterSuite) TestSetCNameSubdomainError(c *check.C) {
	name := "backend1"
	err := s.Router.AddBackend(name)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(name, addr1)
	c.Assert(err, check.IsNil)
	err = s.Router.SetCName("my.host.com", name)
	c.Assert(err, check.IsNil)
	addr, err := s.Router.Addr(name)
	c.Assert(err, check.IsNil)
	err = s.Router.SetCName("sub."+addr, name)
	c.Assert(err, check.Equals, router.ErrCNameNotAllowed)
}
