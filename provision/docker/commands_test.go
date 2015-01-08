// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/testing"
	"launchpad.net/gocheck"
)

func (s *S) TestGitDeployCmds(c *gocheck.C) {
	h := &testing.TestHandler{}
	h.Content = `{"git_url":"git://something/app-name.git"}`
	gandalfServer := testing.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	app := testing.NewFakeApp("app-name", "python", 1)
	env := bind.EnvVar{
		Name:   "http_proxy",
		Value:  "[http://theirproxy.com:3128/, http://teste.com:3111]",
		Public: true,
	}
	app.SetEnv(env)
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
	deployCmd, err := config.GetString("docker:deploy-cmd")
	c.Assert(err, gocheck.IsNil)
	expectedPart1 := fmt.Sprintf("%s git git://something/app-name.git version", deployCmd)
	expectedAgent := fmt.Sprintf(`tsuru_unit_agent tsuru_host app_token app-name "%s" deploy`, expectedPart1)
	expectedCmd := fmt.Sprintf("if [[ $(tsuru_unit_agent --help | head -n1 | grep deploy) ]]; then %s; else %s", expectedAgent, expectedPart1)
	cmds, err := gitDeployCmds(app, "version")
	c.Assert(err, gocheck.IsNil)
	c.Assert(cmds, gocheck.HasLen, 3)
	c.Assert(cmds[0], gocheck.Equals, "/bin/bash")
	c.Assert(cmds[1], gocheck.Equals, "-lc")
	c.Assert(strings.HasPrefix(cmds[2], expectedCmd), gocheck.Equals, true)
	expectedVars := []string{"http_proxy=[http://theirproxy.com:3128/,http://teste.com:3111]", "TSURU_APP_TOKEN=app_token", "TSURU_HOST=tsuru_host"}
	sort.Strings(expectedVars)
	varsPart := strings.TrimSpace(strings.Replace(strings.Replace(cmds[2], expectedCmd, "", 1), "; fi", "", 1))
	cmdVars := strings.Split(varsPart, " ")
	sort.Strings(cmdVars)
	c.Assert(cmdVars, gocheck.DeepEquals, expectedVars)
}

func (s *S) TestArchiveDeployCmds(c *gocheck.C) {
	app := testing.NewFakeApp("app-name", "python", 1)
	env := bind.EnvVar{
		Name:   "http_proxy",
		Value:  "[http://theirproxy.com:3128/, http://teste.com:3111]",
		Public: true,
	}
	app.SetEnv(env)
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
	deployCmd, err := config.GetString("docker:deploy-cmd")
	c.Assert(err, gocheck.IsNil)
	archiveURL := "https://s3.amazonaws.com/wat/archive.tar.gz"
	expectedPart1 := fmt.Sprintf("%s archive %s", deployCmd, archiveURL)
	expectedAgent := fmt.Sprintf(`tsuru_unit_agent tsuru_host app_token app-name "%s" deploy`, expectedPart1)
	expectedCmd := fmt.Sprintf("if [[ $(tsuru_unit_agent --help | head -n1 | grep deploy) ]]; then %s; else %s", expectedAgent, expectedPart1)
	cmds, err := archiveDeployCmds(app, archiveURL)
	c.Assert(err, gocheck.IsNil)
	c.Assert(cmds, gocheck.HasLen, 3)
	c.Assert(cmds[0], gocheck.Equals, "/bin/bash")
	c.Assert(cmds[1], gocheck.Equals, "-lc")
	c.Assert(strings.HasPrefix(cmds[2], expectedCmd), gocheck.Equals, true)
	expectedVars := []string{"http_proxy=[http://theirproxy.com:3128/,http://teste.com:3111]", "TSURU_APP_TOKEN=app_token", "TSURU_HOST=tsuru_host"}
	sort.Strings(expectedVars)
	varsPart := strings.TrimSpace(strings.Replace(strings.Replace(cmds[2], expectedCmd, "", 1), "; fi", "", 1))
	cmdVars := strings.Split(varsPart, " ")
	sort.Strings(cmdVars)
	c.Assert(cmdVars, gocheck.DeepEquals, expectedVars)
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
	cmd := fmt.Sprintf("%s", unitAgentCmd)
	expected := []string{"/bin/bash", "-lc", cmd}
	cmds, err := runWithAgentCmds(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(cmds, gocheck.DeepEquals, expected)
}
