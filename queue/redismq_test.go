// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"github.com/adeven/redismq"
	"launchpad.net/gocheck"
	"time"
)

type RedismqSuite struct {
	queue    *redismq.Queue
	consumer *redismq.Consumer
}

var _ = gocheck.Suite(&RedismqSuite{})

func (s *RedismqSuite) SetUpSuite(c *gocheck.C) {
	var err error
	s.queue = redismq.CreateQueue("localhost", "6379", "", 3, "redismq_tests")
	s.consumer, err = s.queue.AddConsumer("redismq_tests")
	c.Assert(err, gocheck.IsNil)
}

func (s *RedismqSuite) TearDownSuite(c *gocheck.C) {
	s.queue.Delete()
}

func (s *RedismqSuite) TestPut(c *gocheck.C) {
	msg := Message{
		Action: "regenerate-apprc",
		Args:   []string{"myapp"},
	}
	q := redismqQ{name: "default", queue: s.queue, consumer: s.consumer}
	err := q.Put(&msg, 0)
	c.Assert(err, gocheck.IsNil)
	c.Assert(msg.id, gocheck.Not(gocheck.Equals), 0)
	got, err := q.Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(*got, gocheck.DeepEquals, msg)
}

func (s *RedismqSuite) TestPutWithDelay(c *gocheck.C) {
	msg := Message{
		Action: "regenerate-apprc",
		Args:   []string{"myapp"},
	}
	q := redismqQ{name: "default", queue: s.queue, consumer: s.consumer}
	err := q.Put(&msg, 1e9)
	c.Assert(err, gocheck.IsNil)
	_, err = q.Get(1e6)
	c.Assert(err, gocheck.NotNil)
	time.Sleep(15e8)
	got, err := q.Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(*got, gocheck.DeepEquals, msg)
}

func (s *RedismqSuite) TestGet(c *gocheck.C) {
	msg := Message{
		Action: "regenerate-apprc",
		Args:   []string{"myapp"},
	}
	q := redismqQ{name: "default", queue: s.queue, consumer: s.consumer}
	err := q.Put(&msg, 0)
	c.Assert(err, gocheck.IsNil)
	got, err := q.Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(*got, gocheck.DeepEquals, msg)
}

func (s *RedismqSuite) TestGetTimeout(c *gocheck.C) {
	q := redismqQ{name: "default", queue: s.queue, consumer: s.consumer}
	got, err := q.Get(1e6)
	c.Assert(err, gocheck.NotNil)
	c.Assert(got, gocheck.IsNil)
	e, ok := err.(*timeoutError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.timeout, gocheck.Equals, time.Duration(1e6))
}

func (s *RedismqSuite) TestRelease(c *gocheck.C) {
	msg := Message{
		Action: "regenerate-apprc",
		Args:   []string{"myapp"},
	}
	q := redismqQ{name: "default", queue: s.queue, consumer: s.consumer}
	err := q.Release(&msg, 0)
	c.Assert(err, gocheck.IsNil)
	c.Assert(msg.id, gocheck.Not(gocheck.Equals), 0)
	got, err := q.Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(*got, gocheck.DeepEquals, msg)
}
