// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package shutdown

import (
	"testing"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

type testShutdown struct {
	calls int
}

func (t *testShutdown) Shutdown() {
	t.calls++
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

func (s *S) TestAll(c *check.C) {
	ts := &testShutdown{}
	Register(ts)
	values := All()
	c.Assert(values, check.HasLen, 1)
	values[0].Shutdown()
	c.Assert(ts.calls, check.Equals, 1)
}
