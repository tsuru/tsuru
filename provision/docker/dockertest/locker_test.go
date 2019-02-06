// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockertest

import check "gopkg.in/check.v1"

func (s *S) TestFakeLocker(c *check.C) {
	locker := NewFakeLocker()
	c.Check(locker.Lock("app1"), check.Equals, true)
	c.Check(locker.Lock("app2"), check.Equals, true)
	c.Check(locker.Lock("app1"), check.Equals, false)
	locker.Unlock("app1")
	c.Check(locker.Lock("app1"), check.Equals, true)
	c.Check(locker.Lock("app2"), check.Equals, false)
	locker.Unlock("app1")
	locker.Unlock("app2")
	c.Check(locker.Lock("app1"), check.Equals, true)
	c.Check(locker.Lock("app2"), check.Equals, true)
}
