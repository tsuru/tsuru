// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package errors

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	pkgErrors "github.com/pkg/errors"
	check "gopkg.in/check.v1"
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

func (s *S) TestStatusCode(c *check.C) {
	e := &HTTP{
		Code: http.StatusServiceUnavailable,
	}
	c.Assert(e.StatusCode(), check.Equals, http.StatusServiceUnavailable)
}

func (s *S) TestErrorMessage(c *check.C) {
	tt := []struct {
		Description string
		Err         error
		Expectation string
	}{
		{"given HTTP error", &HTTP{Message: "fail"}, "fail"},
		{"given ConflictError error", &ConflictError{Message: "fail"}, "fail"},
		{"given ValidationError error", &ValidationError{Message: "fail"}, "fail"},
		{"given NotAuthorizedError error", &NotAuthorizedError{Message: "fail"}, "fail"},
		{"given CompositeError error without base", &CompositeError{Message: "fail"}, "fail"},
		{"given CompositeError error", &CompositeError{Message: "fail", Base: errors.New("source")},
			"fail Caused by: source"},
	}

	for _, tc := range tt {
		c.Assert(tc.Err.Error(), check.Equals, tc.Expectation)
	}
}

func (s *S) TestMultiError_Add(c *check.C) {
	multiError := NewMultiError()
	expectedError := errors.New("fail")
	multiError.Add(expectedError)
	c.Assert(multiError.errors, check.HasLen, 1)
	c.Assert(multiError.errors[0], check.Equals, expectedError)
}

func (s *S) TestMultiError_ToError(c *check.C) {
	multiError := NewMultiError()
	c.Assert(multiError.ToError(), check.IsNil)

	expectedError := errors.New("fail")
	multiError.Add(expectedError)
	c.Assert(multiError.ToError(), check.Equals, expectedError)

	multiError.Add(errors.New("fail"))
	c.Assert(multiError.ToError(), check.Equals, multiError)
}

func (s *S) TestMultiError_Error(c *check.C) {
	multiError := NewMultiError()
	c.Assert(multiError.Error(), check.Equals, "multi error created but no errors added")

	multiError.Add(errors.New("foo"))
	c.Assert(multiError.Error(), check.Matches, "(?s)foo.*")

	multiError.Add(errors.New("bar"))
	c.Assert(strings.Contains(multiError.Error(), "multiple errors reported (2)"), check.Equals, true)
	c.Assert(strings.Contains(multiError.Error(), "error #0: foo"), check.Equals, true)
	c.Assert(strings.Contains(multiError.Error(), "error #1: bar"), check.Equals, true)
}

func (s *S) TestMultiError_Format(c *check.C) {
	multiError := NewMultiError()
	c.Assert(fmt.Sprintf("%s", multiError), check.Equals, "")

	multiError.Add(errors.New("fail"))
	c.Assert(fmt.Sprintf("%+s", multiError), check.Equals, "fail")
	c.Assert(fmt.Sprintf("%#v", multiError), check.Matches, `(?s).*errorString.*fail.*`)
}
