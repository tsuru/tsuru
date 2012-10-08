// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package validation

import (
	. "launchpad.net/gocheck"
	"testing"
)

type S struct{}

var _ = Suite(&S{})

func Test(t *testing.T) {
	TestingT(t)
}

func (s *S) TestValidateEmail(c *C) {
	var data = []struct {
		input    string
		expected bool
	}{
		{"gopher.golang@corp.globo.com", true},
		{"gopher@corp.globo.com", true},
		{"gopher@souza.cc", true},
		{"gopher@acm.org", true},
		{"gopher@golang.com.br", true},
		{"gopher@gmail.com", true},
		{"gopher@live.com", true},
		{"invalid-gopher", false},
		{"invalid@validate.c", false},
		{"invalid@validate", false},
	}
	for _, d := range data {
		c.Assert(ValidateEmail(d.input), Equals, d.expected)
	}
}

func (s *S) TestValidateLength(c *C) {
	var data = []struct {
		input    string
		min      int
		max      int
		expected bool
	}{
		{"abc", 10, -1, false},
		{"abc", -1, -1, true},
		{"gopher", -1, 3, false},
	}
	for _, d := range data {
		c.Assert(ValidateLength(d.input, d.min, d.max), Equals, d.expected)
	}
}
