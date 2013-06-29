// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package elb

import (
	"github.com/globocom/tsuru/router"
	"launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) TestShouldBeRegistered(c *gocheck.C) {
	r, err := router.Get("elb")
	c.Assert(err, gocheck.IsNil)
	_, ok := r.(elbRouter)
	c.Assert(ok, gocheck.Equals, true)
}
