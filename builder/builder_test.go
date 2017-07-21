// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package builder

import (
	"errors"
	"testing"

	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"

	check "gopkg.in/check.v1"
)

type S struct{}

type customPlatformBuilder struct {
	customBehavior func(PlatformOptions, string) error
}

var _ = check.Suite(S{})
var _ PlatformBuilder = &customPlatformBuilder{}
var _ Builder = &customPlatformBuilder{}

func callCounter(PlatformOptions, string) error {
	return nil
}

func callCounterWithError(PlatformOptions, string) error {
	return errors.New("something is wrong")
}

func (b *customPlatformBuilder) PlatformAdd(opts PlatformOptions) error {
	if b.customBehavior == nil {
		return nil
	}
	return b.customBehavior(opts, "")
}

func (b *customPlatformBuilder) PlatformUpdate(opts PlatformOptions) error {
	if b.customBehavior == nil {
		return nil
	}
	return b.customBehavior(opts, "")
}

func (b *customPlatformBuilder) PlatformRemove(name string) error {
	if b.customBehavior == nil {
		return nil
	}
	return b.customBehavior(PlatformOptions{}, name)
}

func (b *customPlatformBuilder) Build(provision.BuilderDeploy, provision.App, *event.Event, BuildOpts) (string, error) {
	return "", nil
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
	got, err := Get("my-builder")
	c.Assert(err, check.IsNil)
	c.Check(got, check.DeepEquals, b)
	_, err = Get("unknown-builder")
	c.Check(err, check.NotNil)
	expectedMessage := `unknown builder: "unknown-builder"`
	c.Assert(err.Error(), check.Equals, expectedMessage)
}

func (s S) TestGetDefaultBuilder(c *check.C) {
	var b1, b2 Builder
	DefaultBuilder = "default-builder"
	Register("default-builder", b1)
	Register("other-builder", b2)
	got, err := GetDefault()
	c.Check(err, check.IsNil)
	c.Check(got, check.DeepEquals, b1)
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

func (s S) TestPlatformAdd(c *check.C) {
	b1 := customPlatformBuilder{
		customBehavior: callCounter,
	}
	b2 := customPlatformBuilder{
		customBehavior: callCounterWithError,
	}
	Register("builder1", &b1)
	Register("builder2", &b2)
	err := PlatformAdd(PlatformOptions{})
	c.Assert(err, check.IsNil)
}

func (s S) TestPlatformAddError(c *check.C) {
	b1 := customPlatformBuilder{
		customBehavior: callCounterWithError,
	}
	b2 := customPlatformBuilder{
		customBehavior: callCounterWithError,
	}
	Register("builder1", &b1)
	Register("builder2", &b2)
	err := PlatformAdd(PlatformOptions{})
	c.Assert(err, check.ErrorMatches, "(?s).*something is wrong.*something is wrong.*")
}

func (s S) TestPlatformAddNoBuilder(c *check.C) {
	err := PlatformAdd(PlatformOptions{})
	c.Assert(err, check.ErrorMatches, "No builder available")
}

func (s S) TestPlatformUpdate(c *check.C) {
	b1 := customPlatformBuilder{
		customBehavior: callCounterWithError,
	}
	b2 := customPlatformBuilder{
		customBehavior: callCounter,
	}
	Register("builder1", &b1)
	Register("builder2", &b2)
	err := PlatformUpdate(PlatformOptions{})
	c.Assert(err, check.IsNil)
}

func (s S) TestPlatformUpdateError(c *check.C) {
	b1 := customPlatformBuilder{
		customBehavior: callCounterWithError,
	}
	b2 := customPlatformBuilder{
		customBehavior: callCounterWithError,
	}
	b3 := customPlatformBuilder{
		customBehavior: callCounterWithError,
	}
	Register("builder1", &b1)
	Register("builder2", &b2)
	Register("builder3", &b3)
	err := PlatformUpdate(PlatformOptions{})
	c.Assert(err, check.ErrorMatches, "(?s).*something is wrong.*something is wrong.*something is wrong.*")
}

func (s S) TestPlatformUpdateNoBuilder(c *check.C) {
	err := PlatformUpdate(PlatformOptions{})
	c.Assert(err, check.ErrorMatches, "No builder available")
}

func (s S) TestPlatformRemove(c *check.C) {
	b1 := customPlatformBuilder{
		customBehavior: callCounterWithError,
	}
	b2 := customPlatformBuilder{
		customBehavior: callCounterWithError,
	}
	b3 := customPlatformBuilder{
		customBehavior: callCounter,
	}
	Register("builder1", &b1)
	Register("builder2", &b2)
	Register("builder3", &b3)
	err := PlatformRemove("platform-name")
	c.Assert(err, check.IsNil)
}

func (s S) TestPlatformRemoveError(c *check.C) {
	b1 := customPlatformBuilder{
		customBehavior: callCounterWithError,
	}
	b2 := customPlatformBuilder{
		customBehavior: callCounterWithError,
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
