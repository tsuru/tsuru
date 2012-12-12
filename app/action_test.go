// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	. "launchpad.net/gocheck"
)

type helloAction struct {
	executed   bool
	rollbacked bool
}

func (h *helloAction) forward(a App) error {
	h.executed = true
	return nil
}

func (h *helloAction) backward(a App) {
	h.rollbacked = true
}

type errorAction struct {
	rollbacked bool
}

func (e *errorAction) forward(a App) error {
	return errors.New("")
}

func (e *errorAction) backward(a App) {
	e.rollbacked = true
}

func (s *S) TestExecute(c *C) {
	app := App{}
	h := new(helloAction)
	err := Execute(app, []action{h})
	c.Assert(err, IsNil)
	c.Assert(h.executed, Equals, true)
}

func (s *S) TestRollBack(c *C) {
	app := App{}
	h := new(helloAction)
	e := new(errorAction)
	err := Execute(app, []action{h, e})
	c.Assert(err, NotNil)
	c.Assert(e.rollbacked, Equals, true)
	c.Assert(h.rollbacked, Equals, true)
}

func (s *S) TestRollBack2(c *C) {
	app := App{}
	h := new(helloAction)
	e := new(errorAction)
	err := Execute(app, []action{e, h})
	c.Assert(err, NotNil)
	c.Assert(e.rollbacked, Equals, true)
	c.Assert(h.rollbacked, Equals, false)
}

func (s *S) TestExecuteShouldReturnsTheActionError(c *C) {
	app := App{}
	e := new(errorAction)
	err := Execute(app, []action{e})
	c.Assert(err, NotNil)
}
