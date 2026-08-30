// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"testing"

	"github.com/tsuru/config"
	"go.mongodb.org/mongo-driver/bson/primitive"
	check "gopkg.in/check.v1"
)

type S struct{}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) TestConvertEntries(c *check.C) {
	tests := []struct {
		input    any
		expected any
	}{
		{
			input:    map[any]any{"": ""},
			expected: map[string]any{"": ""},
		},
		{
			input:    []any{map[any]any{"": ""}},
			expected: []any{map[string]any{"": ""}},
		},
		{
			input:    []any{map[any]any{"": []any{map[any]any{"a": 1}}}},
			expected: []any{map[string]any{"": []any{map[string]any{"a": 1}}}},
		},
	}
	for _, tt := range tests {
		c.Assert(ConvertEntries(tt.input), check.DeepEquals, tt.expected)
	}
}

func (s *S) TestUnconvertEntries(c *check.C) {
	tests := []struct {
		input    any
		expected any
	}{
		{
			input: map[string]any{
				"headers": primitive.A{
					"test1=a",
					"test2=b",
				},
			},
			expected: map[any]any{
				"headers": []any{
					"test1=a",
					"test2=b",
				},
			},
		},
		{
			input: map[string]any{
				"headers": primitive.M{
					"test1": "a",
					"test2": "b",
				},
			},
			expected: map[any]any{
				"headers": map[any]any{
					"test1": "a",
					"test2": "b",
				},
			},
		},
	}
	for _, tt := range tests {
		c.Assert(UnconvertEntries(tt.input), check.DeepEquals, tt.expected)
	}
}

func (s *S) TestUnmarshalConfig(c *check.C) {
	err := config.ReadConfigBytes([]byte(`
a:
 b:
  - c: 9
    d:
     e: a
     f: 1
  - c: 10
`))
	c.Assert(err, check.IsNil)
	type dtype struct {
		E string
		F int
	}
	type btype struct {
		C int
		D dtype
	}
	type atype struct {
		B []btype
	}
	expected := atype{
		B: []btype{
			{C: 9, D: dtype{
				E: "a",
				F: 1,
			}},
			{C: 10},
		},
	}
	var aval atype
	err = UnmarshalConfig("a", &aval)
	c.Assert(err, check.IsNil)
	c.Assert(aval, check.DeepEquals, expected)
	var bval []btype
	err = UnmarshalConfig("a:b", &bval)
	c.Assert(err, check.IsNil)
	c.Assert(bval, check.DeepEquals, expected.B)

}
