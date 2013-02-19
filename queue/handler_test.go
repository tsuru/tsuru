// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	. "launchpad.net/gocheck"
	"sync/atomic"
	"time"
)

type HandlerSuite struct{}

var _ = Suite(&HandlerSuite{})

func dumb() {
	time.Sleep(1e3)
}

func (s *HandlerSuite) TestStart(c *C) {
	var ct counter
	h1 := executor{inner: func() { ct.increment() }}
	h1.Start()
	c.Assert(h1.state, Equals, running)
	h1.Stop()
	h1.Wait()
	c.Assert(ct.value(), Not(Equals), 0)
}

func (s *HandlerSuite) TestPreempt(c *C) {
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

func (s *HandlerSuite) TestStopNotRunningHandler(c *C) {
	h := executor{inner: dumb}
	err := h.Stop()
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Not running.")
}

type counter struct {
	v int32
}

func (c *counter) increment() {
	old := atomic.LoadInt32(&c.v)
	swapped := atomic.CompareAndSwapInt32(&c.v, old, old+1)
	for !swapped {
		old = atomic.LoadInt32(&c.v)
		swapped = atomic.CompareAndSwapInt32(&c.v, old, old+1)
	}
}

func (c *counter) value() int32 {
	return atomic.LoadInt32(&c.v)
}
