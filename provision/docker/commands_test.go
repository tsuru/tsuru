// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/bind"
	ftesting "github.com/tsuru/tsuru/fs/testing"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/testing"
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
		Value:  "[http://theirproxy.com:3128/, http://teste.com:3111]",
		Public: true,
	}
	app.SetEnv(env)
	deployCmd, err := config.GetString("docker:deploy-cmd")
	c.Assert(err, gocheck.IsNil)
	version := "version"
	appRepo := repository.ReadOnlyURL(app.GetName())
	expected := []string{deployCmd, appRepo, version, `http_proxy=[http://theirproxy.com:3128/,http://teste.com:3111] `}
	cmds, err := deployCmds(app, version)
	c.Assert(err, gocheck.IsNil)
	c.Assert(cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestRunWithAgentCmds(c *gocheck.C) {
	app := testing.NewFakeApp("app-name", "python", 1)
	host_env := bind.EnvVar{
		Name:   "TSURU_HOST",
		Value:  "tsuru_host",
		Public: true,
	}
	token_env := bind.EnvVar{
		Name:   "TSURU_APP_TOKEN",
		Value:  "app_token",
		Public: true,
	}
	app.SetEnv(host_env)
	app.SetEnv(token_env)
	runCmd, err := config.GetString("docker:run-cmd:bin")
	c.Assert(err, gocheck.IsNil)
	unitAgentCmd := fmt.Sprintf("(tsuru_unit_agent tsuru_host app_token app-name %s || %s)", runCmd, runCmd)
	ssh, err := sshCmds()
	sshCmd := strings.Join(ssh, " && ")
	c.Assert(err, gocheck.IsNil)
	cmd := fmt.Sprintf("%s && %s", unitAgentCmd, sshCmd)
	expected := []string{"/bin/bash", "-c", cmd}
	cmds, err := runWithAgentCmds(app)
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
