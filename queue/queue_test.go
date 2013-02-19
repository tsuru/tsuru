// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type S struct{}

var _ = Suite(&S{})

func (s *S) TestMessageDelete(c *C) {
	m := Message{}
	c.Assert(m.delete, Equals, false)
	m.Delete()
	c.Assert(m.delete, Equals, true)
}
