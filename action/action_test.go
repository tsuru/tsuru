// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package action

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type S struct{}

var ctx = context.TODO()
var _ = check.Suite(&S{})

func (s *S) TestSuccessAndParameters(c *check.C) {
	tracer := otel.Tracer("test")
	parentCtx, parentSpan := tracer.Start(ctx, "parent operation")
	defer parentSpan.End()

	actions := []*Action{
		{
			Forward: func(ctx FWContext) (Result, error) {
				c.Assert(ctx.Params, check.DeepEquals, []interface{}{"hello"})

				currentSpan := trace.SpanFromContext(ctx.Context)
				c.Assert(currentSpan, check.Not(check.IsNil))
				return "ok", nil
			},
		},
	}
	pipeline := NewPipeline(actions...)

	err := pipeline.Execute(parentCtx, "hello")
	c.Assert(err, check.IsNil)
}

func (s *S) TestRollback(c *check.C) {
	var backwardCalled bool
	actions := []*Action{
		{
			Forward: func(ctx FWContext) (Result, error) {
				return "ok", nil
			},
			Backward: func(ctx BWContext) {
				c.Assert(ctx.Params, check.DeepEquals, []interface{}{"hello", "world"})
				c.Assert(ctx.FWResult, check.DeepEquals, "ok")

				currentSpan := trace.SpanFromContext(ctx.Context)
				c.Assert(currentSpan, check.Not(check.IsNil))

				backwardCalled = true
			},
		},
		&errorAction,
	}
	pipeline := NewPipeline(actions...)
	tracer := otel.Tracer("test")
	parentCtx, parentSpan := tracer.Start(ctx, "parent operation")
	defer parentSpan.End()
	err := pipeline.Execute(parentCtx, "hello", "world")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to execute.")
	c.Assert(backwardCalled, check.Equals, true)
}

func (s *S) TestRollbackOnPanic(c *check.C) {
	var backwardCalled bool
	actions := []*Action{
		{
			Forward: func(ctx FWContext) (Result, error) {
				return "ok", nil
			},
			Backward: func(ctx BWContext) {
				c.Assert(ctx.Params, check.DeepEquals, []interface{}{"hello", "world"})
				c.Assert(ctx.FWResult, check.DeepEquals, "ok")
				backwardCalled = true
			},
		},
		&panicAction,
	}
	pipeline := NewPipeline(actions...)
	err := pipeline.Execute(ctx, "hello", "world")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `panic running.*`)
	c.Assert(backwardCalled, check.Equals, true)
}

func (s *S) TestRollbackUnrollbackableAction(c *check.C) {
	actions := []*Action{
		&helloAction,
		&unrollbackableAction,
		&errorAction,
	}
	pipeline := NewPipeline(actions...)
	err := pipeline.Execute(ctx, "hello")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to execute.")
}

func (s *S) TestExecuteNoActions(c *check.C) {
	pipeline := NewPipeline()
	err := pipeline.Execute(ctx)
	c.Assert(err, check.Equals, ErrPipelineNoActions)
}

func (s *S) TestExecuteActionWithNilForward(c *check.C) {
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
	err := pipeline.Execute(ctx)
	c.Assert(err, check.Equals, ErrPipelineForwardMissing)
	c.Assert(executed, check.Equals, true)
}

func (s *S) TestExecuteMinParams(c *check.C) {
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
	err := pipeline.Execute(ctx)
	c.Assert(err, check.Equals, ErrPipelineFewParameters)
	c.Assert(executed, check.Equals, true)
}

func (s *S) TestResult(c *check.C) {
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
	err := pipeline.Execute(ctx)
	c.Assert(err, check.IsNil)
	r := pipeline.Result()
	c.Assert(r, check.Equals, "ok")
}

func (s *S) TestDoesntOverwriteResult(c *check.C) {
	myAction := Action{
		Forward: func(ctx FWContext) (Result, error) {
			return ctx.Params[0], nil
		},
		Backward: func(ctx BWContext) {
		},
	}
	pipeline1 := NewPipeline(&myAction)
	err := pipeline1.Execute(ctx, "result1")
	c.Assert(err, check.IsNil)
	pipeline2 := NewPipeline(&myAction)
	err = pipeline2.Execute(ctx, "result2")
	c.Assert(err, check.IsNil)
	r1 := pipeline1.Result()
	c.Assert(r1, check.Equals, "result1")
	r2 := pipeline2.Result()
	c.Assert(r2, check.Equals, "result2")
}

func (s *S) TestActionOnError(c *check.C) {
	returnedErr := errors.New("my awesome error")
	called := false
	expectedParam := "param"
	myAction := Action{
		Forward: func(ctx FWContext) (Result, error) {
			return nil, returnedErr
		},
		OnError: func(ctx FWContext, err error) {
			called = true
			c.Assert(ctx.Params[0], check.Equals, expectedParam)
			c.Assert(err, check.Equals, returnedErr)
		},
	}
	pipeline1 := NewPipeline(&myAction)
	err := pipeline1.Execute(ctx, expectedParam)
	c.Assert(err, check.Equals, returnedErr)
	c.Assert(called, check.Equals, true)
}
