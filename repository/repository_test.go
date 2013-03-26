// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import (
	"errors"
	"fmt"
	"github.com/globocom/config"
	"io"
	"launchpad.net/gocheck"
	"strings"
)

type FakeUnit struct {
	name     string
	commands []string
}

func (u *FakeUnit) RanCommand(cmd string) bool {
	for _, c := range u.commands {
		if c == cmd {
			return true
		}
	}
	return false
}

func (u *FakeUnit) GetName() string {
	return u.name
}

func (u *FakeUnit) Command(stdout, stderr io.Writer, cmd ...string) error {
	u.commands = append(u.commands, cmd[0])
	return nil
}

type FailingCloneUnit struct {
	FakeUnit
}

func (u *FailingCloneUnit) Command(stdout, stderr io.Writer, cmd ...string) error {
	if strings.HasPrefix(cmd[0], "git clone") {
		u.commands = append(u.commands, cmd[0])
		return errors.New("Failed to clone repository, it already exists!")
	}
	return u.FakeUnit.Command(nil, nil, cmd...)
}

func (s *S) TestCloneRepository(c *gocheck.C) {
	u := FakeUnit{name: "my-unit"}
	_, err := clone(&u)
	c.Assert(err, gocheck.IsNil)
	expectedCommand := fmt.Sprintf("git clone %s /home/application/current --depth 1", GetReadOnlyUrl(u.GetName()))
	c.Assert(u.RanCommand(expectedCommand), gocheck.Equals, true)
}

func (s *S) TestCloneRepositoryUndefinedPath(c *gocheck.C) {
	old, _ := config.Get("git:unit-repo")
	config.Unset("git:unit-repo")
	defer config.Set("git:unit-repo", old)
	u := FakeUnit{name: "my-unit"}
	_, err := clone(&u)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `Tsuru is misconfigured: key "git:unit-repo" not found`)
}

func (s *S) TestPullRepository(c *gocheck.C) {
	u := FakeUnit{name: "your-unit"}
	_, err := pull(&u)
	c.Assert(err, gocheck.IsNil)
	expectedCommand := fmt.Sprintf("cd /home/application/current && git pull origin master")
	c.Assert(u.RanCommand(expectedCommand), gocheck.Equals, true)
}

func (s *S) TestPullRepositoryUndefinedPath(c *gocheck.C) {
	old, _ := config.Get("git:unit-repo")
	config.Unset("git:unit-repo")
	defer config.Set("git:unit-repo", old)
	u := FakeUnit{name: "my-unit"}
	_, err := pull(&u)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `Tsuru is misconfigured: key "git:unit-repo" not found`)
}

func (s *S) TestCloneOrPullRepositoryRunsClone(c *gocheck.C) {
	u := FakeUnit{name: "my-unit"}
	_, err := CloneOrPull(&u)
	c.Assert(err, gocheck.IsNil)
	clone := fmt.Sprintf("git clone %s /home/application/current --depth 1", GetReadOnlyUrl(u.GetName()))
	pull := fmt.Sprintf("cd /home/application/current && git pull origin master")
	c.Assert(u.RanCommand(clone), gocheck.Equals, true)
	c.Assert(u.RanCommand(pull), gocheck.Equals, false)
}

func (s *S) TestCloneOrPullRepositoryRunsPullIfCloneFail(c *gocheck.C) {
	u := FailingCloneUnit{FakeUnit{name: "my-unit"}}
	_, err := CloneOrPull(&u)
	c.Assert(err, gocheck.IsNil)
	clone := fmt.Sprintf("git clone %s /home/application/current --depth 1", GetReadOnlyUrl(u.GetName()))
	pull := fmt.Sprintf("cd /home/application/current && git pull origin master")
	c.Assert(u.RanCommand(clone), gocheck.Equals, true)
	c.Assert(u.RanCommand(pull), gocheck.Equals, true)
}

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
