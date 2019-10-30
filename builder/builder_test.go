// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package builder

import (
	"errors"
	"testing"

	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

type S struct{}

var _ = check.Suite(S{})

func callPlatformWithError(appTypes.PlatformOptions) error {
	return errors.New("something is wrong")
}

func callPlatformRemoveWithError(string) error {
	return errors.New("something is wrong")
}

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s S) SetUpTest(c *check.C) {
	builders = make(map[string]Builder)
}

func (s S) TestRegisterAndGetBuilder(c *check.C) {
	var b Builder
	Register("my-builder", b)
	got, err := get("my-builder")
	c.Assert(err, check.IsNil)
	c.Check(got, check.DeepEquals, b)
	_, err = get("unknown-builder")
	c.Check(err, check.NotNil)
	expectedMessage := `unknown builder: "unknown-builder"`
	c.Assert(err.Error(), check.Equals, expectedMessage)
}

func (s S) TestRegistry(c *check.C) {
	var b1, b2, b3 Builder
	Register("builder1", b1)
	Register("builder2", b2)
	Register("builder3", b3)
	builders, err := Registry()
	c.Assert(err, check.IsNil)
	c.Assert(builders, check.HasLen, 3)
}

func (s S) TestPlatformBuild(c *check.C) {
	b1 := MockBuilder{}
	b2 := MockBuilder{
		OnPlatformBuild: callPlatformWithError,
	}
	Register("builder1", &b1)
	Register("builder2", &b2)
	err := PlatformBuild(appTypes.PlatformOptions{})
	c.Assert(err, check.IsNil)
}

func (s S) TestPlatformBuildError(c *check.C) {
	b1 := MockBuilder{
		OnPlatformBuild: callPlatformWithError,
	}
	b2 := MockBuilder{
		OnPlatformBuild: callPlatformWithError,
	}
	Register("builder1", &b1)
	Register("builder2", &b2)
	err := PlatformBuild(appTypes.PlatformOptions{})
	c.Assert(err, check.ErrorMatches, "(?s).*something is wrong.*something is wrong.*")
}

func (s S) TestPlatformBuildNoBuilder(c *check.C) {
	err := PlatformBuild(appTypes.PlatformOptions{})
	c.Assert(err, check.ErrorMatches, "No builder available")
}

func (s S) TestPlatformRemove(c *check.C) {
	b1 := MockBuilder{
		OnPlatformRemove: callPlatformRemoveWithError,
	}
	b2 := MockBuilder{
		OnPlatformRemove: callPlatformRemoveWithError,
	}
	b3 := MockBuilder{}
	Register("builder1", &b1)
	Register("builder2", &b2)
	Register("builder3", &b3)
	err := PlatformRemove("platform-name")
	c.Assert(err, check.IsNil)
}

func (s S) TestPlatformRemoveError(c *check.C) {
	b1 := MockBuilder{
		OnPlatformRemove: callPlatformRemoveWithError,
	}
	b2 := MockBuilder{
		OnPlatformRemove: callPlatformRemoveWithError,
	}
	Register("builder1", &b1)
	Register("builder2", &b2)
	err := PlatformRemove("platform-name")
	c.Assert(err, check.ErrorMatches, "(?s).*something is wrong.*something is wrong.*")
}

func (s S) TestPlatformRemoveNoBuilder(c *check.C) {
	err := PlatformRemove("platform-name")
	c.Assert(err, check.ErrorMatches, "No builder available")
}
