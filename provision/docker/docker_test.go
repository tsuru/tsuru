// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/globocom/commandmocker"
	"launchpad.net/gocheck"
)

func (s *S) TestDockerCreate(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	container := container{name: "container"}
	err, _ = container.create()
	c.Assert(err, gocheck.IsNil)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	expected := "docker run -d base-nginx-sshd-key /usr/sbin/sshd -D"
	c.Assert(commandmocker.Output(tmpdir), gocheck.Equals, expected)
}

func (s *S) TestDockerStart(c *gocheck.C) {
	container := container{name: "container"}
	err := container.start()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestDockerStop(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	container := container{name: "container", instanceId: "id"}
	err = container.stop()
	c.Assert(err, gocheck.IsNil)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	expected := "docker stop id"
	c.Assert(commandmocker.Output(tmpdir), gocheck.Equals, expected)
}

func (s *S) TestDockerDestroy(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	container := container{name: "container", instanceId: "id"}
	err = container.destroy()
	c.Assert(err, gocheck.IsNil)
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	expected := "docker rm id"
	c.Assert(commandmocker.Output(tmpdir), gocheck.Equals, expected)
}

func (s *S) TestContainerIPRunsDockerInspectCommand(c *gocheck.C) {
	tmpdir, err := commandmocker.Add("sudo", "$*")
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	cont := container{name: "vm1", instanceId: "id"}
	cont.ip()
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
	expected := "docker inspect id"
	c.Assert(commandmocker.Output(tmpdir), gocheck.Equals, expected)
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
	tmpdir, err := commandmocker.Add("sudo", cmdReturn)
	c.Assert(err, gocheck.IsNil)
	defer commandmocker.Remove(tmpdir)
	cont := container{name: "vm1", instanceId: "id"}
	ip, err := cont.ip()
	c.Assert(err, gocheck.IsNil)
	c.Assert(ip, gocheck.Equals, "10.10.10.10")
	c.Assert(commandmocker.Ran(tmpdir), gocheck.Equals, true)
}
