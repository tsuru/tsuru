// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	. "launchpad.net/gocheck"
)

type helloAction struct {
	executed     bool
	args         []interface{}
	rollbackArgs []interface{}
	rolledback   bool
}

func (h *helloAction) forward(a *App, args ...interface{}) error {
	h.executed = true
	h.args = args
	return nil
}

func (h *helloAction) backward(a *App, args ...interface{}) {
	h.rollbackArgs = args
	h.rolledback = true
}

func (h *helloAction) rollbackItself() bool {
	return false
}

type errorAction struct {
	rolledback bool
}

func (e *errorAction) forward(a *App, args ...interface{}) error {
	return errors.New("")
}

func (e *errorAction) backward(a *App, args ...interface{}) {
	e.rolledback = true
}

func (h *errorAction) rollbackItself() bool {
	return false
}

type rollingBackItself struct {
	rolledback bool
}

func (a *rollingBackItself) forward(app *App, args ...interface{}) error {
	return errors.New("")
}

func (a *rollingBackItself) backward(app *App, args ...interface{}) {
	a.rolledback = true
}

func (a *rollingBackItself) rollbackItself() bool {
	return true
}

func (s *S) TestExecute(c *C) {
	app := App{}
	h := new(helloAction)
	err := execute(&app, []oldaction{h})
	c.Assert(err, IsNil)
	c.Assert(h.executed, Equals, true)
}

func (s *S) TestArgs(c *C) {
	app := App{}
	h := new(helloAction)
	err := execute(&app, []oldaction{h}, 23, "abcdef")
	c.Assert(err, IsNil)
	c.Assert(h.args, DeepEquals, []interface{}{23, "abcdef"})
}

func (s *S) TestRolbackArgs(c *C) {
	app := App{}
	h := new(helloAction)
	e := new(errorAction)
	err := execute(&app, []oldaction{h, e}, 14, "abc")
	c.Assert(err, NotNil)
	c.Assert(h.rollbackArgs, DeepEquals, []interface{}{14, "abc"})
}

func (s *S) TestRollBackFailureOnSecondAction(c *C) {
	app := App{}
	h := new(helloAction)
	e := new(errorAction)
	err := execute(&app, []oldaction{h, e})
	c.Assert(err, NotNil)
	c.Assert(h.rolledback, Equals, true)
	c.Assert(e.rolledback, Equals, false)
}

func (s *S) TestRollBackFailureOnFirstAction(c *C) {
	app := App{}
	h := new(helloAction)
	e := new(errorAction)
	err := execute(&app, []oldaction{e, h})
	c.Assert(err, NotNil)
	c.Assert(e.rolledback, Equals, false)
	c.Assert(h.rolledback, Equals, false)
}

func (s *S) TestRollBackFailureOnRollingbackItSelfAction(c *C) {
	app := App{}
	h := new(helloAction)
	r := new(rollingBackItself)
	err := execute(&app, []oldaction{h, r})
	c.Assert(err, NotNil)
	c.Assert(h.rolledback, Equals, true)
	c.Assert(r.rolledback, Equals, true)
}

func (s *S) TestExecuteShouldReturnsTheActionError(c *C) {
	app := App{}
	e := new(errorAction)
	err := execute(&app, []oldaction{e})
	c.Assert(err, NotNil)
}
