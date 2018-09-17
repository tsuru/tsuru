// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package validation

import (
	"testing"

	"gopkg.in/check.v1"
)

type S struct{}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) TestValidateEmail(c *check.C) {
	var data = []struct {
		input    string
		expected bool
	}{
		{"test+testing@example.com", true},
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
		c.Check(ValidateEmail(d.input), check.Equals, d.expected)
	}
}

func (s *S) TestValidateLength(c *check.C) {
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
		c.Check(ValidateLength(d.input, d.min, d.max), check.Equals, d.expected)
	}
}

func (s *S) TestValidateName(c *check.C) {
	var data = []struct {
		input    string
		expected bool
	}{
		{"myappmyappmyappmpmyappmyappmyappmyappmyapp", false},
		{"myappmyappmyappmpmyappmyappmyappmyappmyap", false},
		{"myappmyappmyappmpmyappmyappmyappmyappmya", true},
		{"myApp", false},
		{"my app", false},
		{"123myapp", false},
		{"myapp", true},
		{"_theirapp", false},
		{"my-app", true},
		{"-myapp", false},
		{"my_app", false},
		{"b", true},
	}
	for _, d := range data {
		c.Check(ValidateName(d.input), check.Equals, d.expected)
	}
}
