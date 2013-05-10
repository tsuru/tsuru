// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import (
	"fmt"
	"github.com/globocom/config"
	"launchpad.net/gocheck"
)

func (s *S) TestGetRepositoryUrl(c *gocheck.C) {
	url := GetUrl("foobar")
	expected := "git@mygithost:foobar.git"
	c.Assert(url, gocheck.Equals, expected)
}

func (s *S) TestGetReadOnlyUrl(c *gocheck.C) {
	url := GetReadOnlyUrl("foobar")
	expected := "git://mygithost/foobar.git"
	c.Assert(url, gocheck.Equals, expected)
}

func (s *S) TestGetPath(c *gocheck.C) {
	path, err := GetPath()
	c.Assert(err, gocheck.IsNil)
	expected := "/home/application/current"
	c.Assert(path, gocheck.Equals, expected)
}

func (s *S) TestGetGitServer(c *gocheck.C) {
	gitServer, err := config.GetString("git:host")
	c.Assert(err, gocheck.IsNil)
	defer config.Set("git:host", gitServer)
	config.Set("git:host", "gandalf-host.com")
	uri := getGitServer()
	c.Assert(uri, gocheck.Equals, "gandalf-host.com")
}

func (s *S) TestGetServerUri(c *gocheck.C) {
	server, err := config.GetString("git:host")
	c.Assert(err, gocheck.IsNil)
	protocol, err := config.GetString("git:protocol")
	port, err := config.Get("git:port")
	uri := GitServerUri()
	c.Assert(uri, gocheck.Equals, fmt.Sprintf("%s://%s:%d", protocol, server, port))
}

func (s *S) TestGetServerUriWithoutPort(c *gocheck.C) {
	config.Unset("git:port")
	defer config.Set("git:port", 8080)
	server, err := config.GetString("git:host")
	c.Assert(err, gocheck.IsNil)
	protocol, err := config.GetString("git:protocol")
	uri := GitServerUri()
	c.Assert(uri, gocheck.Equals, fmt.Sprintf("%s://%s", protocol, server))
}

func (s *S) TestGetServerUriWithoutProtocol(c *gocheck.C) {
	config.Unset("git:protocol")
	defer config.Set("git:protocol", "http")
	server, err := config.GetString("git:host")
	c.Assert(err, gocheck.IsNil)
	uri := GitServerUri()
	c.Assert(uri, gocheck.Equals, "http://"+server+":8080")
}

func (s *S) TestGetServerUriWithoutHost(c *gocheck.C) {
	old, _ := config.Get("git:host")
	defer config.Set("git:host", old)
	config.Unset("git:host")
	defer func() {
		r := recover()
		c.Assert(r, gocheck.NotNil)
	}()
	GitServerUri()
}
