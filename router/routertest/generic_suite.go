// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package routertest

import (
	"net/url"

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

func (s *RouterSuite) TestRouteAddBackend(c *check.C) {
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
