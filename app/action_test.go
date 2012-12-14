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

func (h *helloAction) forward(a *App) error {
	h.executed = true
	return nil
}

func (h *helloAction) backward(a *App) {
	h.rollbacked = true
}

func (h *helloAction) rollbackItself() bool {
	return false
}

type errorAction struct {
	rollbacked bool
}

func (e *errorAction) forward(a *App) error {
	return errors.New("")
}

func (e *errorAction) backward(a *App) {
	e.rollbacked = true
}

func (h *errorAction) rollbackItself() bool {
	return false
}

type rollingBackItself struct {
	rolledback bool
}

func (a *rollingBackItself) forward(app *App) error {
	return errors.New("")
}

func (a *rollingBackItself) backward(app *App) {
	a.rolledback = true
}

func (a *rollingBackItself) rollbackItself() bool {
	return true
}

func (s *S) TestExecute(c *C) {
	app := App{}
	h := new(helloAction)
	err := execute(&app, []action{h})
	c.Assert(err, IsNil)
	c.Assert(h.executed, Equals, true)
}

func (s *S) TestRollBackFailureOnSecondAction(c *C) {
	app := App{}
	h := new(helloAction)
	e := new(errorAction)
	err := execute(&app, []action{h, e})
	c.Assert(err, NotNil)
	c.Assert(h.rollbacked, Equals, true)
	c.Assert(e.rollbacked, Equals, false)
}

func (s *S) TestRollBackFailureOnFirstAction(c *C) {
	app := App{}
	h := new(helloAction)
	e := new(errorAction)
	err := execute(&app, []action{e, h})
	c.Assert(err, NotNil)
	c.Assert(e.rollbacked, Equals, false)
	c.Assert(h.rollbacked, Equals, false)
}

func (s *S) TestRollBackFailureOnRollingbackItSelfAction(c *C) {
	app := App{}
	h := new(helloAction)
	r := new(rollingBackItself)
	err := execute(&app, []action{h, r})
	c.Assert(err, NotNil)
	c.Assert(h.rollbacked, Equals, true)
	c.Assert(r.rolledback, Equals, true)
}

func (s *S) TestExecuteShouldReturnsTheActionError(c *C) {
	app := App{}
	e := new(errorAction)
	err := execute(&app, []action{e})
	c.Assert(err, NotNil)
}
