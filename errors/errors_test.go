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

func (s *S) TestHTTPError(c *gocheck.C) {
	e := HTTP{500, "Internal server error"}
	c.Assert(e.Error(), gocheck.Equals, e.Message)
}

func (s *S) TestValidationError(c *gocheck.C) {
	e := ValidationError{Message: "something"}
	c.Assert(e.Error(), gocheck.Equals, "something")
}
