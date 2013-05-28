// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nginx

import (
	"github.com/globocom/commandmocker"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/fs/testing"
	"github.com/globocom/tsuru/router"
	"io/ioutil"
	"launchpad.net/gocheck"
)

func (s *S) TestShouldBeRegistered(c *gocheck.C) {
	r, err := router.Get("nginx")
	c.Assert(err, gocheck.IsNil)
	c.Assert(r, gocheck.FitsTypeOf, &NginxRouter{})
}

func (s *S) TestAddRoute(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	config.Set("nginx:domain", "andrewzito.com")
	config.Set("nginx:routes-path", "testdata")
	rfs := &testing.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	var r NginxRouter
	err = r.AddRoute("name", "127.0.0.1")
	c.Assert(err, gocheck.IsNil)
	file, err := rfs.Open("testdata/name")
	c.Assert(err, gocheck.IsNil)
	data, err := ioutil.ReadAll(file)
	c.Assert(err, gocheck.IsNil)
	expected := `server {
	listen 80;
	server_name name.andrewzito.com;
	location / {
		proxy_pass http://127.0.0.1;
	}
}`
	c.Assert(string(data), gocheck.Equals, expected)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	expected = "service nginx restart"
	c.Assert(commandmocker.Output(tmpdir), gocheck.Equals, expected)
}

func (s *S) TestAddCName(c *gocheck.C) {
	var r NginxRouter
	err := r.AddCName("myapp.com", "myapp")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestUnsetCName(c *gocheck.C) {
	var r NginxRouter
	err := r.UnsetCName("myapp.com", "10.10.10.10")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestRestart(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	var r NginxRouter
	err = r.restart()
	c.Assert(err, gocheck.IsNil)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	expected := "service nginx restart"
	c.Assert(commandmocker.Output(tmpdir), gocheck.Equals, expected)
}

func (s *S) TestRemoveBackend(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	config.Set("nginx:routes-path", "testdata")
	rfs := &testing.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	var r NginxRouter
	err = r.RemoveBackend("name")
	c.Assert(err, gocheck.IsNil)
	c.Assert(rfs.HasAction("remove testdata/name"), gocheck.Equals, true)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	expected := "service nginx restart"
	c.Assert(commandmocker.Output(tmpdir), gocheck.Equals, expected)
}

func (s *S) TestAddr(c *gocheck.C) {
	config.Set("nginx:domain", "andrewzito.com")
	var r NginxRouter
	addr, _ := r.Addr("name")
	c.Assert(addr, gocheck.Equals, "name.andrewzito.com")
}
