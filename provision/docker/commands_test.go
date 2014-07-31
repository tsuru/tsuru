// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/testing"
	"launchpad.net/gocheck"
	"strings"
)

func (s *S) TestGitDeployCmds(c *gocheck.C) {
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
	expected := []string{deployCmd, "git", appRepo, version, "http_proxy=[http://theirproxy.com:3128/,http://teste.com:3111] "}
	cmds, err := gitDeployCmds(app, version)
	c.Assert(err, gocheck.IsNil)
	c.Assert(cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestArchiveDeployCmds(c *gocheck.C) {
	app := testing.NewFakeApp("app-name", "python", 1)
	env := bind.EnvVar{
		Name:   "http_proxy",
		Value:  "[http://theirproxy.com:3128/, http://teste.com:3111]",
		Public: true,
	}
	app.SetEnv(env)
	deployCmd, err := config.GetString("docker:deploy-cmd")
	c.Assert(err, gocheck.IsNil)
	archiveURL := "https://s3.amazonaws.com/wat/archive.tar.gz"
	expected := []string{
		deployCmd, "archive", archiveURL,
		"http_proxy=[http://theirproxy.com:3128/,http://teste.com:3111] ",
	}
	cmds, err := archiveDeployCmds(app, archiveURL)
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
	unitAgentCmd := fmt.Sprintf("tsuru_unit_agent tsuru_host app_token app-name %s", runCmd)
	key := []byte("key-content")
	ssh, err := sshCmds(key)
	sshCmd := strings.Join(ssh, " && ")
	c.Assert(err, gocheck.IsNil)
	cmd := fmt.Sprintf("%s && %s", unitAgentCmd, sshCmd)
	expected := []string{"/bin/bash", "-c", cmd}
	cmds, err := runWithAgentCmds(app, key)
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
	cmds, err := sshCmds([]byte(keyContent))
	c.Assert(err, gocheck.IsNil)
	c.Assert(cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestSSHCmdsDefaultSSHDPath(c *gocheck.C) {
	keyContent := []byte("ssh-rsa ohwait! me@machine")
	commands, err := sshCmds(keyContent)
	c.Assert(err, gocheck.IsNil)
	c.Assert(commands[1], gocheck.Equals, "sudo /usr/sbin/sshd -D")
}

func (s *S) TestSSHCmdsMissingAddKeyCommand(c *gocheck.C) {
	old, _ := config.Get("docker:ssh:add-key-cmd")
	defer config.Set("docker:ssh:add-key-cmd", old)
	config.Unset("docker:ssh:add-key-cmd")
	commands, err := sshCmds([]byte("mykey"))
	c.Assert(commands, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
}
