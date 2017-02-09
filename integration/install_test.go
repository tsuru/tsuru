// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"regexp"

	"gopkg.in/check.v1"
)

type postFunc func(c *check.C, res *Result, env *Environment)

type CmdWithExp struct {
	C     *Command
	E     Expected
	P     postFunc
	Defer bool
}

type CmdList []CmdWithExp

var T = NewCommand("tsuru").WithArgs

var afterInstallFlow = CmdList{
	{C: T("target-add", "integration", "{{.targetaddr}}")},
	{C: T("target-list"), E: Expected{Stdout: `\s+integration .*`}},
	{C: T("target-set", "integration")},
	{C: T("target-list"), E: Expected{Stdout: `\* integration .*`}},

	{C: T("app-list"), E: Expected{ExitCode: 1, Stderr: `.*you're not authenticated.*`}},
	{C: T("login", "{{.adminuser}}").WithInput("{{.adminpassword}}")},

	{C: T("team-create", "integration-team")},
	{Defer: true, C: T("team-remove", "-y", "integration-team")},
	{C: T("team-list"), E: Expected{Stdout: `(?s).*integration-team.*`}},
	{C: T("pool-add", "--provisioner", "docker", "integration-pool-docker")},
	{Defer: true, C: T("pool-remove", "-y", "integration-pool-docker")},
	{C: T("pool-add", "--provisioner", "swarm", "integration-pool-swarm")},
	{Defer: true, C: T("pool-remove", "-y", "integration-pool-swarm")},

	{C: T("pool-teams-add", "integration-pool-docker", "integration-team")},
	{Defer: true, C: T("pool-teams-remove", "integration-pool-docker", "integration-team")},
	{C: T("pool-teams-add", "integration-pool-swarm", "integration-team")},
	{Defer: true, C: T("pool-teams-remove", "integration-pool-swarm", "integration-team")},

	{C: T("platform-add", "integration-python", "-i", "tsuru/python")},
	{Defer: true, C: T("platform-remove", "-y", "integration-python")},
	{C: T("platform-list"), E: Expected{Stdout: "- integration-python"}},

	{C: T("node-update", "{{.nodeaddr}}", "pool=integration-pool-docker")},

	{C: T("app-create", "integration-app-python", "integration-python",
		"-t", "integration-team", "-o", "integration-pool-docker")},
	{Defer: true, C: T("app-remove", "-y", "-a", "integration-app-python")},
	{C: T("app-deploy", "-a", "integration-app-python", "{{.examplesdir}}/python")},
	{C: T("app-info", "-a", "integration-app-python"), P: func(c *check.C, res *Result, env *Environment) {
		addrRE := regexp.MustCompile(`(?s)Address: (.*?)\n`)
		parts := addrRE.FindStringSubmatch(res.Stdout.String())
		env.Set("appaddr", parts[1])
	}},
	{C: NewCommand("curl", "-sSf", "http://{{.appaddr}}")},
}

func (s *S) TestBase(c *check.C) {
	env := NewEnvironment()
	if env.Get("targetaddr") == "" {
		return
	}
	for _, f := range afterInstallFlow {
		if f.Defer {
			defer func(cmd *Command) {
				res := cmd.Run(env)
				c.Check(res, ResultOk)
			}(f.C)
			continue
		}
		res := f.C.Run(env)
		c.Assert(res, ResultMatches, f.E)
		if f.P != nil {
			f.P(c, res, env)
		}
	}
}
