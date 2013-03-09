// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package errors

import (
	"launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) TestErrorMethodShouldReturnTheMessageString(c *gocheck.C) {
	e := Http{500, "Internal server error"}
	c.Assert(e.Error(), gocheck.Equals, e.Message)
}
