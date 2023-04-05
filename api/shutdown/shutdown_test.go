// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package shutdown

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

type testShutdown struct {
	sleep time.Duration
	calls int32
}

func (t *testShutdown) Shutdown(ctx context.Context) error {
	atomic.AddInt32(&t.calls, 1)
	time.Sleep(t.sleep)
	return nil
}

func (s *S) SetUpTest(c *check.C) {
	registered = nil
}

func (s *S) TestRegister(c *check.C) {
	ts := &testShutdown{}
	Register(ts)
	c.Assert(registered, check.HasLen, 1)
	c.Assert(atomic.LoadInt32(&ts.calls), check.Equals, int32(0))
}

func (s *S) TestDo(c *check.C) {
	ts := &testShutdown{}
	ts2 := &testShutdown{}
	Register(ts)
	Register(ts2)
	err := Do(context.Background(), io.Discard)
	c.Assert(err, check.IsNil)
	c.Assert(atomic.LoadInt32(&ts.calls), check.Equals, int32(1))
	c.Assert(atomic.LoadInt32(&ts2.calls), check.Equals, int32(1))
}

func (s *S) TestDoTimeout(c *check.C) {
	ts := &testShutdown{
		sleep: time.Duration(2) * time.Second,
	}
	Register(ts)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	err := Do(ctx, io.Discard)
	cancel()
	c.Assert(err, check.DeepEquals, context.DeadlineExceeded)
	c.Assert(atomic.LoadInt32(&ts.calls), check.Equals, int32(1))
}
