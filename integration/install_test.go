// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"regexp"
	"strings"
	"time"

	"gopkg.in/check.v1"
)

var (
	T            = NewCommand("tsuru").WithArgs
	allPlatforms = []string{
		"python",
		"go",
		"buildpack",
		"cordova",
		"elixir",
		"java",
		"nodejs",
		"php",
		"play",
		"pypy",
		"python3",
		"ruby",
		"static",
	}
	allProvisioners = []string{
		"docker",
		"swarm",
	}
	flows = []ExecFlow{
		targetTest(),
		loginTest(),
		teamTest(),
		poolAdd(),
		nodeRemove(),
		platformAdd(),
		exampleApps(),
	}
)

func targetTest() ExecFlow {
	flow := ExecFlow{}
	targetName := "integration-target"
	flow.Add(T("target-add", targetName, "{{.targetaddr}}"))
	flow.AddRollback(T("target-remove", targetName))
	flow.Add(T("target-list"), Expected{Stdout: `\s+` + targetName + ` .*`})
	flow.Add(T("target-set", targetName))
	flow.Add(T("target-list"), Expected{Stdout: `\* ` + targetName + ` .*`})
	return flow
}

func loginTest() ExecFlow {
	flow := ExecFlow{}
	flow.Add(T("login", "{{.adminuser}}").WithInput("{{.adminpassword}}"))
	return flow
}

func teamTest() ExecFlow {
	flow := ExecFlow{
		provides: []string{"team"},
	}
	teamName := "integration-team"
	flow.Add(T("team-create", teamName))
	flow.AddHook(func(c *check.C, res *Result) {
		res.Env.Set("team", teamName)
	})
	flow.AddRollback(T("team-remove", "-y", teamName))
	return flow
}

func poolAdd() ExecFlow {
	flow := ExecFlow{
		provides: []string{"poolnames"},
	}
	for _, prov := range allProvisioners {
		poolName := "ipool-" + prov
		flow.Add(T("pool-add", "--provisioner", prov, poolName))
		flow.AddHook(func(c *check.C, res *Result) {
			res.Env.Add("poolnames", poolName)
		})
		flow.AddRollback(T("pool-remove", "-y", poolName))
		flow.Add(T("pool-teams-add", poolName, "{{.team}}"))
		flow.AddRollback(T("pool-teams-remove", poolName, "{{.team}}"))
		flow.Add(T("node-add", "{{.nodeopts}}", "pool="+poolName))
		flow.Add(T("event-list"))
		flow.AddHook(func(c *check.C, res *Result) {
			nodeopts := res.Env.All("nodeopts")
			res.Env.Set("nodeopts", append(nodeopts[1:], nodeopts[0])...)
			regex := regexp.MustCompile(`node.create.*?node:\s+(.*?)\s+`)
			parts := regex.FindStringSubmatch(res.Stdout.String())
			c.Assert(parts, check.HasLen, 2)
			res.Env.Add("nodeaddrs", parts[1])
			res.Env.Set("nodeaddr", parts[1])
		})
		flow.AddHook(func(c *check.C, res *Result) {
			timeout := time.After(time.Minute)
			regex := regexp.MustCompile(res.Env.Get("nodeaddr") + `.*?ready`)
			for {
				res = T("node-list").Run(res.Env)
				if regex.MatchString(res.Stdout.String()) {
					return
				}
				select {
				case <-time.After(time.Second):
				case <-timeout:
					c.Fatalf("node %q not ready after 1 minute", res.Env.Get("nodeaddr"))
					return
				}
			}
		})
	}
	return flow
}

func nodeRemove() ExecFlow {
	flow := ExecFlow{
		matrix: map[string]string{
			"node": "nodeaddrs",
		},
	}
	flow.AddRollback(T("node-remove", "-y", "--no-rebalance", "{{.node}}"))
	return flow
}

func platformAdd() ExecFlow {
	flow := ExecFlow{
		provides: []string{"platforms"},
	}
	for _, plat := range allPlatforms {
		integrationPlat := "iplat-" + plat
		flow.Add(T("platform-add", integrationPlat, "-i", "tsuru/"+plat))
		flow.AddHook(func(c *check.C, res *Result) {
			res.Env.Add("platforms", integrationPlat)
		})
		flow.AddRollback(T("platform-remove", "-y", integrationPlat))
		flow.Add(T("platform-list"), Expected{Stdout: "(?s).*- " + integrationPlat + ".*"})
	}
	return flow
}

func exampleApps() ExecFlow {
	flow := ExecFlow{
		matrix: map[string]string{
			"pool": "poolnames",
			"plat": "platforms",
		},
	}
	appName := "iapp-{{.plat}}-{{.pool}}"
	flow.Add(T("app-create", appName, "{{.plat}}", "-t", "{{.team}}", "-o", "{{.pool}}"))
	flow.AddRollback(T("app-remove", "-y", "-a", appName))
	flow.Add(T("app-info", "-a", appName))
	flow.AddHook(func(c *check.C, res *Result) {
		platRE := regexp.MustCompile(`(?s)Platform: (.*?)\n`)
		parts := platRE.FindStringSubmatch(res.Stdout.String())
		c.Assert(parts, check.HasLen, 2)
		res.Env.Set("language", strings.Replace(parts[1], "iplat-", "", -1))
	})
	flow.Add(T("app-deploy", "-a", appName, "{{.examplesdir}}/{{.language}}"))
	flow.Add(T("app-info", "-a", appName))
	flow.AddHook(func(c *check.C, res *Result) {
		addrRE := regexp.MustCompile(`(?s)Address: (.*?)\n`)
		parts := addrRE.FindStringSubmatch(res.Stdout.String())
		c.Assert(parts, check.HasLen, 2)
		res.Env.Set("appaddr", parts[1])
	})
	flow.Add(NewCommand("curl", "-sSf", "http://{{.appaddr}}"))
	return flow
}

func (s *S) TestBase(c *check.C) {
	env := NewEnvironment()
	if !env.Has("targetaddr") {
		return
	}
	var executedFlows []*ExecFlow
	defer func() {
		for i := len(executedFlows) - 1; i >= 0; i-- {
			executedFlows[i].Rollback(c, env)
		}
	}()
	for i := range flows {
		f := &flows[i]
		if len(f.provides) > 0 {
			providesAll := true
			for _, envVar := range f.provides {
				if env.Get(envVar) == "" {
					providesAll = false
					break
				}
			}
			if providesAll {
				continue
			}
		}
		executedFlows = append(executedFlows, f)
		f.Run(c, env)
	}
}
