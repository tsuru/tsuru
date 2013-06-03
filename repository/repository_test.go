// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import (
	"github.com/globocom/config"
	"launchpad.net/gocheck"
)

func (s *S) TestGetRepositoryURL(c *gocheck.C) {
	url := ReadWriteURL("foobar")
	expected := "git@public.mygithost:foobar.git"
	c.Assert(url, gocheck.Equals, expected)
}

func (s *S) TestGetRepositoryURLWithoutSetting(c *gocheck.C) {
	old, _ := config.Get("git:rw-host")
	defer config.Set("git:rw-host", old)
	config.Unset("git:rw-host")
	defer func() {
		r := recover()
		c.Assert(r, gocheck.NotNil)
	}()
	ReadWriteURL("foobar")
}

func (s *S) TestGetReadOnlyURL(c *gocheck.C) {
	url := ReadOnlyURL("foobar")
	expected := "git://private.mygithost/foobar.git"
	c.Assert(url, gocheck.Equals, expected)
}

func (s *S) TestGetReadOnlyURLNoSetting(c *gocheck.C) {
	old, _ := config.Get("git:ro-host")
	defer config.Set("git:ro-host", old)
	config.Unset("git:ro-host")
	defer func() {
		r := recover()
		c.Assert(r, gocheck.NotNil)
	}()
	ReadOnlyURL("foobar")
}

func (s *S) TestGetPath(c *gocheck.C) {
	path, err := GetPath()
	c.Assert(err, gocheck.IsNil)
	expected := "/home/application/current"
	c.Assert(path, gocheck.Equals, expected)
}

func (s *S) TestGetServerUri(c *gocheck.C) {
	server, err := config.GetString("git:api-server")
	c.Assert(err, gocheck.IsNil)
	uri := ServerURL()
	c.Assert(uri, gocheck.Equals, server)
}

func (s *S) TestGetServerUriWithoutSetting(c *gocheck.C) {
	old, _ := config.Get("git:api-server")
	defer config.Set("git:api-server", old)
	config.Unset("git:api-server")
	defer func() {
		r := recover()
		c.Assert(r, gocheck.NotNil)
	}()
	ServerURL()
}
