// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"bytes"
	"encoding/gob"
	"github.com/globocom/config"
	"launchpad.net/gocheck"
	"sync/atomic"
	"time"
)

type BeanstalkSuite struct{}

var _ = gocheck.Suite(&BeanstalkSuite{})

func (s *BeanstalkSuite) SetUpSuite(c *gocheck.C) {
	config.Set("queue-server", "127.0.0.1:11300")
	// Cleaning the queue. All tests must clean their mess, but we can't
	// guarante the state of the queue before running them.
	cleanQ(c)
}

func (s *BeanstalkSuite) SetUpTest(c *gocheck.C) {
	conn = nil
}

func (s *BeanstalkSuite) TestConnection(c *gocheck.C) {
	cn, err := connection()
	c.Assert(err, gocheck.IsNil)
	defer cn.Close()
	tubes, err := cn.ListTubes()
	c.Assert(err, gocheck.IsNil)
	c.Assert(tubes[0], gocheck.Equals, "default")
}

func (s *BeanstalkSuite) TestConnectionQueueServerUndefined(c *gocheck.C) {
	old, _ := config.Get("queue-server")
	config.Unset("queue-server")
	defer config.Set("queue-server", old)
	conn, err := connection()
	c.Assert(err, gocheck.IsNil)
	c.Assert(conn, gocheck.NotNil)
}

func (s *BeanstalkSuite) TestConnectionResfused(c *gocheck.C) {
	old, _ := config.Get("queue-server")
	config.Set("queue-server", "127.0.0.1:11301")
	defer config.Set("queue-server", old)
	conn, err := connection()
	c.Assert(conn, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
}

func (s *BeanstalkSuite) TestConnectionDoubleCall(c *gocheck.C) {
	cn1, err := connection()
	c.Assert(err, gocheck.IsNil)
	defer cn1.Close()
	c.Assert(cn1, gocheck.Equals, conn)
	cn2, err := connection()
	c.Assert(err, gocheck.IsNil)
	c.Assert(cn2, gocheck.Equals, cn1)
}

func (s *BeanstalkSuite) TestConnectionClosed(c *gocheck.C) {
	cn1, err := connection()
	c.Assert(err, gocheck.IsNil)
	cn1.Close()
	cn2, err := connection()
	c.Assert(err, gocheck.IsNil)
	defer cn2.Close()
	_, err = cn2.ListTubes()
	c.Assert(err, gocheck.IsNil)
}

func (s *BeanstalkSuite) TestPut(c *gocheck.C) {
	msg := Message{
		Action: "regenerate-apprc",
		Args:   []string{"myapp"},
	}
	q := beanstalkdQ{name: "default"}
	err := q.Put(&msg, 0)
	c.Assert(err, gocheck.IsNil)
	c.Assert(msg.id, gocheck.Not(gocheck.Equals), 0)
	defer conn.Delete(msg.id)
	id, body, err := conn.Reserve(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(id, gocheck.Equals, msg.id)
	var got Message
	buf := bytes.NewBuffer(body)
	err = gob.NewDecoder(buf).Decode(&got)
	c.Assert(err, gocheck.IsNil)
	got.id = msg.id
	c.Assert(got, gocheck.DeepEquals, msg)
}

func (s *BeanstalkSuite) TestPutWithDelay(c *gocheck.C) {
	msg := Message{
		Action: "do-something",
		Args:   []string{"nothing"},
	}
	q := beanstalkdQ{name: "default"}
	err := q.Put(&msg, 1e9)
	c.Assert(err, gocheck.IsNil)
	defer conn.Delete(msg.id)
	_, _, err = conn.Reserve(1e6)
	c.Assert(err, gocheck.NotNil)
	time.Sleep(1e9)
	id, _, err := conn.Reserve(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(id, gocheck.Equals, msg.id)
}

func (s *BeanstalkSuite) TestPutAndGetFromSpecificQueue(c *gocheck.C) {
	msg := Message{
		Action: "do-something",
		Args:   []string{"everything"},
	}
	q := beanstalkdQ{name: "here"}
	err := q.Put(&msg, 0)
	c.Assert(err, gocheck.IsNil)
	defer q.Delete(&msg)
	dQ := beanstalkdQ{name: "default"}
	_, err = dQ.Get(1e6)
	c.Assert(err, gocheck.NotNil)
	got, err := q.Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(got.Action, gocheck.Equals, "do-something")
	c.Assert(got.Args, gocheck.DeepEquals, []string{"everything"})
}

func (s *BeanstalkSuite) TestGet(c *gocheck.C) {
	msg := Message{
		Action: "regenerate-apprc",
		Args:   []string{"myapprc"},
	}
	q := beanstalkdQ{name: "default"}
	err := q.Put(&msg, 0)
	c.Assert(err, gocheck.IsNil)
	defer conn.Delete(msg.id)
	got, err := q.Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(*got, gocheck.DeepEquals, msg)
}

func (s *BeanstalkSuite) TestGetFromEmptyQueue(c *gocheck.C) {
	q := beanstalkdQ{name: "default"}
	msg, err := q.Get(1e6)
	c.Assert(msg, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Timed out waiting for message after 1ms.")
}

func (s *BeanstalkSuite) TestGetInvalidMessage(c *gocheck.C) {
	conn, err := connection()
	c.Assert(err, gocheck.IsNil)
	id, err := conn.Put([]byte("hello world"), 1, 0, 10e9)
	defer conn.Delete(id) // sanity
	q := beanstalkdQ{name: "default"}
	msg, err := q.Get(1e6)
	c.Assert(msg, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `Invalid message: "hello world"`)
	_, _, err = conn.Reserve(1e6)
	c.Assert(err, gocheck.NotNil)
	c.Assert(timeoutRegexp.MatchString(err.Error()), gocheck.Equals, true)
}

func (s *BeanstalkSuite) TestRelease(c *gocheck.C) {
	conn, err := connection()
	c.Assert(err, gocheck.IsNil)
	msg := Message{Action: "do-something"}
	q := beanstalkdQ{name: "default"}
	err = q.Put(&msg, 0)
	c.Assert(err, gocheck.IsNil)
	defer q.Delete(&msg)
	copy, err := q.Get(1e6)
	c.Assert(err, gocheck.IsNil)
	err = q.Release(&msg, 0)
	c.Assert(err, gocheck.IsNil)
	id, _, err := conn.Reserve(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(id, gocheck.Equals, copy.id)
}

func (s *BeanstalkSuite) TestReleaseWithDelay(c *gocheck.C) {
	conn, err := connection()
	c.Assert(err, gocheck.IsNil)
	msg := Message{Action: "do-something"}
	q := beanstalkdQ{name: "default"}
	err = q.Put(&msg, 0)
	c.Assert(err, gocheck.IsNil)
	defer q.Delete(&msg)
	copy, err := q.Get(1e6)
	c.Assert(err, gocheck.IsNil)
	err = q.Release(&msg, 1e9)
	c.Assert(err, gocheck.IsNil)
	_, _, err = conn.Reserve(1e6)
	c.Assert(err, gocheck.NotNil)
	time.Sleep(1e9)
	id, _, err := conn.Reserve(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(id, gocheck.Equals, copy.id)
}

func (s *BeanstalkSuite) TestReleaseMessageWithoutID(c *gocheck.C) {
	msg := Message{Action: "do-something"}
	q := beanstalkdQ{name: "default"}
	err := q.Release(&msg, 0)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Unknown message.")
}

func (s *BeanstalkSuite) TestReleaseMessageNotFound(c *gocheck.C) {
	msg := Message{Action: "do-otherthing", id: 12884}
	q := beanstalkdQ{name: "default"}
	err := q.Release(&msg, 0)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Message not found.")
}

func (s *BeanstalkSuite) TestDelete(c *gocheck.C) {
	msg := Message{
		Action: "create-app",
		Args:   []string{"something"},
	}
	q := beanstalkdQ{name: "default"}
	err := q.Put(&msg, 0)
	c.Assert(err, gocheck.IsNil)
	defer conn.Delete(msg.id)
	err = q.Delete(&msg)
	c.Assert(err, gocheck.IsNil)
}

func (s *BeanstalkSuite) TestDeleteUnknownMessage(c *gocheck.C) {
	msg := Message{
		Action: "create-app",
		Args:   []string{"something"},
		id:     837826742,
	}
	q := beanstalkdQ{name: "default"}
	err := q.Delete(&msg)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Message not found.")
}

func (s *BeanstalkSuite) TestDeleteMessageWithoutID(c *gocheck.C) {
	msg := Message{
		Action: "create-app",
		Args:   []string{"something"},
	}
	q := beanstalkdQ{name: "default"}
	err := q.Delete(&msg)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Unknown message.")
}

func (s *BeanstalkSuite) TestBeanstalkQSatisfiesQueue(c *gocheck.C) {
	var _ Q = &beanstalkdQ{}
}

func (s *BeanstalkSuite) TestBeanstalkFactoryGet(c *gocheck.C) {
	var factory beanstalkdFactory
	q, err := factory.Get("someq")
	c.Assert(err, gocheck.IsNil)
	bq, ok := q.(*beanstalkdQ)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(bq.name, gocheck.Equals, "someq")
}

func (s *BeanstalkSuite) TestBeanstalkFactoryHandler(c *gocheck.C) {
	msg := Message{
		Action: "create-app",
		Args:   []string{"something"},
		id:     837826742,
	}
	q := beanstalkdQ{name: "default"}
	q.Put(&msg, 0)
	defer q.Delete(&msg)
	var called int32
	var dumb = func(m *Message) {
		atomic.StoreInt32(&called, 1)
	}
	var factory beanstalkdFactory
	handler, err := factory.Handler(dumb, "default")
	c.Assert(err, gocheck.IsNil)
	exec, ok := handler.(*executor)
	c.Assert(ok, gocheck.Equals, true)
	exec.inner()
	time.Sleep(1e3)
	c.Assert(atomic.LoadInt32(&called), gocheck.Equals, int32(1))
}

func (s *BeanstalkSuite) TestBeanstalkFactoryHandlerDeleteMessage(c *gocheck.C) {
	var factory beanstalkdFactory
	msg := Message{
		Action: "create-app",
		Args:   []string{"something"},
		id:     837826742,
	}
	q := beanstalkdQ{name: "default"}
	q.Put(&msg, 0)
	defer q.Delete(&msg) // sanity
	handler, err := factory.Handler(func(m *Message) { m.Delete() }, "default")
	c.Assert(err, gocheck.IsNil)
	handler.(*executor).inner()
	time.Sleep(1e3)
	q.Release(&msg, 0) // sanity
	cn, err := connection()
	c.Assert(err, gocheck.IsNil)
	_, _, err = cn.Reserve(1e6)
	c.Assert(err, gocheck.NotNil)
}

func (s *BeanstalkSuite) TestBeanstalkFactoryHandlerReleaseMessage(c *gocheck.C) {
	var factory beanstalkdFactory
	msg := Message{
		Action: "create-app",
		Args:   []string{"something"},
		id:     837826742,
	}
	q := beanstalkdQ{name: "default"}
	q.Put(&msg, 0)
	defer q.Delete(&msg)
	handler, err := factory.Handler(func(m *Message) { time.Sleep(1e3) }, "default")
	c.Assert(err, gocheck.IsNil)
	handler.(*executor).inner()
	time.Sleep(1e6)
	cn, err := connection()
	c.Assert(err, gocheck.IsNil)
	id, _, err := cn.Reserve(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(id, gocheck.Equals, msg.id)
}

func (s *BeanstalkSuite) TestBeanstalkFactoryIsInFactoriesMap(c *gocheck.C) {
	f, ok := factories["beanstalkd"]
	c.Assert(ok, gocheck.Equals, true)
	_, ok = f.(beanstalkdFactory)
	c.Assert(ok, gocheck.Equals, true)
}

func cleanQ(c *gocheck.C) {
	cn, err := connection()
	c.Assert(err, gocheck.IsNil)
	var id uint64
	for err == nil {
		if id, _, err = cn.Reserve(1e6); err == nil {
			err = cn.Delete(id)
		}
	}
}
