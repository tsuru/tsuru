// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"github.com/garyburd/redigo/redis"
	"github.com/tsuru/config"
	"launchpad.net/gocheck"
	"sync/atomic"
	"time"
)

type RedismqSuite struct {
	pool *redis.Pool
}

var _ = gocheck.Suite(&RedismqSuite{})

func (s *RedismqSuite) SetUpSuite(c *gocheck.C) {
	s.pool = redis.NewPool(func() (redis.Conn, error) {
		return redis.Dial("tcp", "127.0.0.1:6379")
	}, 10)
	config.Set("queue", "redis")
}

func (s *RedismqSuite) TearDownSuite(c *gocheck.C) {
	config.Unset("queue")
}

func (s *RedismqSuite) TestPut(c *gocheck.C) {
	msg := Message{
		Action: "regenerate-apprc",
		Args:   []string{"myapp"},
	}
	q := redismqQ{name: "default", pool: s.pool, prefix: "test", maxSize: 10}
	err := q.Put(&msg, 0)
	c.Assert(err, gocheck.IsNil)
	got, err := q.Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(*got, gocheck.DeepEquals, msg)
}

func (s *RedismqSuite) TestPutWithDelay(c *gocheck.C) {
	msg := Message{
		Action: "regenerate-apprc",
		Args:   []string{"myapp"},
	}
	q := redismqQ{name: "default", pool: s.pool, prefix: "tests", maxSize: 10}
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
	q := redismqQ{name: "default", pool: s.pool, prefix: "tests", maxSize: 10}
	err := q.Put(&msg, 0)
	c.Assert(err, gocheck.IsNil)
	got, err := q.Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(*got, gocheck.DeepEquals, msg)
}

func (s *RedismqSuite) TestGetTimeout(c *gocheck.C) {
	q := redismqQ{name: "default", pool: s.pool, prefix: "tests", maxSize: 10}
	got, err := q.Get(1e6)
	c.Assert(err, gocheck.NotNil)
	c.Assert(got, gocheck.IsNil)
	e, ok := err.(*timeoutError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.timeout, gocheck.Equals, time.Duration(1e6))
}

func (s *RedismqSuite) TestPutAndGetMaxSize(c *gocheck.C) {
	msg1 := Message{Action: "regenerate-apprc", Args: []string{"myapp"}}
	msg2 := Message{Action: "regenerate-apprc", Args: []string{"yourapp"}}
	msg3 := Message{Action: "regenerate-apprc", Args: []string{"hisapp"}}
	msg4 := Message{Action: "regenerate-apprc", Args: []string{"herapp"}}
	q := redismqQ{name: "default", pool: s.pool, prefix: "tests", maxSize: 3}
	err := q.Put(&msg1, 0)
	c.Assert(err, gocheck.IsNil)
	err = q.Put(&msg2, 0)
	c.Assert(err, gocheck.IsNil)
	err = q.Put(&msg3, 0)
	c.Assert(err, gocheck.IsNil)
	err = q.Put(&msg4, 0)
	c.Assert(err, gocheck.IsNil)
	msgs := make([]Message, 3)
	for i := range msgs {
		msg, err := q.Get(1e6)
		c.Check(err, gocheck.IsNil)
		msgs[i] = *msg
	}
	expected := []Message{msg2, msg3, msg4}
	c.Assert(msgs, gocheck.DeepEquals, expected)
}

func (s *RedismqSuite) TestFactoryGet(c *gocheck.C) {
	var factory redismqQFactory
	q, err := factory.Get("ancient")
	c.Assert(err, gocheck.IsNil)
	rq, ok := q.(*redismqQ)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(rq.name, gocheck.Equals, "ancient")
	msg := Message{Action: "wat", Args: []string{"a", "b"}}
	err = rq.Put(&msg, 0)
	c.Assert(err, gocheck.IsNil)
	got, err := rq.Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(*got, gocheck.DeepEquals, msg)
}

func (s *RedismqSuite) TestRedismqFactoryHandler(c *gocheck.C) {
	var factory redismqQFactory
	q, err := factory.Get("civil")
	c.Assert(err, gocheck.IsNil)
	msg := Message{
		Action: "create-app",
		Args:   []string{"something"},
	}
	q.Put(&msg, 0)
	var called int32
	var dumb = func(m *Message) {
		atomic.StoreInt32(&called, 1)
		c.Assert(m.Action, gocheck.Equals, msg.Action)
		c.Assert(m.Args, gocheck.DeepEquals, msg.Args)
	}
	handler, err := factory.Handler(dumb, "civil")
	c.Assert(err, gocheck.IsNil)
	exec, ok := handler.(*executor)
	c.Assert(ok, gocheck.Equals, true)
	exec.inner()
	time.Sleep(1e3)
	c.Assert(atomic.LoadInt32(&called), gocheck.Equals, int32(1))
	_, err = q.Get(1e6)
	c.Assert(err, gocheck.NotNil)
}

func (s *RedismqSuite) TestRedismqFactoryPutMessageBackOnFailure(c *gocheck.C) {
	var factory redismqQFactory
	q, err := factory.Get("wheels")
	c.Assert(err, gocheck.IsNil)
	msg := Message{Action: "create-app"}
	q.Put(&msg, 0)
	var dumb = func(m *Message) {
		m.Fail()
		time.Sleep(1e3)
	}
	handler, err := factory.Handler(dumb, "wheels")
	c.Assert(err, gocheck.IsNil)
	handler.(*executor).inner()
	time.Sleep(1e6)
	_, err = q.Get(1e6)
	c.Assert(err, gocheck.IsNil)
}

func (s *RedismqSuite) TestRedisMqFactoryIsInFactoriesMap(c *gocheck.C) {
	f, ok := factories["redis"]
	c.Assert(ok, gocheck.Equals, true)
	_, ok = f.(redismqQFactory)
	c.Assert(ok, gocheck.Equals, true)
}
