// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package errors

import (
	"testing"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (s *S) TestHTTPError(c *check.C) {
	e := HTTP{500, "Internal server error"}
	c.Assert(e.Error(), check.Equals, e.Message)
}

func (s *S) TestValidationError(c *check.C) {
	e := ValidationError{Message: "something"}
	c.Assert(e.Error(), check.Equals, "something")
}
