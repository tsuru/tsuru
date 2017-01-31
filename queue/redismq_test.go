// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"time"

	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

type RedismqSuite struct {
	factory *redisPubSubFactory
}

var _ = check.Suite(&RedismqSuite{})

func (s *RedismqSuite) SetUpSuite(c *check.C) {
	config.Set("pubsub:redis-server", "127.0.0.1:6379")
	config.Set("pubsub:redis-db", 4)
	s.factory = &redisPubSubFactory{}
	q := redisPubSub{name: "default", factory: s.factory, prefix: "test"}
	conn, err := s.factory.getConn()
	c.Assert(err, check.IsNil)
	conn.Del(q.key())
}

func (s *RedismqSuite) TestFactoryGet(c *check.C) {
	var factory redisPubSubFactory
	q, err := factory.PubSub("ancient")
	c.Assert(err, check.IsNil)
	rq, ok := q.(*redisPubSub)
	c.Assert(ok, check.Equals, true)
	c.Assert(rq.name, check.Equals, "ancient")
}

func (s *RedismqSuite) TestRedisPubSub(c *check.C) {
	var factory redisPubSubFactory
	q, err := factory.PubSub("mypubsub")
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
	var factory redisPubSubFactory
	q, err := factory.PubSub("mypubsub")
	c.Assert(err, check.IsNil)
	pubSubQ, ok := q.(PubSubQ)
	c.Assert(ok, check.Equals, true)
	msgChan, err := pubSubQ.Sub()
	c.Assert(err, check.IsNil)
	err = pubSubQ.Pub([]byte("anla'shok"))
	c.Assert(err, check.IsNil)
	done := make(chan bool)
	doneUnsub := make(chan bool)
	shouldUnsub := make(chan bool)
	go func() {
		<-shouldUnsub
		pubSubQ.UnSub()
		doneUnsub <- true
	}()
	go func() {
		msgs := make([][]byte, 0)
		for msg := range msgChan {
			close(shouldUnsub)
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
	select {
	case <-doneUnsub:
	case <-time.After(1e9):
		c.Error("Timeout waiting for unsub.")
	}
}

func (s *RedismqSuite) TestRedisPubSubTimeout(c *check.C) {
	config.Set("pubsub:redis-read-timeout", 0.1)
	defer config.Unset("pubsub:redis-read-timeout")
	var factory redisPubSubFactory
	q, err := factory.PubSub("mypubsub")
	c.Assert(err, check.IsNil)
	pubSubQ, ok := q.(PubSubQ)
	c.Assert(ok, check.Equals, true)
	msgChan, err := pubSubQ.Sub()
	c.Assert(err, check.IsNil)
	time.Sleep(200 * time.Millisecond)
	err = pubSubQ.Pub([]byte("entil'zha"))
	c.Assert(err, check.IsNil)
	val := <-msgChan
	c.Assert(val, check.IsNil)
}
