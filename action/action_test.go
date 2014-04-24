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

func (s *S) TestExecuteMinParams(c *gocheck.C) {
	var executed bool
	actions := []*Action{
		{
			Forward: func(ctx FWContext) (Result, error) {
				return "ok", nil
			},
			Backward: func(ctx BWContext) {
				executed = true
			},
			MinParams: 0,
		},
		{
			Forward: func(ctx FWContext) (Result, error) {
				return "ok", nil
			},
			MinParams: 1,
		},
	}
	pipeline := NewPipeline(actions...)
	err := pipeline.Execute()
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Not enough parameters to call Action.Forward.")
	c.Assert(executed, gocheck.Equals, true)
}

func (s *S) TestResult(c *gocheck.C) {
	actions := []*Action{
		{
			Forward: func(ctx FWContext) (Result, error) {
				return "ok", nil
			},
			Backward: func(ctx BWContext) {
			},
		},
	}
	pipeline := NewPipeline(actions...)
	err := pipeline.Execute()
	c.Assert(err, gocheck.IsNil)
	r := pipeline.Result()
	c.Assert(r, gocheck.Equals, "ok")
}

func (s *S) TestDoesntOverwriteResult(c *gocheck.C) {
	myAction := Action{
		Forward: func(ctx FWContext) (Result, error) {
			return ctx.Params[0], nil
		},
		Backward: func(ctx BWContext) {
		},
	}
	pipeline1 := NewPipeline(&myAction)
	err := pipeline1.Execute("result1")
	c.Assert(err, gocheck.IsNil)
	pipeline2 := NewPipeline(&myAction)
	err = pipeline2.Execute("result2")
	c.Assert(err, gocheck.IsNil)
	r1 := pipeline1.Result()
	c.Assert(r1, gocheck.Equals, "result1")
	r2 := pipeline2.Result()
	c.Assert(r2, gocheck.Equals, "result2")
}
