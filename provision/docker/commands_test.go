// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"github.com/globocom/config"
	ftesting "github.com/globocom/tsuru/fs/testing"
	"github.com/globocom/tsuru/testing"
	"launchpad.net/gocheck"
	"os"
)

func (s *S) TestDeployCmds(c *gocheck.C) {
	app := testing.NewFakeApp("app-name", "python", 1)
	docker, err := config.GetString("docker:binary")
	c.Assert(err, gocheck.IsNil)
	deployCmd, err := config.GetString("docker:deploy-cmd")
	c.Assert(err, gocheck.IsNil)
	imageName := getImage(app)
	expected := []string{docker, "run", imageName, deployCmd}
	cmds, err := deployCmds(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestRunCmds(c *gocheck.C) {
	app := testing.NewFakeApp("app-name", "python", 1)
	docker, err := config.GetString("docker:binary")
	c.Assert(err, gocheck.IsNil)
	runCmd, err := config.GetString("docker:run-cmd:bin")
	c.Assert(err, gocheck.IsNil)
	imageName := getImage(app)
	port, err := config.GetString("docker:run-cmd:port")
	c.Assert(err, gocheck.IsNil)
	expected := []string{docker, "run", "-d", "-t", "-p", port, imageName, "/bin/bash", "-c", runCmd}
	cmds, err := runCmds(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestSSHCmds(c *gocheck.C) {
	addKeyCommand, err := config.GetString("docker:ssh:add-key-cmd")
	c.Assert(err, gocheck.IsNil)
	keyContent := "key-content"
	sshdPath := "/usr/sbin/sshd"
	expected := []string{
		fmt.Sprintf("%s %s", addKeyCommand, keyContent),
		sshdPath + " -D",
	}
	cmds, err := sshCmds()
	c.Assert(err, gocheck.IsNil)
	c.Assert(cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestSSHCmdsDefaultSSHDPath(c *gocheck.C) {
	rfs := ftesting.RecordingFs{}
	f, err := rfs.Create("/opt/me/id_dsa.pub")
	c.Assert(err, gocheck.IsNil)
	f.Write([]byte("ssh-rsa ohwait! me@machine"))
	f.Close()
	old := fsystem
	fsystem = &rfs
	defer func() {
		fsystem = old
	}()
	config.Set("docker:ssh:public-key", "/opt/me/id_dsa.pub")
	defer config.Unset("docker:ssh:public-key")
	commands, err := sshCmds()
	c.Assert(err, gocheck.IsNil)
	c.Assert(commands[1], gocheck.Equals, "/usr/sbin/sshd -D")
}

func (s *S) TestSSHCmdsDefaultKeyFile(c *gocheck.C) {
	rfs := ftesting.RecordingFs{}
	f, err := rfs.Create(os.ExpandEnv("${HOME}/.ssh/id_rsa.pub"))
	c.Assert(err, gocheck.IsNil)
	f.Write([]byte("ssh-rsa ohwait! me@machine"))
	f.Close()
	old := fsystem
	fsystem = &rfs
	defer func() {
		fsystem = old
	}()
	commands, err := sshCmds()
	c.Assert(err, gocheck.IsNil)
	c.Assert(commands[0], gocheck.Equals, "/var/lib/tsuru/add-key ssh-rsa ohwait! me@machine")
}

func (s *S) TestSSHCmdsMissingAddKeyCommand(c *gocheck.C) {
	old, _ := config.Get("docker:ssh:add-key-cmd")
	defer config.Set("docker:ssh:add-key-cmd", old)
	config.Unset("docker:ssh:add-key-cmd")
	commands, err := sshCmds()
	c.Assert(commands, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestSSHCmdsKeyFileNotFound(c *gocheck.C) {
	old := fsystem
	fsystem = &ftesting.RecordingFs{}
	defer func() {
		fsystem = old
	}()
	commands, err := sshCmds()
	c.Assert(commands, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(os.IsNotExist(err), gocheck.Equals, true)
}
