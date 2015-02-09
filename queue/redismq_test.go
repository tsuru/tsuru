// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"time"

	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

type RedismqSuite struct {
	factory *redismqQFactory
}

var _ = check.Suite(&RedismqSuite{})

func (s *RedismqSuite) SetUpSuite(c *check.C) {
	s.factory = &redismqQFactory{}
	config.Set("queue", "redis")
	q := redismqQ{name: "default", factory: s.factory, prefix: "test"}
	conn, err := s.factory.getConn()
	c.Assert(err, check.IsNil)
	conn.Do("DEL", q.key())
}

func (s *RedismqSuite) TearDownSuite(c *check.C) {
	config.Unset("queue")
}

func (s *RedismqSuite) TestFactoryGetPool(c *check.C) {
	var factory redismqQFactory
	pool := factory.getPool()
	c.Assert(pool.IdleTimeout, check.Equals, 5*time.Minute)
	c.Assert(pool.MaxIdle, check.Equals, 20)
}

func (s *RedismqSuite) TestFactoryGet(c *check.C) {
	var factory redismqQFactory
	q, err := factory.Get("ancient")
	c.Assert(err, check.IsNil)
	rq, ok := q.(*redismqQ)
	c.Assert(ok, check.Equals, true)
	c.Assert(rq.name, check.Equals, "ancient")
}

func (s *RedismqSuite) TestRedisMqFactoryIsInFactoriesMap(c *check.C) {
	f, ok := factories["redis"]
	c.Assert(ok, check.Equals, true)
	_, ok = f.(*redismqQFactory)
	c.Assert(ok, check.Equals, true)
}

func (s *RedismqSuite) TestRedisPubSub(c *check.C) {
	var factory redismqQFactory
	q, err := factory.Get("mypubsub")
	c.Assert(err, check.IsNil)
	pubSubQ, ok := q.(PubSubQ)
	c.Assert(ok, check.Equals, true)
	msgChan, err := pubSubQ.Sub()
	c.Assert(err, check.IsNil)
	err = pubSubQ.Pub([]byte("entil'zha"))
	c.Assert(err, check.IsNil)
	c.Assert(<-msgChan, check.DeepEquals, []byte("entil'zha"))
}

func (s *RedismqSuite) TestRedisPubSubUnsub(c *check.C) {
	var factory redismqQFactory
	q, err := factory.Get("mypubsub")
	c.Assert(err, check.IsNil)
	pubSubQ, ok := q.(PubSubQ)
	c.Assert(ok, check.Equals, true)
	msgChan, err := pubSubQ.Sub()
	c.Assert(err, check.IsNil)
	err = pubSubQ.Pub([]byte("anla'shok"))
	c.Assert(err, check.IsNil)
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
		c.Assert(msgs, check.DeepEquals, [][]byte{[]byte("anla'shok")})
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(1e9):
		c.Error("Timeout waiting for message.")
	}
}
