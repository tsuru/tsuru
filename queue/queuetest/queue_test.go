// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queuetest

import (
	"testing"
	"time"

	"launchpad.net/gocheck"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) TestFakeQPubSub(c *gocheck.C) {
	q := FakePubSubQ{}
	msgChan, err := q.Sub()
	c.Assert(err, gocheck.IsNil)
	err = q.Pub([]byte("muad'dib"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(<-msgChan, gocheck.DeepEquals, []byte("muad'dib"))
}

func (s *S) TestFakeQPubSubUnSub(c *gocheck.C) {
	q := FakePubSubQ{}
	msgChan, err := q.Sub()
	c.Assert(err, gocheck.IsNil)
	err = q.Pub([]byte("arrakis"))
	c.Assert(err, gocheck.IsNil)
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
		c.Assert(msgs, gocheck.DeepEquals, [][]byte{[]byte("arrakis")})
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(1e9):
		c.Error("Timeout waiting for message.")
	}
}

func (s *S) TestFakeQFactoryGet(c *gocheck.C) {
	f := NewFakePubSubQFactory()
	q, err := f.Get("default")
	c.Assert(err, gocheck.IsNil)
	_, ok := q.(*FakePubSubQ)
	c.Assert(ok, gocheck.Equals, true)
	q2, err := f.Get("default")
	c.Assert(err, gocheck.IsNil)
	c.Assert(q, gocheck.Equals, q2)
	q3, err := f.Get("non-default")
	c.Assert(err, gocheck.IsNil)
	c.Assert(q, gocheck.Not(gocheck.Equals), q3)
}
