// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"github.com/tsuru/tsuru/safe"
	"launchpad.net/gocheck"
	"time"
)

type ExecutorSuite struct{}

var _ = gocheck.Suite(&ExecutorSuite{})

func dumb() {
	time.Sleep(1e3)
}

func (s *ExecutorSuite) TestStart(c *gocheck.C) {
	var ct safe.Counter
	h1 := executor{inner: func() { ct.Increment() }}
	h1.Start()
	c.Assert(h1.state, gocheck.Equals, running)
	h1.Stop()
	h1.Wait()
	c.Assert(ct.Val(), gocheck.Not(gocheck.Equals), 0)
}

func (s *ExecutorSuite) TestPreempt(c *gocheck.C) {
	h1 := executor{inner: dumb}
	h2 := executor{inner: dumb}
	h3 := executor{inner: dumb}
	h1.Start()
	h2.Start()
	h3.Start()
	Preempt()
	c.Assert(h1.state, gocheck.Equals, stopped)
	c.Assert(h2.state, gocheck.Equals, stopped)
	c.Assert(h3.state, gocheck.Equals, stopped)
}

func (s *ExecutorSuite) TestStopNotRunningExecutor(c *gocheck.C) {
	h := executor{inner: dumb}
	err := h.Stop()
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Not running.")
}

func (s *ExecutorSuite) TestExecutorImplementsHandler(c *gocheck.C) {
	var _ Handler = &executor{}
}
