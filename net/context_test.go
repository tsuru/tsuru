// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"context"
	"time"

	check "gopkg.in/check.v1"
)

type TsuruForTesting string

const TSURU_STR = TsuruForTesting("tsuru")

func (s *S) TestWithoutCancelContext(c *check.C) {
	ctx := context.WithValue(context.Background(), TSURU_STR, "power")
	ctx, cancel := context.WithCancel(ctx)
	ctx = WithoutCancel(ctx)
	cancel()

	c.Assert(ctx.Err(), check.IsNil)
	c.Assert(ctx.Done(), check.IsNil)
	c.Assert(ctx.Value(TSURU_STR), check.Equals, "power")
}

func (s *S) TestWithoutCancelRemovesDeadline(c *check.C) {
	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	ctx := WithoutCancel(parent)
	_, ok := ctx.Deadline()

	c.Assert(ok, check.Equals, false)
}

func (s *S) TestCancelableParentContext(c *check.C) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx := WithoutCancel(parent)

	c.Assert(CancelableParentContext(ctx), check.Equals, parent)
}
