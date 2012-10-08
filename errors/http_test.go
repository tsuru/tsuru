// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package errors

import (
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

func (s *S) TestErrorMethodShouldReturnTheMessageString(c *C) {
	e := Http{500, "Internal server error"}
	c.Assert(e.Error(), Equals, e.Message)
}
