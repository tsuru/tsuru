// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"github.com/globocom/tsuru/router"
	"launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) TestShouldBeRegistered(c *gocheck.C) {
	r, err := router.Get("fake")
	c.Assert(err, gocheck.IsNil)
	c.Assert(r, gocheck.FitsTypeOf, &FakeRouter{})
}

func (s *S) TestAddBackend(c *gocheck.C) {
	var r FakeRouter
	err := r.AddBackend("foo")
	c.Assert(err, gocheck.IsNil)
	defer r.RemoveBackend("foo")
	c.Assert(r.HasBackend("foo"), gocheck.Equals, true)
}

func (s *S) TestRemoveBackend(c *gocheck.C) {
	var r FakeRouter
	err := r.AddBackend("bar")
	c.Assert(err, gocheck.IsNil)
	err = r.RemoveBackend("bar")
	c.Assert(err, gocheck.IsNil)
	c.Assert(r.HasBackend("bar"), gocheck.Equals, false)
}

func (s *S) TestAddRoute(c *gocheck.C) {
	var r FakeRouter
	err := r.AddRoute("name", "127.0.0.1")
	c.Assert(err, gocheck.IsNil)
	c.Assert(r.HasRoute("name"), gocheck.Equals, true)
}

func (s *S) TestRemoveRoute(c *gocheck.C) {
	var r FakeRouter
	err := r.AddRoute("name", "127.0.0.1")
	c.Assert(err, gocheck.IsNil)
	err = r.RemoveRoute("name", "127.0.0.1")
	c.Assert(err, gocheck.IsNil)
	c.Assert(r.HasRoute("name"), gocheck.Equals, false)
}

func (s *S) TestAddr(c *gocheck.C) {
	var r FakeRouter
	err := r.AddRoute("name", "127.0.0.1")
	c.Assert(err, gocheck.IsNil)
	addr, err := r.Addr("name")
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, "127.0.0.1")
	addr, err = r.Addr("unknown")
	c.Assert(addr, gocheck.Equals, "")
	c.Assert(err.Error(), gocheck.Equals, "Route not found")
}
