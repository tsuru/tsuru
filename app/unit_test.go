// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/globocom/tsuru/api/bind"
	. "launchpad.net/gocheck"
)

func (s *S) TestUnitGetName(c *C) {
	u := Unit{Name: "abcdef", app: &App{Name: "2112"}}
	c.Assert(u.GetName(), Equals, "abcdef")
}

func (s *S) TestUnitGetMachine(c *C) {
	u := Unit{Machine: 10}
	c.Assert(u.GetMachine(), Equals, u.Machine)
}

func (s *S) TestUnitShouldBeABinderUnit(c *C) {
	var _ bind.Unit = &Unit{}
}
