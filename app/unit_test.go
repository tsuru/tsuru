// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/globocom/tsuru/api/bind"
	"github.com/globocom/tsuru/provision"
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

func (s *S) TestUnitGetStatus(c *C) {
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

func (s *S) TestUnitShouldBeABinderUnit(c *C) {
	var _ bind.Unit = &Unit{}
}
