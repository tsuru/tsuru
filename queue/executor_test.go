// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"github.com/globocom/tsuru/safe"
	. "launchpad.net/gocheck"
	"time"
)

type ExecutorSuite struct{}

var _ = Suite(&ExecutorSuite{})

func dumb() {
	time.Sleep(1e3)
}

func (s *ExecutorSuite) TestStart(c *C) {
	var ct safe.Counter
	h1 := executor{inner: func() { ct.Increment() }}
	h1.Start()
	c.Assert(h1.state, Equals, running)
	h1.Stop()
	h1.Wait()
	c.Assert(ct.Val(), Not(Equals), 0)
}

func (s *ExecutorSuite) TestPreempt(c *C) {
	h1 := executor{inner: dumb}
	h2 := executor{inner: dumb}
	h3 := executor{inner: dumb}
	h1.Start()
	h2.Start()
	h3.Start()
	Preempt()
	c.Assert(h1.state, Equals, stopped)
	c.Assert(h2.state, Equals, stopped)
	c.Assert(h3.state, Equals, stopped)
}

func (s *ExecutorSuite) TestStopNotRunningExecutor(c *C) {
	h := executor{inner: dumb}
	err := h.Stop()
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Not running.")
}

func (s *ExecutorSuite) TestExecutorImplementsHandler(c *C) {
	var _ Handler = &executor{}
}
