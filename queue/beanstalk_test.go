// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"bytes"
	"encoding/gob"
	"github.com/globocom/config"
	. "launchpad.net/gocheck"
	"time"
)

type BeanstalkSuite struct{}

var _ = Suite(&BeanstalkSuite{})

func (s *BeanstalkSuite) SetUpSuite(c *C) {
	config.Set("queue-server", "127.0.0.1:11300")
	// Cleaning the queue. All tests must clean their mess, but we can't
	// guarante the state of the queue before running them.
	cleanQ(c)
}

func (s *BeanstalkSuite) SetUpTest(c *C) {
	conn = nil
}

func (s *BeanstalkSuite) TestConnection(c *C) {
	cn, err := connection()
	c.Assert(err, IsNil)
	defer cn.Close()
	tubes, err := cn.ListTubes()
	c.Assert(err, IsNil)
	c.Assert(tubes[0], Equals, "default")
}

func (s *BeanstalkSuite) TestConnectionQueueServerUndefined(c *C) {
	old, _ := config.Get("queue-server")
	config.Unset("queue-server")
	defer config.Set("queue-server", old)
	conn, err := connection()
	c.Assert(err, IsNil)
	c.Assert(conn, NotNil)
}

func (s *BeanstalkSuite) TestConnectionResfused(c *C) {
	old, _ := config.Get("queue-server")
	config.Set("queue-server", "127.0.0.1:11301")
	defer config.Set("queue-server", old)
	conn, err := connection()
	c.Assert(conn, IsNil)
	c.Assert(err, NotNil)
}

func (s *BeanstalkSuite) TestConnectionDoubleCall(c *C) {
	cn1, err := connection()
	c.Assert(err, IsNil)
	defer cn1.Close()
	c.Assert(cn1, Equals, conn)
	cn2, err := connection()
	c.Assert(err, IsNil)
	c.Assert(cn2, Equals, cn1)
}

func (s *BeanstalkSuite) TestConnectionClosed(c *C) {
	cn1, err := connection()
	c.Assert(err, IsNil)
	cn1.Close()
	cn2, err := connection()
	c.Assert(err, IsNil)
	defer cn2.Close()
	_, err = cn2.ListTubes()
	c.Assert(err, IsNil)
}

func (s *BeanstalkSuite) TestPut(c *C) {
	msg := Message{
		Action: "regenerate-apprc",
		Args:   []string{"myapp"},
	}
	q := beanstalkQ{name: "default"}
	err := q.Put(&msg, 0)
	c.Assert(err, IsNil)
	c.Assert(msg.id, Not(Equals), 0)
	defer conn.Delete(msg.id)
	id, body, err := conn.Reserve(1e6)
	c.Assert(err, IsNil)
	c.Assert(id, Equals, msg.id)
	var got Message
	buf := bytes.NewBuffer(body)
	err = gob.NewDecoder(buf).Decode(&got)
	c.Assert(err, IsNil)
	got.id = msg.id
	c.Assert(got, DeepEquals, msg)
}

func (s *BeanstalkSuite) TestPutWithDelay(c *C) {
	msg := Message{
		Action: "do-something",
		Args:   []string{"nothing"},
	}
	q := beanstalkQ{name: "default"}
	err := q.Put(&msg, 1e9)
	c.Assert(err, IsNil)
	defer conn.Delete(msg.id)
	_, _, err = conn.Reserve(1e6)
	c.Assert(err, NotNil)
	time.Sleep(1e9)
	id, _, err := conn.Reserve(1e6)
	c.Assert(err, IsNil)
	c.Assert(id, Equals, msg.id)
}

func (s *BeanstalkSuite) TestPutAndGetFromSpecificQueue(c *C) {
	msg := Message{
		Action: "do-something",
		Args:   []string{"everything"},
	}
	q := beanstalkQ{name: "here"}
	err := q.Put(&msg, 0)
	c.Assert(err, IsNil)
	defer q.Delete(&msg)
	dQ := beanstalkQ{name: "default"}
	_, err = dQ.Get(1e6)
	c.Assert(err, NotNil)
	got, err := q.Get(1e6)
	c.Assert(err, IsNil)
	c.Assert(got.Action, Equals, "do-something")
	c.Assert(got.Args, DeepEquals, []string{"everything"})
}

func (s *BeanstalkSuite) TestGet(c *C) {
	msg := Message{
		Action: "regenerate-apprc",
		Args:   []string{"myapprc"},
	}
	q := beanstalkQ{name: "default"}
	err := q.Put(&msg, 0)
	c.Assert(err, IsNil)
	defer conn.Delete(msg.id)
	got, err := q.Get(1e6)
	c.Assert(err, IsNil)
	c.Assert(*got, DeepEquals, msg)
}

func (s *BeanstalkSuite) TestGetConnectionError(c *C) {
	old, _ := config.Get("queue-server")
	defer config.Set("queue-server", old)
	config.Unset("queue-server")
	q := beanstalkQ{name: "default"}
	msg, err := q.Get(1e6)
	c.Assert(msg, IsNil)
	c.Assert(err, NotNil)
}

func (s *BeanstalkSuite) TestGetFromEmptyQueue(c *C) {
	q := beanstalkQ{name: "default"}
	msg, err := q.Get(1e6)
	c.Assert(msg, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Timed out waiting for message after 1ms.")
}

func (s *BeanstalkSuite) TestGetInvalidMessage(c *C) {
	conn, err := connection()
	c.Assert(err, IsNil)
	id, err := conn.Put([]byte("hello world"), 1, 0, 10e9)
	defer conn.Delete(id) // sanity
	q := beanstalkQ{name: "default"}
	msg, err := q.Get(1e6)
	c.Assert(msg, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, `Invalid message: "hello world"`)
	_, _, err = conn.Reserve(1e6)
	c.Assert(err, NotNil)
	c.Assert(timeoutRegexp.MatchString(err.Error()), Equals, true)
}

func (s *BeanstalkSuite) TestRelease(c *C) {
	conn, err := connection()
	c.Assert(err, IsNil)
	msg := Message{Action: "do-something"}
	q := beanstalkQ{name: "default"}
	err = q.Put(&msg, 0)
	c.Assert(err, IsNil)
	defer q.Delete(&msg)
	copy, err := q.Get(1e6)
	c.Assert(err, IsNil)
	err = q.Release(&msg, 0)
	c.Assert(err, IsNil)
	id, _, err := conn.Reserve(1e6)
	c.Assert(err, IsNil)
	c.Assert(id, Equals, copy.id)
}

func (s *BeanstalkSuite) TestReleaseWithDelay(c *C) {
	conn, err := connection()
	c.Assert(err, IsNil)
	msg := Message{Action: "do-something"}
	q := beanstalkQ{name: "default"}
	err = q.Put(&msg, 0)
	c.Assert(err, IsNil)
	defer q.Delete(&msg)
	copy, err := q.Get(1e6)
	c.Assert(err, IsNil)
	err = q.Release(&msg, 1e9)
	c.Assert(err, IsNil)
	_, _, err = conn.Reserve(1e6)
	c.Assert(err, NotNil)
	time.Sleep(1e9)
	id, _, err := conn.Reserve(1e6)
	c.Assert(err, IsNil)
	c.Assert(id, Equals, copy.id)
}

func (s *BeanstalkSuite) TestReleaseMessageWithoutId(c *C) {
	msg := Message{Action: "do-something"}
	q := beanstalkQ{name: "default"}
	err := q.Release(&msg, 0)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Unknown message.")
}

func (s *BeanstalkSuite) TestReleaseMessageNotFound(c *C) {
	msg := Message{Action: "do-otherthing", id: 12884}
	q := beanstalkQ{name: "default"}
	err := q.Release(&msg, 0)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Message not found.")
}

func (s *BeanstalkSuite) TestReleaseConnectionError(c *C) {
	old, _ := config.Get("queue-server")
	defer config.Set("queue-server", old)
	config.Unset("queue-server")
	q := beanstalkQ{name: "default"}
	err := q.Release(&Message{id: 1}, 0)
	c.Assert(err, NotNil)
}

func (s *BeanstalkSuite) TestDelete(c *C) {
	msg := Message{
		Action: "create-app",
		Args:   []string{"something"},
	}
	q := beanstalkQ{name: "default"}
	err := q.Put(&msg, 0)
	c.Assert(err, IsNil)
	defer conn.Delete(msg.id)
	err = q.Delete(&msg)
	c.Assert(err, IsNil)
}

func (s *BeanstalkSuite) TestDeleteConnectionError(c *C) {
	old, _ := config.Get("queue-server")
	defer config.Set("queue-server", old)
	config.Unset("queue-server")
	q := beanstalkQ{name: "default"}
	err := q.Delete(&Message{})
	c.Assert(err, NotNil)
}

func (s *BeanstalkSuite) TestDeleteUnknownMessage(c *C) {
	msg := Message{
		Action: "create-app",
		Args:   []string{"something"},
		id:     837826742,
	}
	q := beanstalkQ{name: "default"}
	err := q.Delete(&msg)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Message not found.")
}

func (s *BeanstalkSuite) TestDeleteMessageWithoutId(c *C) {
	msg := Message{
		Action: "create-app",
		Args:   []string{"something"},
	}
	q := beanstalkQ{name: "default"}
	err := q.Delete(&msg)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Unknown message.")
}

func cleanQ(c *C) {
	cn, err := connection()
	c.Assert(err, IsNil)
	var id uint64
	for err == nil {
		if id, _, err = cn.Reserve(1e6); err == nil {
			err = cn.Delete(id)
		}
	}
}
