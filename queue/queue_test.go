// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"testing"

	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type S struct{}

var _ = check.Suite(&S{})

func (s *S) TestFactory(c *check.C) {
	config.Set("queue", "redis")
	defer config.Unset("queue")
	f, err := Factory()
	c.Assert(err, check.IsNil)
	_, ok := f.(*redismqQFactory)
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestFactoryConfigUndefined(c *check.C) {
	f, err := Factory()
	c.Assert(err, check.IsNil)
	_, ok := f.(*redismqQFactory)
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestFactoryConfigUnknown(c *check.C) {
	config.Set("queue", "unknown")
	defer config.Unset("queue")
	f, err := Factory()
	c.Assert(f, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, `Queue "unknown" is not known.`)
}

func (s *S) TestRegister(c *check.C) {
	config.Set("queue", "unregistered")
	defer config.Unset("queue")
	Register("unregistered", &redismqQFactory{})
	_, err := Factory()
	c.Assert(err, check.IsNil)
}
