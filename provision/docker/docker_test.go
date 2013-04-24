// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"github.com/globocom/commandmocker"
	"github.com/globocom/config"
	etesting "github.com/globocom/tsuru/exec/testing"
	"launchpad.net/gocheck"
)

func (s *S) TestDockerCreate(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	config.Set("docker:authorized-key-path", "somepath")
	container := container{name: "container"}
	_, err := container.create()
	c.Assert(err, gocheck.IsNil)
	args := []string{"run", "-d", "base", "/bin/bash", "myapp", "somepath"}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestDockerStart(c *gocheck.C) {
	container := container{name: "container"}
	err := container.start()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestDockerStop(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	container := container{name: "container", instanceId: "id"}
	err := container.stop()
	c.Assert(err, gocheck.IsNil)
	args := []string{"stop", "id"}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestDockerDestroy(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	container := container{name: "container", instanceId: "id"}
	err := container.remove()
	c.Assert(err, gocheck.IsNil)
	args := []string{"rm", "id"}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestDockerCommit(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	ctnr := container{name: "container", instanceId: "id"}
	err := ctnr.commit("app-name")
	c.Assert(err, gocheck.IsNil)
	registryUser, err := config.GetString("docker:registry-user")
	c.Assert(err, gocheck.IsNil)
	imageName := fmt.Sprintf("%s/app-name", registryUser)
	args := []string{"commit", "id", imageName}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestContainerIPRunsDockerInspectCommand(c *gocheck.C) {
	fexec := &etesting.FakeExecutor{}
	execut = fexec
	defer func() {
		execut = nil
	}()
	cont := container{name: "vm1", instanceId: "id"}
	cont.ip()
	args := []string{"inspect", "id"}
	c.Assert(fexec.ExecutedCmd("docker", args), gocheck.Equals, true)
}

func (s *S) TestContainerIPReturnsIpFromDockerInspect(c *gocheck.C) {
	cmdReturn := `
    {
            \"NetworkSettings\": {
            \"IpAddress\": \"10.10.10.10\",
            \"IpPrefixLen\": 8,
            \"Gateway\": \"10.65.41.1\",
            \"PortMapping\": {}
    }
}`
	tmpdir, err := commandmocker.Add("docker", cmdReturn)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	cont := container{name: "vm1", instanceId: "id"}
	ip, err := cont.ip()
	c.Assert(err, gocheck.IsNil)
	c.Assert(ip, gocheck.Equals, "10.10.10.10")
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
}
