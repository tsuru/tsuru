// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/apitest"
	"github.com/tsuru/tsuru/hc"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"gopkg.in/check.v1"
)

func (s *S) TestHealthCheck(c *check.C) {
	handler := apitest.TestHandler{Content: "WORKING"}
	server := repositorytest.StartGandalfTestServer(&handler)
	defer server.Close()
	err := healthCheck()
	c.Assert(err, check.IsNil)
}

func (s *S) TestHealthCheckFailure(c *check.C) {
	handler := apitest.TestHandler{Content: "epic fail"}
	server := repositorytest.StartGandalfTestServer(&handler)
	defer server.Close()
	err := healthCheck()
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "unexpected status - epic fail")
}

func (s *S) TestHealthCheckDisabled(c *check.C) {
	config.Unset("git:api-server")
	err := healthCheck()
	c.Assert(err, check.Equals, hc.ErrDisabledComponent)
}

func (s *S) TestGetRepositoryURLCallsGandalfGetRepository(c *check.C) {
	url := ReadWriteURL("foobar")
	c.Assert(s.h.Url, check.Equals, "/repository/foobar?:name=foobar")
	c.Assert(s.h.Method, check.Equals, "GET")
	c.Assert(url, check.Equals, "git@git.tsuru.io:foobar.git")
}

func (s *S) TestGetReadOnlyURL(c *check.C) {
	url := ReadOnlyURL("foobar")
	c.Assert(s.h.Url, check.Equals, "/repository/foobar?:name=foobar")
	c.Assert(s.h.Method, check.Equals, "GET")
	expected := "git://git.tsuru.io/foobar.git"
	c.Assert(url, check.Equals, expected)
}

func (s *S) TestGetPath(c *check.C) {
	path, err := GetPath()
	c.Assert(err, check.IsNil)
	expected := "/home/application/current"
	c.Assert(path, check.Equals, expected)
}

func (s *S) TestGetServerURL(c *check.C) {
	server, err := config.GetString("git:api-server")
	c.Assert(err, check.IsNil)
	url, err := ServerURL()
	c.Assert(err, check.IsNil)
	c.Assert(url, check.Equals, server)
}

func (s *S) TestGetServerURLWithoutSetting(c *check.C) {
	old, _ := config.Get("git:api-server")
	defer config.Set("git:api-server", old)
	config.Unset("git:api-server")
	url, err := ServerURL()
	c.Assert(url, check.Equals, "")
	c.Assert(err, check.Equals, ErrGandalfDisabled)
}

func (s *S) TestRegister(c *check.C) {
	mngr := nopManager{}
	Register("nope", mngr)
	defer func() {
		delete(managers, "nope")
	}()
	c.Assert(managers["nope"], check.Equals, mngr)
}

func (s *S) TestRegisterOnNilMap(c *check.C) {
	oldManagers := managers
	managers = nil
	defer func() {
		managers = oldManagers
	}()
	mngr := nopManager{}
	Register("nope", mngr)
	c.Assert(managers["nope"], check.Equals, mngr)
}

func (s *S) TestManager(c *check.C) {
	mngr := nopManager{}
	Register("nope", mngr)
	config.Set("repo-manager", "nope")
	defer config.Unset("repo-manager")
	current := Manager()
	c.Assert(current, check.Equals, mngr)
}

func (s *S) TestManagerUnconfigured(c *check.C) {
	mngr := nopManager{}
	Register("nope", mngr)
	gandalf := nopManager{}
	Register("gandalf", gandalf)
	config.Unset("repo-manager")
	current := Manager()
	c.Assert(current, check.Equals, gandalf)
}

func (s *S) TestManagerUnknown(c *check.C) {
	config.Set("repo-manager", "something")
	defer config.Unset("repo-manager")
	current := Manager()
	c.Assert(current, check.FitsTypeOf, nopManager{})
}
