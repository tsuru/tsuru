// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import (
	"errors"
	"fmt"
	"github.com/globocom/config"
	"io"
	. "launchpad.net/gocheck"
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

func (s *S) TestCloneRepository(c *C) {
	u := FakeUnit{name: "my-unit"}
	_, err := clone(&u)
	c.Assert(err, IsNil)
	expectedCommand := fmt.Sprintf("git clone %s /home/application/current --depth 1", GetReadOnlyUrl(u.GetName()))
	c.Assert(u.RanCommand(expectedCommand), Equals, true)
}

func (s *S) TestPullRepository(c *C) {
	u := FakeUnit{name: "your-unit"}
	_, err := pull(&u)
	c.Assert(err, IsNil)
	expectedCommand := fmt.Sprintf("cd /home/application/current && git pull origin master")
	c.Assert(u.RanCommand(expectedCommand), Equals, true)
}

func (s *S) TestCloneOrPullRepositoryRunsClone(c *C) {
	u := FakeUnit{name: "my-unit"}
	_, err := CloneOrPull(&u)
	c.Assert(err, IsNil)
	clone := fmt.Sprintf("git clone %s /home/application/current --depth 1", GetReadOnlyUrl(u.GetName()))
	pull := fmt.Sprintf("cd /home/application/current && git pull origin master")
	c.Assert(u.RanCommand(clone), Equals, true)
	c.Assert(u.RanCommand(pull), Equals, false)
}

func (s *S) TestCloneOrPullRepositoryRunsPullIfCloneFail(c *C) {
	u := FailingCloneUnit{FakeUnit{name: "my-unit"}}
	_, err := CloneOrPull(&u)
	c.Assert(err, IsNil)
	clone := fmt.Sprintf("git clone %s /home/application/current --depth 1", GetReadOnlyUrl(u.GetName()))
	pull := fmt.Sprintf("cd /home/application/current && git pull origin master")
	c.Assert(u.RanCommand(clone), Equals, true)
	c.Assert(u.RanCommand(pull), Equals, true)
}

func (s *S) TestGetRepositoryUrl(c *C) {
	url := GetUrl("foobar")
	expected := "git@mygithost:foobar.git"
	c.Assert(url, Equals, expected)
}

func (s *S) TestGetReadOnlyUrl(c *C) {
	url := GetReadOnlyUrl("foobar")
	expected := "git://mygithost/foobar.git"
	c.Assert(url, Equals, expected)
}

func (s *S) TestGetPath(c *C) {
	path, err := GetPath()
	c.Assert(err, IsNil)
	expected := "/home/application/current"
	c.Assert(path, Equals, expected)
}

func (s *S) TestGetGitServer(c *C) {
	gitServer, err := config.GetString("git:host")
	c.Assert(err, IsNil)
	defer config.Set("git:host", gitServer)
	config.Set("git:host", "gandalf-host.com")
	uri := getGitServer()
	c.Assert(uri, Equals, "gandalf-host.com")
}

func (s *S) TestGetServerUri(c *C) {
	server, err := config.GetString("git:host")
	c.Assert(err, IsNil)
	protocol, err := config.GetString("git:protocol")
	port, err := config.Get("git:port")
	uri := GitServerUri()
	c.Assert(uri, Equals, fmt.Sprintf("%s://%s:%d", protocol, server, port))
}

func (s *S) TestGetServerUriWithoutPort(c *C) {
	config.Unset("git:port")
	defer config.Set("git:port", 8080)
	server, err := config.GetString("git:host")
	c.Assert(err, IsNil)
	protocol, err := config.GetString("git:protocol")
	uri := GitServerUri()
	c.Assert(uri, Equals, fmt.Sprintf("%s://%s", protocol, server))
}

func (s *S) TestGetServerUriWithoutProtocol(c *C) {
	config.Unset("git:protocol")
	defer config.Set("git:protocol", "http")
	server, err := config.GetString("git:host")
	c.Assert(err, IsNil)
	uri := GitServerUri()
	c.Assert(uri, Equals, "http://"+server+":8080")
}

func (s *S) TestGetServerUriWithoutHost(c *C) {
	old, _ := config.Get("git:host")
	defer config.Set("git:host", old)
	config.Unset("git:host")
	defer func() {
		r := recover()
		c.Assert(r, NotNil)
	}()
	GitServerUri()
}
