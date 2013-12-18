// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"github.com/globocom/config"
	"launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) TestMessageFail(c *gocheck.C) {
	m := Message{}
	c.Assert(m.fail, gocheck.Equals, false)
	m.Fail()
	c.Assert(m.fail, gocheck.Equals, true)
}

func (s *S) TestFactory(c *gocheck.C) {
	config.Set("queue", "beanstalkd")
	defer config.Unset("queue")
	f, err := Factory()
	c.Assert(err, gocheck.IsNil)
	_, ok := f.(beanstalkdFactory)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestFactoryConfigUndefined(c *gocheck.C) {
	f, err := Factory()
	c.Assert(err, gocheck.IsNil)
	_, ok := f.(beanstalkdFactory)
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
	Register("unregistered", beanstalkdFactory{})
	_, err := Factory()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestTimeoutError(c *gocheck.C) {
	var err error = &timeoutError{timeout: 5e9}
	expected := "Timed out waiting for message after 5s."
	c.Assert(err.Error(), gocheck.Equals, expected)
}
