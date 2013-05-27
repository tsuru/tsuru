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
	c.Assert(r, gocheck.FitsTypeOf, &fakeRouter{})
}

func (s *S) TestAddBackend(c *gocheck.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("foo")
	c.Assert(err, gocheck.IsNil)
	defer r.RemoveBackend("foo")
	c.Assert(r.HasBackend("foo"), gocheck.Equals, true)
}

func (s *S) TestAddDuplicateBackend(c *gocheck.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("foo")
	c.Assert(err, gocheck.IsNil)
	err = r.AddBackend("foo")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Backend already exists")
}

func (s *S) TestRemoveBackend(c *gocheck.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("bar")
	c.Assert(err, gocheck.IsNil)
	err = r.RemoveBackend("bar")
	c.Assert(err, gocheck.IsNil)
	c.Assert(r.HasBackend("bar"), gocheck.Equals, false)
}

func (s *S) TestRemoveUnknownBackend(c *gocheck.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.RemoveBackend("bar")
	c.Assert(err, gocheck.Equals, ErrBackendNotFound)
}

func (s *S) TestAddRoute(c *gocheck.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("name")
	c.Assert(err, gocheck.IsNil)
	err = r.AddRoute("name", "127.0.0.1")
	c.Assert(err, gocheck.IsNil)
	c.Assert(r.HasRoute("name", "127.0.0.1"), gocheck.Equals, true)
}

func (s *S) TestAddRouteBackendNotFound(c *gocheck.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddRoute("name", "127.0.0.1")
	c.Assert(err, gocheck.Equals, ErrBackendNotFound)
}

func (s *S) TestRemoveRoute(c *gocheck.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("name")
	c.Assert(err, gocheck.IsNil)
	err = r.AddRoute("name", "127.0.0.1")
	c.Assert(err, gocheck.IsNil)
	err = r.RemoveRoute("name", "127.0.0.1")
	c.Assert(err, gocheck.IsNil)
	c.Assert(r.HasRoute("name", "127.0.0.1"), gocheck.Equals, false)
}

func (s *S) TestRemoveRouteBackendNotFound(c *gocheck.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.RemoveRoute("name", "127.0.0.1")
	c.Assert(err, gocheck.Equals, ErrBackendNotFound)
}

func (s *S) TestRemoveUnknownRoute(c *gocheck.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("name")
	c.Assert(err, gocheck.IsNil)
	err = r.RemoveRoute("name", "127.0.0.1")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Route not found")
}

func (s *S) TestAddCName(c *gocheck.C) {
	var r fakeRouter
	err := r.AddCName("myapp.com", "name")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestRemoveCName(c *gocheck.C) {
	var r fakeRouter
	err := r.RemoveCName("myapp.com", "127.0.0.1")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestAddr(c *gocheck.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("name")
	c.Assert(err, gocheck.IsNil)
	err = r.AddRoute("name", "127.0.0.1")
	c.Assert(err, gocheck.IsNil)
	addr, err := r.Addr("name")
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, "127.0.0.1")
	addr, err = r.Addr("unknown")
	c.Assert(addr, gocheck.Equals, "")
	c.Assert(err, gocheck.Equals, ErrBackendNotFound)
}

func (s *S) TestReset(c *gocheck.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("name")
	c.Assert(err, gocheck.IsNil)
	err = r.AddRoute("name", "127.0.0.1")
	c.Assert(err, gocheck.IsNil)
	r.Reset()
	c.Assert(r.HasBackend("name"), gocheck.Equals, false)
}
