// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queuetest

import (
	"testing"
	"time"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type S struct{}

var _ = check.Suite(&S{})

func (s *S) TestFakeQPubSub(c *check.C) {
	q := FakePubSubQ{}
	msgChan, err := q.Sub()
	c.Assert(err, check.IsNil)
	err = q.Pub([]byte("muad'dib"))
	c.Assert(err, check.IsNil)
	c.Assert(<-msgChan, check.DeepEquals, []byte("muad'dib"))
}

func (s *S) TestFakeQPubSubUnSub(c *check.C) {
	q := FakePubSubQ{}
	msgChan, err := q.Sub()
	c.Assert(err, check.IsNil)
	err = q.Pub([]byte("arrakis"))
	c.Assert(err, check.IsNil)
	done := make(chan bool)
	go func() {
		time.Sleep(5e8)
		q.UnSub()
	}()
	go func() {
		msgs := make([][]byte, 0)
		for msg := range msgChan {
			msgs = append(msgs, msg)
		}
		c.Assert(msgs, check.DeepEquals, [][]byte{[]byte("arrakis")})
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(1e9):
		c.Error("Timeout waiting for message.")
	}
}

func (s *S) TestFakeQFactoryGet(c *check.C) {
	f := NewFakePubSubQFactory()
	q, err := f.PubSub("default")
	c.Assert(err, check.IsNil)
	_, ok := q.(*FakePubSubQ)
	c.Assert(ok, check.Equals, true)
	q2, err := f.PubSub("default")
	c.Assert(err, check.IsNil)
	c.Assert(q, check.Equals, q2)
	q3, err := f.PubSub("non-default")
	c.Assert(err, check.IsNil)
	c.Assert(q, check.Not(check.Equals), q3)
}
