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

func (s *S) TestAddRoute(c *gocheck.C) {
	var r FakeRouter
	err := r.AddRoute("name", "127.0.0.1")
	c.Assert(err, gocheck.IsNil)
	c.Assert(r.HasRoute("name"), gocheck.Equals, true)
}

func (s *S) TestRestart(c *gocheck.C) {
	var r FakeRouter
	err := r.Restart()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestRemoveRoute(c *gocheck.C) {
	var r FakeRouter
	err := r.AddRoute("name", "127.0.0.1")
	c.Assert(err, gocheck.IsNil)
	err = r.RemoveRoute("name")
	c.Assert(err, gocheck.IsNil)
	c.Assert(r.HasRoute("name"), gocheck.Equals, false)
}
