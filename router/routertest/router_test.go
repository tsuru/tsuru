// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package routertest

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/router"
	"launchpad.net/gocheck"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct {
	conn *db.Storage
}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "router_fake_tests")
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TearDownSuite(c *gocheck.C) {
	dbtest.ClearAllCollections(s.conn.Collection("router_fake_tests").Database)
}

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

func (s *S) TestSetCName(c *gocheck.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("name")
	c.Assert(err, gocheck.IsNil)
	err = r.AddRoute("name", "127.0.0.1")
	err = r.SetCName("myapp.com", "name")
	c.Assert(err, gocheck.IsNil)
	c.Assert(r.HasBackend("myapp.com"), gocheck.Equals, true)
	c.Assert(r.HasRoute("myapp.com", "127.0.0.1"), gocheck.Equals, true)
}

func (s *S) TestUnsetCName(c *gocheck.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("name")
	c.Assert(err, gocheck.IsNil)
	err = r.AddRoute("name", "127.0.0.1")
	err = r.SetCName("myapp.com", "name")
	c.Assert(err, gocheck.IsNil)
	err = r.UnsetCName("myapp.com", "name")
	c.Assert(err, gocheck.IsNil)
	c.Assert(r.HasBackend("myapp.com"), gocheck.Equals, false)
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

func (s *S) TestRoutes(c *gocheck.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("name")
	c.Assert(err, gocheck.IsNil)
	err = r.AddRoute("name", "127.0.0.1")
	c.Assert(err, gocheck.IsNil)
	routes, err := r.Routes("name")
	c.Assert(err, gocheck.IsNil)
	c.Assert(routes, gocheck.DeepEquals, []string{"127.0.0.1"})
}

func (s *S) TestSwap(c *gocheck.C) {
	instance1 := "127.0.0.1"
	instance2 := "127.0.0.2"
	backend1 := "b1"
	backend2 := "b2"
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend(backend1)
	c.Assert(err, gocheck.IsNil)
	err = r.AddRoute(backend1, instance1)
	c.Assert(err, gocheck.IsNil)
	err = r.AddBackend(backend2)
	c.Assert(err, gocheck.IsNil)
	err = r.AddRoute(backend2, instance2)
	c.Assert(err, gocheck.IsNil)
	retrieved1, err := router.Retrieve(backend1)
	c.Assert(err, gocheck.IsNil)
	c.Assert(retrieved1, gocheck.Equals, backend1)
	retrieved2, err := router.Retrieve(backend2)
	c.Assert(err, gocheck.IsNil)
	c.Assert(retrieved2, gocheck.Equals, backend2)
	err = r.Swap(backend1, backend2)
	c.Assert(err, gocheck.IsNil)
	routes, err := r.Routes(backend2)
	c.Assert(err, gocheck.IsNil)
	c.Assert(routes, gocheck.DeepEquals, []string{instance2})
	routes, err = r.Routes(backend1)
	c.Assert(err, gocheck.IsNil)
	c.Assert(routes, gocheck.DeepEquals, []string{instance1})
	retrieved1, err = router.Retrieve(backend1)
	c.Assert(err, gocheck.IsNil)
	c.Assert(retrieved1, gocheck.Equals, backend2)
	retrieved2, err = router.Retrieve(backend2)
	c.Assert(err, gocheck.IsNil)
	c.Assert(retrieved2, gocheck.Equals, backend1)
	addr, err := r.Addr(backend1)
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, "127.0.0.2")
	addr, err = r.Addr(backend2)
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, "127.0.0.1")
}
