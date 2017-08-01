// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package shutdown

import (
	"context"
	"io/ioutil"
	"testing"
	"time"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

type testShutdown struct {
	sleep time.Duration
	calls int
}

func (t *testShutdown) Shutdown(ctx context.Context) error {
	t.calls++
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
	c.Assert(ts.calls, check.Equals, 0)
}

func (s *S) TestDo(c *check.C) {
	ts := &testShutdown{}
	ts2 := &testShutdown{}
	Register(ts)
	Register(ts2)
	err := Do(context.Background(), ioutil.Discard)
	c.Assert(err, check.IsNil)
	c.Assert(ts.calls, check.Equals, 1)
	c.Assert(ts2.calls, check.Equals, 1)
}

func (s *S) TestDoTimeout(c *check.C) {
	ts := &testShutdown{
		sleep: time.Duration(2) * time.Second,
	}
	Register(ts)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	err := Do(ctx, ioutil.Discard)
	cancel()
	c.Assert(err, check.DeepEquals, context.DeadlineExceeded)
	c.Assert(ts.calls, check.Equals, 1)
}
