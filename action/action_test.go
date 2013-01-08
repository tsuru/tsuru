// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package action

import (
	"launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) TestSucessAndParameters(c *gocheck.C) {
	actions := []*Action{
		{
			Forward: func(ctx FWContext) (Result, error) {
				c.Assert(ctx.Params, gocheck.DeepEquals, []interface{}{"hello"})
				return "ok", nil
			},
		},
	}
	pipeline := NewPipeline(actions...)
	err := pipeline.Execute("hello")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestRollback(c *gocheck.C) {
	actions := []*Action{
		{
			Forward: func(ctx FWContext) (Result, error) {
				return "ok", nil
			},
			Backward: func(ctx BWContext) {
				c.Assert(ctx.Params, gocheck.DeepEquals, []interface{}{"hello", "world"})
				c.Assert(ctx.FWResult, gocheck.DeepEquals, "ok")
			},
		},
		&errorAction,
	}
	pipeline := NewPipeline(actions...)
	err := pipeline.Execute("hello", "world")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Failed to execute.")
}

func (s *S) TestRollbackUnrollbackableAction(c *gocheck.C) {
	actions := []*Action{
		&helloAction,
		&unrollbackableAction,
		&errorAction,
	}
	pipeline := NewPipeline(actions...)
	err := pipeline.Execute("hello")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Failed to execute.")
}

func (s *S) TestExecuteNoActions(c *gocheck.C) {
	pipeline := NewPipeline()
	err := pipeline.Execute()
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "No actions to execute.")
}

func (s *S) TestExecuteActionWithNilForward(c *gocheck.C) {
	var executed bool
	actions := []*Action{
		{
			Forward: func(ctx FWContext) (Result, error) {
				return "ok", nil
			},
			Backward: func(ctx BWContext) {
				executed = true
			},
		},
		{
			Forward:  nil,
			Backward: nil,
		},
	}
	pipeline := NewPipeline(actions...)
	err := pipeline.Execute()
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "All actions must define the forward function.")
	c.Assert(executed, gocheck.Equals, true)
}
