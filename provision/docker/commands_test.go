// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/testing"
	"launchpad.net/gocheck"
)

func (s *S) TestDeployCmds(c *gocheck.C) {
	app := testing.NewFakeApp("app-name", "python", 1)
	cmds, err := deployCmds(app)
	c.Assert(err, gocheck.IsNil)
	docker, err := config.GetString("docker:binary")
	c.Assert(err, gocheck.IsNil)
	deployCmd, err := config.GetString("docker:deploy-cmd")
	c.Assert(err, gocheck.IsNil)
	imageName := getImage(app)
	expected := []string{docker, "run", imageName, deployCmd}
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
