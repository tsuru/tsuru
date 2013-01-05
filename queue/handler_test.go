// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"github.com/globocom/config"
	. "launchpad.net/gocheck"
	"strconv"
	"sync"
	"time"
)

type HandlerSuite struct {
	h *Handler
}

var _ = Suite(&HandlerSuite{})

var msgs MessageList

func dumbHandle(msg *Message) {
	msgs.Add(*msg)
	Delete(msg)
}

func (s *HandlerSuite) SetUpSuite(c *C) {
	s.h = &Handler{F: dumbHandle}
}

func (s *HandlerSuite) TestHandleMessages(c *C) {
	config.Set("queue-server", "127.0.0.1:11300")
	s.h.Start()
	c.Check(r.handlers, HasLen, 1)
	err := Put(&Message{Action: "do-something", Args: []string{"this"}})
	c.Check(err, IsNil)
	time.Sleep(1e9)
	s.h.Stop()
	c.Check(r.handlers, HasLen, 0)
	expected := []Message{
		{Action: "do-something", Args: []string{"this"}},
	}
	ms := msgs.Get()
	ms[0].id = 0
	c.Assert(ms, DeepEquals, expected)
	s.h.Wait()
}

func (s *HandlerSuite) TestPreempt(c *C) {
	config.Set("queue-server", "127.0.0.1:11300")
	var dumb = func(m *Message) {}
	h1 := Handler{F: dumb}
	h2 := Handler{F: dumb}
	h3 := Handler{F: dumb}
	h1.Start()
	h2.Start()
	h3.Start()
	Preempt()
	c.Assert(h1.state, Equals, stopped)
	c.Assert(h2.state, Equals, stopped)
	c.Assert(h3.state, Equals, stopped)
}

func (s *HandlerSuite) TestPreemptWithMessagesInTheQueue(c *C) {
	config.Set("queue-server", "127.0.0.1:11300")
	for i := 0; i < 100; i++ {
		Put(&Message{
			Action: "save-something",
			Args:   []string{strconv.Itoa(i)},
		})
	}
	var sleeper = func(m *Message) {
		Delete(m)
		time.Sleep(1e6)
	}
	h1 := Handler{F: sleeper}
	h1.Start()
	Preempt()
	c.Assert(h1.state, Equals, stopped)
	cleanQ(c)
}

func (s *HandlerSuite) TestStopNotRunningHandler(c *C) {
	h := Handler{F: nil}
	err := h.Stop()
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Not running.")
}

func (s *HandlerSuite) TestDryRun(c *C) {
	h := Handler{F: nil}
	err := h.DryRun()
	c.Assert(err, IsNil)
	c.Assert(h.state, Equals, running)
	h.Stop()
	h.Wait()
	c.Assert(h.state, Equals, stopped)
}

func (s *HandlerSuite) TestDryRunRunningHandler(c *C) {
	h := Handler{F: func(m *Message) { Delete(m) }}
	err := h.DryRun()
	c.Assert(err, IsNil)
	defer h.Stop()
	err = h.DryRun()
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Already running.")
}

type MessageList struct {
	sync.Mutex
	msgs []Message
}

func (l *MessageList) Add(m Message) {
	l.Lock()
	l.msgs = append(l.msgs, m)
	l.Unlock()
}

func (l *MessageList) Get() []Message {
	l.Lock()
	defer l.Unlock()
	return l.msgs
}
