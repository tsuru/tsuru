// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"github.com/tsuru/tsuru/queue"
	"launchpad.net/gocheck"
)

func (s *S) TestFakeQPutAndGet(c *gocheck.C) {
	q := FakeQ{}
	msg := queue.Message{Action: "do-something"}
	err := q.Put(&msg, 0)
	c.Assert(err, gocheck.IsNil)
	m, err := q.Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(m.Action, gocheck.Equals, msg.Action)
}

func (s *S) TestFakeQPutAndGetMultipleMessages(c *gocheck.C) {
	q := FakeQ{}
	messages := []queue.Message{
		{Action: "do-something"},
		{Action: "do-otherthing"},
		{Action: "do-all-things"},
		{Action: "do-anything"},
	}
	for _, m := range messages {
		copy := m
		q.Put(&copy, 0)
	}
	got := make([]queue.Message, len(messages))
	for i := range got {
		msg, err := q.Get(1e6)
		c.Check(err, gocheck.IsNil)
		got[i] = *msg
	}
	c.Assert(got, gocheck.DeepEquals, messages)
}

func (s *S) TestFakeQGetTimeout(c *gocheck.C) {
	q := FakeQ{}
	m, err := q.Get(1e6)
	c.Assert(m, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Timed out.")
}

func (s *S) TestFakeQPutWithTimeout(c *gocheck.C) {
	q := FakeQ{}
	msg := queue.Message{Action: "do-something"}
	err := q.Put(&msg, 1e6)
	c.Assert(err, gocheck.IsNil)
	_, err = q.Get(1e3)
	c.Assert(err, gocheck.NotNil)
	_, err = q.Get(1e9)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestFakeHandlerStart(c *gocheck.C) {
	h := fakeHandler{}
	c.Assert(h.running, gocheck.Equals, int32(0))
	h.Start()
	c.Assert(h.running, gocheck.Equals, int32(1))
}

func (s *S) TestFakeHandlerStop(c *gocheck.C) {
	h := fakeHandler{}
	h.Start()
	h.Stop()
	c.Assert(h.running, gocheck.Equals, int32(0))
}

func (s *S) TestFakeQFactoryGet(c *gocheck.C) {
	f := NewFakeQFactory()
	q, err := f.Get("default")
	c.Assert(err, gocheck.IsNil)
	_, ok := q.(*FakeQ)
	c.Assert(ok, gocheck.Equals, true)
	q2, err := f.Get("default")
	c.Assert(err, gocheck.IsNil)
	c.Assert(q, gocheck.Equals, q2)
	q3, err := f.Get("non-default")
	c.Assert(err, gocheck.IsNil)
	c.Assert(q, gocheck.Not(gocheck.Equals), q3)
}

func (s *S) TestFakeQFactoryHandler(c *gocheck.C) {
	f := NewFakeQFactory()
	h, err := f.Handler(nil)
	c.Assert(err, gocheck.IsNil)
	_, ok := h.(*fakeHandler)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestCleanQ(c *gocheck.C) {
	msg := queue.Message{Action: "do-something", Args: []string{"wat"}}
	q, err := factory.Get("firedance")
	c.Assert(err, gocheck.IsNil)
	err = q.Put(&msg, 0)
	c.Assert(err, gocheck.IsNil)
	q2, err := factory.Get("hush")
	c.Assert(err, gocheck.IsNil)
	err = q2.Put(&msg, 0)
	c.Assert(err, gocheck.IsNil)
	q3, err := factory.Get("rocket")
	c.Assert(err, gocheck.IsNil)
	err = q3.Put(&msg, 0)
	c.Assert(err, gocheck.IsNil)
	CleanQ("firedance", "hush")
	_, err = q.Get(1e6)
	c.Assert(err, gocheck.NotNil)
	_, err = q2.Get(1e6)
	c.Assert(err, gocheck.NotNil)
	_, err = q3.Get(1e6)
	c.Assert(err, gocheck.IsNil)
}
