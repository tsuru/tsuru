// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/provision"
	"launchpad.net/gocheck"
)

func (s *S) TestUnitGetName(c *gocheck.C) {
	u := Unit{Name: "abcdef", app: &App{Name: "2112"}}
	c.Assert(u.GetName(), gocheck.Equals, "abcdef")
}

func (s *S) TestUnitGetMachine(c *gocheck.C) {
	u := Unit{Machine: 10}
	c.Assert(u.GetMachine(), gocheck.Equals, u.Machine)
}

func (s *S) TestUnitGetStatus(c *gocheck.C) {
	var tests = []struct {
		input    string
		expected provision.Status
	}{
		{"started", provision.StatusStarted},
		{"pending", provision.StatusPending},
		{"creating", provision.StatusCreating},
		{"down", provision.StatusDown},
		{"error", provision.StatusError},
		{"installing", provision.StatusInstalling},
		{"creating", provision.StatusCreating},
	}
	for _, test := range tests {
		u := Unit{State: test.input}
		got := u.GetStatus()
		if got != test.expected {
			c.Errorf("u.GetStatus(): want %q, got %q.", test.expected, got)
		}
	}
}

func (s *S) TestUnitShouldBeABinderUnit(c *gocheck.C) {
	var _ bind.Unit = &Unit{}
}

func (s *S) TestUnitSliceLen(c *gocheck.C) {
	units := UnitSlice{Unit{}, Unit{}}
	c.Assert(units.Len(), gocheck.Equals, 2)
}
