// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package safe

import (
	"runtime"
	"sync"

	"launchpad.net/gocheck"
)

func (*S) TestMultiLocker(c *gocheck.C) {
	locker := multiLocker{m: make(map[string]*sync.Mutex)}
	locker.Lock("user@tsuru.io")
	locker.Lock("another.user@tsuru.io")
	locker.Unlock("user@tsuru.io")
	locker.Unlock("another.user@tsuru.io")
}

func (*S) TestMultiLockerSingle(c *gocheck.C) {
	locker := multiLocker{m: make(map[string]*sync.Mutex)}
	locker.Lock("user@tsuru.io")
	locker.Unlock("user@tsuru.io")
	locker.Lock("user@tsuru.io")
	locker.Unlock("user@tsuru.io")
}

func (*S) TestMultiLockerUsage(c *gocheck.C) {
	locker := multiLocker{m: make(map[string]*sync.Mutex)}
	locker.Lock("user@tsuru.io")
	count := 0
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		locker.Lock("user@tsuru.io")
		count--
		locker.Unlock("user@tsuru.io")
		wg.Done()
	}()
	runtime.Gosched()
	c.Assert(count, gocheck.Equals, 0)
	locker.Unlock("user@tsuru.io")
	wg.Wait()
	c.Assert(count, gocheck.Equals, -1)
}

func (*S) TestMultiLockerUnlocked(c *gocheck.C) {
	defer func() {
		r := recover()
		c.Assert(r, gocheck.NotNil)
	}()
	locker := multiLocker{m: make(map[string]*sync.Mutex)}
	locker.Unlock("user@tsuru.io")
}

func (*S) TestMultiLockerFunction(c *gocheck.C) {
	locker := MultiLocker()
	locker.Lock("user@tsuru.io")
	defer locker.Unlock("user@tsuru.io")
}
