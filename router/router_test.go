// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"launchpad.net/gocheck"
)

func (s *S) TestRegisterAndGet(c *gocheck.C) {
	var r Router
	Register("router", r)
	got, err := Get("router")
	c.Assert(err, gocheck.IsNil)
	c.Assert(r, gocheck.DeepEquals, got)
	_, err = Get("unknown-router")
	c.Assert(err, gocheck.Not(gocheck.IsNil))
	expectedMessage := `Unknown router: "unknown-router".`
	c.Assert(expectedMessage, gocheck.Equals, err.Error())
}

func (s *S) TestStore(c *gocheck.C) {
	err := Store("appname", "routername")
	c.Assert(err, gocheck.IsNil)
}
