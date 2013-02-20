// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"github.com/globocom/config"
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type S struct{}

var _ = Suite(&S{})

func (s *S) TestMessageDelete(c *C) {
	m := Message{}
	c.Assert(m.delete, Equals, false)
	m.Delete()
	c.Assert(m.delete, Equals, true)
}

func (s *S) TestFactory(c *C) {
	config.Set("queue", "beanstalk")
	defer config.Unset("queue")
	f, err := Factory()
	c.Assert(err, IsNil)
	_, ok := f.(beanstalkFactory)
	c.Assert(ok, Equals, true)
}

func (s *S) TestFactoryConfigUndefined(c *C) {
	f, err := Factory()
	c.Assert(err, IsNil)
	_, ok := f.(beanstalkFactory)
	c.Assert(ok, Equals, true)
}

func (s *S) TestFactoryConfigUnknown(c *C) {
	config.Set("queue", "unknown")
	defer config.Unset("queue")
	f, err := Factory()
	c.Assert(f, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, `Queue "unknown" is not known.`)
}

func (s *S) TestRegister(c *C) {
	config.Set("queue", "unregistered")
	defer config.Unset("queue")
	Register("unregistered", beanstalkFactory{})
	_, err := Factory()
	c.Assert(err, IsNil)
}
