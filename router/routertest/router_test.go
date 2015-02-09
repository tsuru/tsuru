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
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	conn *db.Storage
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "router_fake_tests")
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	dbtest.ClearAllCollections(s.conn.Collection("router_fake_tests").Database)
}

func (s *S) TestShouldBeRegistered(c *check.C) {
	r, err := router.Get("fake")
	c.Assert(err, check.IsNil)
	c.Assert(r, check.FitsTypeOf, &fakeRouter{})
}

func (s *S) TestAddBackend(c *check.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("foo")
	c.Assert(err, check.IsNil)
	defer r.RemoveBackend("foo")
	c.Assert(r.HasBackend("foo"), check.Equals, true)
}

func (s *S) TestAddDuplicateBackend(c *check.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("foo")
	c.Assert(err, check.IsNil)
	err = r.AddBackend("foo")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Backend already exists")
}

func (s *S) TestRemoveBackend(c *check.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("bar")
	c.Assert(err, check.IsNil)
	err = r.RemoveBackend("bar")
	c.Assert(err, check.IsNil)
	c.Assert(r.HasBackend("bar"), check.Equals, false)
}

func (s *S) TestRemoveUnknownBackend(c *check.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.RemoveBackend("bar")
	c.Assert(err, check.Equals, ErrBackendNotFound)
}

func (s *S) TestAddRoute(c *check.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("name")
	c.Assert(err, check.IsNil)
	err = r.AddRoute("name", "127.0.0.1")
	c.Assert(err, check.IsNil)
	c.Assert(r.HasRoute("name", "127.0.0.1"), check.Equals, true)
}

func (s *S) TestAddRouteBackendNotFound(c *check.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddRoute("name", "127.0.0.1")
	c.Assert(err, check.Equals, ErrBackendNotFound)
}

func (s *S) TestRemoveRoute(c *check.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("name")
	c.Assert(err, check.IsNil)
	err = r.AddRoute("name", "127.0.0.1")
	c.Assert(err, check.IsNil)
	err = r.RemoveRoute("name", "127.0.0.1")
	c.Assert(err, check.IsNil)
	c.Assert(r.HasRoute("name", "127.0.0.1"), check.Equals, false)
}

func (s *S) TestRemoveRouteBackendNotFound(c *check.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.RemoveRoute("name", "127.0.0.1")
	c.Assert(err, check.Equals, ErrBackendNotFound)
}

func (s *S) TestRemoveUnknownRoute(c *check.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("name")
	c.Assert(err, check.IsNil)
	err = r.RemoveRoute("name", "127.0.0.1")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Route not found")
}

func (s *S) TestSetCName(c *check.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("name")
	c.Assert(err, check.IsNil)
	err = r.AddRoute("name", "127.0.0.1")
	err = r.SetCName("myapp.com", "name")
	c.Assert(err, check.IsNil)
	c.Assert(r.HasBackend("myapp.com"), check.Equals, true)
	c.Assert(r.HasRoute("myapp.com", "127.0.0.1"), check.Equals, true)
}

func (s *S) TestUnsetCName(c *check.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("name")
	c.Assert(err, check.IsNil)
	err = r.AddRoute("name", "127.0.0.1")
	err = r.SetCName("myapp.com", "name")
	c.Assert(err, check.IsNil)
	err = r.UnsetCName("myapp.com", "name")
	c.Assert(err, check.IsNil)
	c.Assert(r.HasBackend("myapp.com"), check.Equals, false)
}

func (s *S) TestAddr(c *check.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("name")
	c.Assert(err, check.IsNil)
	err = r.AddRoute("name", "127.0.0.1")
	c.Assert(err, check.IsNil)
	addr, err := r.Addr("name")
	c.Assert(err, check.IsNil)
	c.Assert(addr, check.Equals, "127.0.0.1")
	addr, err = r.Addr("unknown")
	c.Assert(addr, check.Equals, "")
	c.Assert(err, check.Equals, ErrBackendNotFound)
}

func (s *S) TestReset(c *check.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("name")
	c.Assert(err, check.IsNil)
	err = r.AddRoute("name", "127.0.0.1")
	c.Assert(err, check.IsNil)
	r.Reset()
	c.Assert(r.HasBackend("name"), check.Equals, false)
}

func (s *S) TestRoutes(c *check.C) {
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend("name")
	c.Assert(err, check.IsNil)
	err = r.AddRoute("name", "127.0.0.1")
	c.Assert(err, check.IsNil)
	routes, err := r.Routes("name")
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []string{"127.0.0.1"})
}

func (s *S) TestSwap(c *check.C) {
	instance1 := "127.0.0.1"
	instance2 := "127.0.0.2"
	backend1 := "b1"
	backend2 := "b2"
	r := fakeRouter{backends: make(map[string][]string)}
	err := r.AddBackend(backend1)
	c.Assert(err, check.IsNil)
	err = r.AddRoute(backend1, instance1)
	c.Assert(err, check.IsNil)
	err = r.AddBackend(backend2)
	c.Assert(err, check.IsNil)
	err = r.AddRoute(backend2, instance2)
	c.Assert(err, check.IsNil)
	retrieved1, err := router.Retrieve(backend1)
	c.Assert(err, check.IsNil)
	c.Assert(retrieved1, check.Equals, backend1)
	retrieved2, err := router.Retrieve(backend2)
	c.Assert(err, check.IsNil)
	c.Assert(retrieved2, check.Equals, backend2)
	err = r.Swap(backend1, backend2)
	c.Assert(err, check.IsNil)
	routes, err := r.Routes(backend2)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []string{instance2})
	routes, err = r.Routes(backend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []string{instance1})
	retrieved1, err = router.Retrieve(backend1)
	c.Assert(err, check.IsNil)
	c.Assert(retrieved1, check.Equals, backend2)
	retrieved2, err = router.Retrieve(backend2)
	c.Assert(err, check.IsNil)
	c.Assert(retrieved2, check.Equals, backend1)
	addr, err := r.Addr(backend1)
	c.Assert(err, check.IsNil)
	c.Assert(addr, check.Equals, "127.0.0.2")
	addr, err = r.Addr(backend2)
	c.Assert(err, check.IsNil)
	c.Assert(addr, check.Equals, "127.0.0.1")
}
