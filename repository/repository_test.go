// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import (
	"github.com/tsuru/config"
	"launchpad.net/gocheck"
)

func (s *S) TestGetRepositoryURLCallsGandalfGetRepository(c *gocheck.C) {
	url := ReadWriteURL("foobar")
	c.Assert(s.h.Url, gocheck.Equals, "/repository/foobar?:name=foobar")
	c.Assert(s.h.Method, gocheck.Equals, "GET")
	c.Assert(url, gocheck.Equals, "git@git.tsuru.io:foobar.git")
}

func (s *S) TestGetReadOnlyURL(c *gocheck.C) {
	url := ReadOnlyURL("foobar")
	c.Assert(s.h.Url, gocheck.Equals, "/repository/foobar?:name=foobar")
	c.Assert(s.h.Method, gocheck.Equals, "GET")
	expected := "git://git.tsuru.io/foobar.git"
	c.Assert(url, gocheck.Equals, expected)
}

func (s *S) TestGetPath(c *gocheck.C) {
	path, err := GetPath()
	c.Assert(err, gocheck.IsNil)
	expected := "/home/application/current"
	c.Assert(path, gocheck.Equals, expected)
}

func (s *S) TestGetServerURL(c *gocheck.C) {
	server, err := config.GetString("git:api-server")
	c.Assert(err, gocheck.IsNil)
	url, err := ServerURL()
	c.Assert(err, gocheck.IsNil)
	c.Assert(url, gocheck.Equals, server)
}

func (s *S) TestGetServerURLWithoutSetting(c *gocheck.C) {
	old, _ := config.Get("git:api-server")
	defer config.Set("git:api-server", old)
	config.Unset("git:api-server")
	url, err := ServerURL()
	c.Assert(url, gocheck.Equals, "")
	c.Assert(err.Error(), gocheck.Equals, "git server is disabled")
}
