// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"github.com/tsuru/config"
	"launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) TestFactory(c *gocheck.C) {
	config.Set("queue", "redis")
	defer config.Unset("queue")
	f, err := Factory()
	c.Assert(err, gocheck.IsNil)
	_, ok := f.(*redismqQFactory)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestFactoryConfigUndefined(c *gocheck.C) {
	f, err := Factory()
	c.Assert(err, gocheck.IsNil)
	_, ok := f.(*redismqQFactory)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestFactoryConfigUnknown(c *gocheck.C) {
	config.Set("queue", "unknown")
	defer config.Unset("queue")
	f, err := Factory()
	c.Assert(f, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `Queue "unknown" is not known.`)
}

func (s *S) TestRegister(c *gocheck.C) {
	config.Set("queue", "unregistered")
	defer config.Unset("queue")
	Register("unregistered", &redismqQFactory{})
	_, err := Factory()
	c.Assert(err, gocheck.IsNil)
}
