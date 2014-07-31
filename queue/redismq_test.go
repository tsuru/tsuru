// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"github.com/tsuru/config"
	"launchpad.net/gocheck"
	"time"
)

type RedismqSuite struct {
	factory *redismqQFactory
}

var _ = gocheck.Suite(&RedismqSuite{})

func (s *RedismqSuite) SetUpSuite(c *gocheck.C) {
	s.factory = &redismqQFactory{}
	config.Set("queue", "redis")
	q := redismqQ{name: "default", factory: s.factory, prefix: "test"}
	conn, err := s.factory.getConn()
	c.Assert(err, gocheck.IsNil)
	conn.Do("DEL", q.key())
}

func (s *RedismqSuite) TearDownSuite(c *gocheck.C) {
	config.Unset("queue")
}

func (s *RedismqSuite) TestFactoryGetPool(c *gocheck.C) {
	var factory redismqQFactory
	pool := factory.getPool()
	c.Assert(pool.IdleTimeout, gocheck.Equals, 5*time.Minute)
	c.Assert(pool.MaxIdle, gocheck.Equals, 20)
}

func (s *RedismqSuite) TestFactoryGet(c *gocheck.C) {
	var factory redismqQFactory
	q, err := factory.Get("ancient")
	c.Assert(err, gocheck.IsNil)
	rq, ok := q.(*redismqQ)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(rq.name, gocheck.Equals, "ancient")
}

func (s *RedismqSuite) TestRedisMqFactoryIsInFactoriesMap(c *gocheck.C) {
	f, ok := factories["redis"]
	c.Assert(ok, gocheck.Equals, true)
	_, ok = f.(*redismqQFactory)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *RedismqSuite) TestRedisPubSub(c *gocheck.C) {
	var factory redismqQFactory
	q, err := factory.Get("mypubsub")
	c.Assert(err, gocheck.IsNil)
	pubSubQ, ok := q.(PubSubQ)
	c.Assert(ok, gocheck.Equals, true)
	msgChan, err := pubSubQ.Sub()
	c.Assert(err, gocheck.IsNil)
	err = pubSubQ.Pub([]byte("entil'zha"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(<-msgChan, gocheck.DeepEquals, []byte("entil'zha"))
}

func (s *RedismqSuite) TestRedisPubSubUnsub(c *gocheck.C) {
	var factory redismqQFactory
	q, err := factory.Get("mypubsub")
	c.Assert(err, gocheck.IsNil)
	pubSubQ, ok := q.(PubSubQ)
	c.Assert(ok, gocheck.Equals, true)
	msgChan, err := pubSubQ.Sub()
	c.Assert(err, gocheck.IsNil)
	err = pubSubQ.Pub([]byte("anla'shok"))
	c.Assert(err, gocheck.IsNil)
	done := make(chan bool)
	go func() {
		time.Sleep(5e8)
		pubSubQ.UnSub()
	}()
	go func() {
		msgs := make([][]byte, 0)
		for msg := range msgChan {
			msgs = append(msgs, msg)
		}
		c.Assert(msgs, gocheck.DeepEquals, [][]byte{[]byte("anla'shok")})
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(1e9):
		c.Error("Timeout waiting for message.")
	}
}
