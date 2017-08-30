// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package errors

import (
	"errors"
	"fmt"
	"testing"

	pkgErrors "github.com/pkg/errors"
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

func (s *S) TestMultiErrorFormat(c *check.C) {
	cause := errors.New("root error")
	e := NewMultiError(errors.New("error 1"), pkgErrors.WithStack(cause))
	c.Assert(fmt.Sprintf("%s", e), check.Equals, "multiple errors reported (2): error 0: error 1 - error 1: root error")
}

func (s *S) TestMultiErrorFormatSingle(c *check.C) {
	e := NewMultiError(errors.New("error 1"))
	c.Assert(fmt.Sprintf("%s", e), check.Equals, "error 1")
}
