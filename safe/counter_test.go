// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package safe

import (
	"runtime"
	"sync"

	"launchpad.net/gocheck"
)

func (s *S) TestNewCounter(c *gocheck.C) {
	ct := NewCounter(2)
	c.Assert(ct.v, gocheck.Equals, int64(2))
}

func (s *S) TestCounterVal(c *gocheck.C) {
	ct := NewCounter(2)
	ct.v = 5
	c.Assert(ct.Val(), gocheck.Equals, int64(5))
}

func (s *S) TestIncrement(c *gocheck.C) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(16))
	n := 50
	var wg sync.WaitGroup
	var ct Counter
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			for i := 0; i < n; i++ {
				ct.Increment()
			}
			wg.Done()
		}()
	}
	wg.Wait()
	c.Assert(ct.Val(), gocheck.Equals, int64(n*n))
}

func (s *S) TestDecrement(c *gocheck.C) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(16))
	n := 50
	var wg sync.WaitGroup
	ct := NewCounter(int64(n * n))
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			for i := 0; i < n; i++ {
				ct.Decrement()
			}
			wg.Done()
		}()
	}
	wg.Wait()
	c.Assert(ct.Val(), gocheck.Equals, int64(0))
}
