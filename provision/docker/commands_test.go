// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app/bind"
	ftesting "github.com/globocom/tsuru/fs/testing"
	"github.com/globocom/tsuru/repository"
	"github.com/globocom/tsuru/testing"
	"launchpad.net/gocheck"
	"os"
	"strings"
)

func (s *S) TestDeployCmds(c *gocheck.C) {
	h := &testing.TestHandler{}
	gandalfServer := testing.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	app := testing.NewFakeApp("app-name", "python", 1)
	env := bind.EnvVar{
		Name:   "http_proxy",
		Value:  "http://theirproxy.com:3128/",
		Public: true,
	}
	app.SetEnv(env)
	deployCmd, err := config.GetString("docker:deploy-cmd")
	c.Assert(err, gocheck.IsNil)
	version := "version"
	appRepo := repository.ReadOnlyURL(app.GetName())
	expected := []string{deployCmd, appRepo, version, `http_proxy=http://theirproxy.com:3128/ `}
	cmds, err := deployCmds(app, version)
	c.Assert(err, gocheck.IsNil)
	c.Assert(cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestRunCmds(c *gocheck.C) {
	runCmd, err := config.GetString("docker:run-cmd:bin")
	c.Assert(err, gocheck.IsNil)
	ssh, err := sshCmds()
	sshCmd := strings.Join(ssh, " && ")
	c.Assert(err, gocheck.IsNil)
	cmd := fmt.Sprintf("%s && %s", runCmd, sshCmd)
	expected := []string{"/bin/bash", "-c", cmd}
	cmds, err := runCmds()
	c.Assert(err, gocheck.IsNil)
	c.Assert(cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestSSHCmds(c *gocheck.C) {
	addKeyCommand, err := config.GetString("docker:ssh:add-key-cmd")
	c.Assert(err, gocheck.IsNil)
	keyContent := "key-content"
	sshdPath := "sudo /usr/sbin/sshd"
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
	c.Assert(commands[1], gocheck.Equals, "sudo /usr/sbin/sshd -D")
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
