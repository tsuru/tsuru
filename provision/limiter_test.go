// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"runtime"
	"sync"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"gopkg.in/check.v1"
)

type LimiterSuite struct {
	limiter ActionLimiter
}

func init() {
	check.Suite(&LimiterSuite{
		limiter: &LocalLimiter{},
	})
	check.Suite(&LimiterSuite{
		limiter: &MongodbLimiter{},
	})
}

func (s *LimiterSuite) SetUpTest(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "provision_limiter_tests_s")
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = dbtest.ClearAllCollections(conn.Apps().Database)
	c.Assert(err, check.IsNil)
}

func (s *LimiterSuite) TestLocalLimiterAddDone(c *check.C) {
	l := s.limiter
	l.SetLimit(3)
	l.Start("node1")
	l.Start("node1")
	doneFunc := l.Start("node1")
	c.Assert(l.Len("node1"), check.Equals, 3)
	c.Assert(l.Len("node2"), check.Equals, 0)
	done := make(chan bool)
	go func() {
		l.Start("node1")
		close(done)
	}()
	select {
	case <-done:
		c.Fatal("add should have blocked")
	case <-time.After(100 * time.Millisecond):
	}
	c.Assert(l.Len("node1"), check.Equals, 3)
	l.Start("node2")
	c.Assert(l.Len("node2"), check.Equals, 1)
	doneFunc()
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
	doneCh := make(chan func(), 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			doneCh <- l.Start("n1")
		}()
	}
	wg.Wait()
	close(doneCh)
	c.Assert(l.Len("n1"), check.Equals, 100)
	for f := range doneCh {
		wg.Add(1)
		go func(f func()) {
			defer wg.Done()
			f()
		}(f)
	}
	wg.Wait()
	c.Assert(l.Len("n1"), check.Equals, 0)
}

func (s *LimiterSuite) TestLocalLimiterAddDoneRace2(c *check.C) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(10))
	l := s.limiter
	l.SetLimit(100)
	wg := sync.WaitGroup{}
	doneCh := make(chan func(), 100)
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			doneCh <- l.Start("n1")
		}()
		go func() {
			defer wg.Done()
			(<-doneCh)()
		}()
	}
	wg.Wait()
	c.Assert(l.Len("n1"), check.Equals, 0)
}

func (s *LimiterSuite) TestLocalLimiterAddDoneZeroLimit(c *check.C) {
	l := s.limiter
	l.SetLimit(0)
	var doneSlice []func()
	for i := 0; i < 100; i++ {
		doneSlice = append(doneSlice, l.Start("n1"))
	}
	c.Assert(l.Len("n1"), check.Equals, 0)
	for i := 0; i < 100; i++ {
		doneSlice[i]()
	}
	c.Assert(l.Len("n1"), check.Equals, 0)
}
