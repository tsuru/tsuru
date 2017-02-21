// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"time"

	"gopkg.in/check.v1"
)

var (
	T            = NewCommand("tsuru").WithArgs
	allPlatforms = []string{
		"tsuru/python",
		"tsuru/go",
		"tsuru/buildpack",
		"tsuru/cordova",
		"tsuru/elixir",
		"tsuru/java",
		"tsuru/nodejs",
		"tsuru/php",
		"tsuru/play",
		"tsuru/pypy",
		"tsuru/python3",
		"tsuru/ruby",
		"tsuru/static",
	}
	allProvisioners = []string{
		"docker",
		"swarm",
	}
	flows = []ExecFlow{
		platformsToInstall(),
		installerConfigTest(),
		installerTest(),
		targetTest(),
		loginTest(),
		removeInstallNodes(),
		quotaTest(),
		teamTest(),
		poolAdd(),
		nodeRemove(),
		platformAdd(),
		exampleApps(),
	}
)

var installerConfig = `driver:
  name: virtualbox
  options:
    virtualbox-cpu-count: 2
    virtualbox-memory: 2048
hosts:
  apps:
    size: 2
components:
  tsuru:
    version: latest
    install-dashboard: false
`

func platformsToInstall() ExecFlow {
	flow := ExecFlow{
		provides: []string{"platformimages"},
	}
	flow.AddHook(func(c *check.C, res *Result) {
		for _, platImg := range allPlatforms {
			res.Env.Add("platformimages", platImg)
		}
	})
	return flow
}

func installerConfigTest() ExecFlow {
	flow := ExecFlow{
		provides: []string{"installerconfig"},
	}
	flow.AddHook(func(c *check.C, res *Result) {
		f, err := ioutil.TempFile("", "installer-config")
		c.Assert(err, check.IsNil)
		defer f.Close()
		f.Write([]byte(installerConfig))
		res.Env.Set("installerconfig", f.Name())
	})
	flow.AddRollback(NewCommand("rm", "{{.installerconfig}}"))
	return flow
}

func installerTest() ExecFlow {
	flow := ExecFlow{
		provides: []string{"targetaddr"},
	}
	flow.Add(T("install", "--config", "{{.installerconfig}}").WithTimeout(9 * time.Minute))
	flow.AddRollback(T("uninstall", "-y"))
	flow.AddHook(func(c *check.C, res *Result) {
		regex := regexp.MustCompile(`(?si).*Core Hosts:.*?([\d.]+)\s.*`)
		parts := regex.FindStringSubmatch(res.Stdout.String())
		c.Assert(parts, check.HasLen, 2)
		targetHost := parts[1]
		regex = regexp.MustCompile(`(?si).*Tsuru API.*?\|\s(\d+)`)
		parts = regex.FindStringSubmatch(res.Stdout.String())
		c.Assert(parts, check.HasLen, 2)
		targetPort := parts[1]
		res.Env.Set("targetaddr", fmt.Sprintf("http://%s:%s", targetHost, targetPort))
		regex = regexp.MustCompile(`\| (https?[^\s]+?) \|`)
		allParts := regex.FindAllStringSubmatch(res.Stdout.String(), -1)
		for _, parts = range allParts {
			c.Assert(parts, check.HasLen, 2)
			res.Env.Add("nodeopts", fmt.Sprintf("--register address=%s --cacert ~/.tsuru/installs/tsuru/certs/ca.pem --clientcert ~/.tsuru/installs/tsuru/certs/cert.pem --clientkey ~/.tsuru/installs/tsuru/certs/key.pem", parts[1]))
			res.Env.Add("nodestoremove", parts[1])
		}
		regex = regexp.MustCompile(`Username: (.+)`)
		parts = regex.FindStringSubmatch(res.Stdout.String())
		res.Env.Set("adminuser", parts[1])
		regex = regexp.MustCompile(`Password: (.+)`)
		parts = regex.FindStringSubmatch(res.Stdout.String())
		res.Env.Set("adminpassword", parts[1])
	})
	return flow
}

func targetTest() ExecFlow {
	flow := ExecFlow{}
	targetName := "integration-target"
	flow.Add(T("target-add", targetName, "{{.targetaddr}}"))
	flow.Add(T("target-list"), Expected{Stdout: `\s+` + targetName + ` .*`})
	flow.Add(T("target-set", targetName))
	return flow
}

func loginTest() ExecFlow {
	flow := ExecFlow{}
	flow.Add(T("login", "{{.adminuser}}").WithInput("{{.adminpassword}}"))
	return flow
}

func removeInstallNodes() ExecFlow {
	flow := ExecFlow{
		matrix: map[string]string{
			"node": "nodestoremove",
		},
	}
	flow.Add(T("node-remove", "-y", "--no-rebalance", "{{.node}}"))
	return flow
}

func quotaTest() ExecFlow {
	flow := ExecFlow{}
	flow.Add(T("user-quota-change", "{{.adminuser}}", "100"))
	flow.Add(T("user-quota-view", "{{.adminuser}}"), Expected{Stdout: `(?s)Apps usage.*/100`})
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
			regex = regexp.MustCompile(parts[1] + `.*?ready`)
			ok := retry(time.Minute, func() bool {
				res = T("node-list").Run(res.Env)
				return regex.MatchString(res.Stdout.String())
			})
			c.Assert(ok, check.Equals, true, check.Commentf("node not ready after 1 minute: %v", res))
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
		matrix: map[string]string{
			"platimg": "platformimages",
		},
	}
	flow.AddHook(func(c *check.C, res *Result) {
		img := res.Env.Get("platimg")
		res.Env.Set("platimgsuffix", img[strings.LastIndex(img, "/")+1:])
	})
	flow.Add(T("platform-add", "iplat-{{.platimgsuffix}}", "-i", "{{.platimg}}"))
	flow.AddHook(func(c *check.C, res *Result) {
		res.Env.Add("platforms", "iplat-"+res.Env.Get("platimgsuffix"))
	})
	flow.AddRollback(T("platform-remove", "-y", "iplat-{{.platimgsuffix}}"))
	flow.Add(T("platform-list"), Expected{Stdout: "(?s).*- iplat-{{.platimgsuffix}}.*"})
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
	flow.AddHook(func(c *check.C, res *Result) {
		cmd := NewCommand("curl", "-sSf", "http://{{.appaddr}}")
		ok := retry(time.Minute, func() bool {
			res = cmd.Run(res.Env)
			return res.ExitCode == 0
		})
		c.Assert(ok, check.Equals, true, check.Commentf("invalid result: %v", res))
	})
	return flow
}

func (s *S) TestBase(c *check.C) {
	env := NewEnvironment()
	if !env.Has("enabled") {
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
