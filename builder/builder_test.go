// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package builder

import (
	"reflect"
	"testing"

	check "gopkg.in/check.v1"
)

type S struct{}

var _ = check.Suite(S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s S) SetUpTest(c *check.C) {
	builders = make(map[string]Builder)
}

func (s S) TestRegisterAndGetBuilder(c *check.C) {
	var b Builder
	Register("my-builder", b)
	got, err := Get("my-builder")
	c.Assert(err, check.IsNil)
	c.Check(got, check.DeepEquals, b)
	_, err = Get("unknown-builder")
	c.Check(err, check.NotNil)
	expectedMessage := `unknown builder: "unknown-builder"`
	c.Assert(err.Error(), check.Equals, expectedMessage)
}

func (s S) TestRegistry(c *check.C) {
	var b1, b2 Builder
	Register("my-builder", b1)
	Register("your-builder", b2)
	builders, err := Registry()
	c.Assert(err, check.IsNil)
	alt1 := []Builder{b1, b2}
	alt2 := []Builder{b2, b1}
	if !reflect.DeepEqual(builders, alt1) && !reflect.DeepEqual(builders, alt2) {
		c.Errorf("Registry(): Expected %#v. Got %#v.", alt1, builders)
	}
}
