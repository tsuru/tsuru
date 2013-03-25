// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"launchpad.net/gocheck"
)

func (s *S) TestNewLogListener(c *gocheck.C) {
	app := App{Name: "myapp"}
	l := NewLogListener(&app)
	c.Assert(l.state, gocheck.Equals, open)
	c.Assert(l.C, gocheck.NotNil)
	close(l.c)
	_, ok := <-l.C
	c.Assert(ok, gocheck.Equals, false)
	ls := listeners.m["myapp"]
	c.Assert(ls, gocheck.HasLen, 1)
	c.Assert(ls[0], gocheck.Equals, l)
	delete(listeners.m, "myapp")
}
