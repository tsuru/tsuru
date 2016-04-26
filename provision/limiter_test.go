// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"runtime"
	"sync"
	"time"

	"gopkg.in/check.v1"
)

type LimiterSuite struct {
	limiter ActionLimiter
}

func init() {
	check.Suite(&LimiterSuite{
		limiter: &LocalLimiter{},
	})
}

func (s *LimiterSuite) TestLocalLimiterAddDone(c *check.C) {
	l := s.limiter
	l.SetLimit(3)
	l.Add("node1")
	l.Add("node1")
	l.Add("node1")
	c.Assert(l.Len("node1"), check.Equals, 3)
	c.Assert(l.Len("node2"), check.Equals, 0)
	done := make(chan bool)
	go func() {
		l.Add("node1")
		close(done)
	}()
	select {
	case <-done:
		c.Fatal("add should have blocked")
	case <-time.After(100 * time.Millisecond):
	}
	l.Add("node2")
	c.Assert(l.Len("node2"), check.Equals, 1)
	l.Done("node1")
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		c.Fatal("timed out waiting for unblock")
	}
	c.Assert(l.Len("node1"), check.Equals, 3)
}

func (s *LimiterSuite) TestLocalLimiterAddDoneRace(c *check.C) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(10))
	l := s.limiter
	l.SetLimit(100)
	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Add("n1")
		}()
	}
	wg.Wait()
	c.Assert(l.Len("n1"), check.Equals, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Done("n1")
		}()
	}
	wg.Wait()
	c.Assert(l.Len("n1"), check.Equals, 0)
}

func (s *LimiterSuite) TestLocalLimiterAddDoneRace2(c *check.C) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(10))
	l := s.limiter
	l.SetLimit(100)
	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			l.Add("n1")
		}()
		go func() {
			defer wg.Done()
			l.Done("n1")
		}()
	}
	wg.Wait()
	c.Assert(l.Len("n1"), check.Equals, 0)
}

func (s *LimiterSuite) TestLocalLimiterAddDoneZeroLimit(c *check.C) {
	l := s.limiter
	l.SetLimit(0)
	for i := 0; i < 100; i++ {
		l.Add("n1")
	}
	c.Assert(l.Len("n1"), check.Equals, 0)
	for i := 0; i < 100; i++ {
		l.Done("n1")
	}
	c.Assert(l.Len("n1"), check.Equals, 0)
}
