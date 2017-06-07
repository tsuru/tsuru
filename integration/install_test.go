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

	"os"

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
	clusterProvisioners = []clusterManager{
		&kubernetesClusterManager{},
	}
	flows = []ExecFlow{
		platformsToInstall(),
		installerConfigTest(),
		installerComposeTest(),
		installerTest(),
		targetTest(),
		loginTest(),
		removeInstallNodes(),
		quotaTest(),
		teamTest(),
		poolAdd(),
		platformAdd(),
		exampleApps(),
	}
)

var installerConfig = fmt.Sprintf(`driver:
  name: virtualbox
  options:
    virtualbox-cpu-count: 2
    virtualbox-memory: 2048
docker-flags:
  - experimental
hosts:
  apps:
    size: %d
components:
  install-dashboard: false
`, len(allProvisioners)+len(clusterProvisioners))

func platformsToInstall() ExecFlow {
	flow := ExecFlow{
		provides: []string{"platformimages"},
	}
	flow.forward = func(c *check.C, env *Environment) {
		for _, platImg := range allPlatforms {
			env.Add("platformimages", platImg)
		}
	}
	return flow
}

func installerConfigTest() ExecFlow {
	flow := ExecFlow{
		provides: []string{"installerconfig"},
	}
	flow.forward = func(c *check.C, env *Environment) {
		f, err := ioutil.TempFile("", "installer-config")
		c.Assert(err, check.IsNil)
		defer f.Close()
		f.Write([]byte(installerConfig))
		c.Assert(err, check.IsNil)
		env.Set("installerconfig", f.Name())
	}
	flow.backward = func(c *check.C, env *Environment) {
		res := NewCommand("rm", "{{.installerconfig}}").Run(env)
		c.Check(res, ResultOk)
	}
	return flow
}

func installerComposeTest() ExecFlow {
	flow := ExecFlow{
		provides: []string{"installercompose"},
	}
	flow.forward = func(c *check.C, env *Environment) {
		composeFile, err := ioutil.TempFile("", "installer-compose")
		c.Assert(err, check.IsNil)
		defer composeFile.Close()
		f, err := ioutil.TempFile("", "installer-config")
		c.Assert(err, check.IsNil)
		defer func() {
			res := NewCommand("rm", f.Name()).Run(env)
			c.Check(res, ResultOk)
			f.Close()
		}()
		res := T("install-config-init", f.Name(), composeFile.Name()).Run(env)
		c.Assert(res, ResultOk)
		composeData, err := ioutil.ReadFile(composeFile.Name())
		c.Assert(err, check.IsNil)
		composeData = []byte(strings.Replace(string(composeData), "tsuru/api:v1", "tsuru/api:latest", 1))
		err = ioutil.WriteFile(composeFile.Name(), composeData, 0644)
		c.Assert(err, check.IsNil)
		env.Set("installercompose", composeFile.Name())
	}
	flow.backward = func(c *check.C, env *Environment) {
		res := NewCommand("rm", "{{.installercompose}}").Run(env)
		c.Check(res, ResultOk)
	}
	return flow
}

func installerTest() ExecFlow {
	flow := ExecFlow{
		provides: []string{"targetaddr"},
	}
	flow.forward = func(c *check.C, env *Environment) {
		res := T("install-create", "--config", "{{.installerconfig}}", "--compose", "{{.installercompose}}").WithTimeout(60 * time.Minute).Run(env)
		c.Assert(res, ResultOk)
		regex := regexp.MustCompile(`(?si).*Core Hosts:.*?([\d.]+)\s.*`)
		parts := regex.FindStringSubmatch(res.Stdout.String())
		c.Assert(parts, check.HasLen, 2)
		targetHost := parts[1]
		regex = regexp.MustCompile(`(?si).*tsuru_tsuru.*?\|\s(\d+)`)
		parts = regex.FindStringSubmatch(res.Stdout.String())
		c.Assert(parts, check.HasLen, 2)
		targetPort := parts[1]
		env.Set("targetaddr", fmt.Sprintf("http://%s:%s", targetHost, targetPort))
		regex = regexp.MustCompile(`\| (https?[^\s]+?) \|`)
		allParts := regex.FindAllStringSubmatch(res.Stdout.String(), -1)
		certsDir := fmt.Sprintf("%s/.tsuru/installs/tsuru/certs", os.Getenv("HOME"))
		for _, parts = range allParts {
			c.Assert(parts, check.HasLen, 2)
			env.Add("nodeopts", fmt.Sprintf("--register address=%s --cacert %s/ca.pem --clientcert %s/cert.pem --clientkey %s/key.pem", parts[1], certsDir, certsDir, certsDir))
			env.Add("installernodes", parts[1])
		}
		regex = regexp.MustCompile(`Username: ([[:print:]]+)`)
		parts = regex.FindStringSubmatch(res.Stdout.String())
		env.Set("adminuser", parts[1])
		regex = regexp.MustCompile(`Password: ([[:print:]]+)`)
		parts = regex.FindStringSubmatch(res.Stdout.String())
		env.Set("adminpassword", parts[1])
	}
	flow.backward = func(c *check.C, env *Environment) {
		res := T("install-remove", "--config", "{{.installerconfig}}", "-y").Run(env)
		c.Check(res, ResultOk)
	}
	return flow
}

func targetTest() ExecFlow {
	flow := ExecFlow{}
	flow.forward = func(c *check.C, env *Environment) {
		targetName := "integration-target"
		res := T("target-add", targetName, "{{.targetaddr}}").Run(env)
		c.Assert(res, ResultOk)
		res = T("target-list").Run(env)
		c.Assert(res, ResultMatches, Expected{Stdout: `\s+` + targetName + ` .*`})
		res = T("target-set", targetName).Run(env)
		c.Assert(res, ResultOk)
	}
	return flow
}

func loginTest() ExecFlow {
	flow := ExecFlow{}
	flow.forward = func(c *check.C, env *Environment) {
		res := T("login", "{{.adminuser}}").WithInput("{{.adminpassword}}").Run(env)
		c.Assert(res, ResultOk)
	}
	return flow
}

func removeInstallNodes() ExecFlow {
	flow := ExecFlow{
		matrix: map[string]string{
			"node": "installernodes",
		},
	}
	flow.forward = func(c *check.C, env *Environment) {
		res := T("node-remove", "-y", "--no-rebalance", "{{.node}}").Run(env)
		c.Assert(res, ResultOk)
	}
	return flow
}

func quotaTest() ExecFlow {
	flow := ExecFlow{
		requires: []string{"adminuser"},
	}
	flow.forward = func(c *check.C, env *Environment) {
		res := T("user-quota-change", "{{.adminuser}}", "100").Run(env)
		c.Assert(res, ResultOk)
		res = T("user-quota-view", "{{.adminuser}}").Run(env)
		c.Assert(res, ResultMatches, Expected{Stdout: `(?s)Apps usage.*/100`})
	}
	return flow
}

func teamTest() ExecFlow {
	flow := ExecFlow{
		provides: []string{"team"},
	}
	teamName := "integration-team"
	flow.forward = func(c *check.C, env *Environment) {
		res := T("team-create", teamName).Run(env)
		c.Assert(res, ResultOk)
		env.Set("team", teamName)
	}
	flow.backward = func(c *check.C, env *Environment) {
		res := T("team-remove", "-y", teamName).Run(env)
		c.Check(res, ResultOk)
	}
	return flow
}

func poolAdd() ExecFlow {
	flow := ExecFlow{
		provides: []string{"poolnames"},
	}
	flow.forward = func(c *check.C, env *Environment) {
		for _, prov := range allProvisioners {
			poolName := "ipool-" + prov
			res := T("pool-add", "--provisioner", prov, poolName).Run(env)
			c.Assert(res, ResultOk)
			env.Add("poolnames", poolName)
			res = T("pool-constraint-set", poolName, "team", "{{.team}}").Run(env)
			c.Assert(res, ResultOk)
			res = T("node-add", "{{.nodeopts}}", "pool="+poolName).Run(env)
			c.Assert(res, ResultOk)
			res = T("event-list").Run(env)
			c.Assert(res, ResultOk)
			nodeopts := env.All("nodeopts")
			env.Set("nodeopts", append(nodeopts[1:], nodeopts[0])...)
			regex := regexp.MustCompile(`node.create.*?node:\s+(.*?)\s+`)
			parts := regex.FindStringSubmatch(res.Stdout.String())
			c.Assert(parts, check.HasLen, 2)
			env.Add("nodeaddrs", parts[1])
			regex = regexp.MustCompile(parts[1] + `.*?ready`)
			ok := retry(time.Minute, func() bool {
				res = T("node-list").Run(env)
				return regex.MatchString(res.Stdout.String())
			})
			c.Assert(ok, check.Equals, true, check.Commentf("node not ready after 1 minute: %v", res))
		}
		for _, cluster := range clusterProvisioners {
			poolName := "ipool-" + cluster.Provisioner()
			res := T("pool-add", "--provisioner", cluster.Provisioner(), poolName).Run(env)
			c.Assert(res, ResultOk)
			env.Add("poolName", poolName)
			res = T("pool-constraint-set", poolName, "team", "{{.team}}").Run(env)
			c.Assert(res, ResultOk)
			res = cluster.Start(env)
			c.Assert(res, ResultOk)
			clusterName := "icluster-" + cluster.Provisioner()
			certFiles := cluster.CertificateFiles()
			res = T("cluster-update", clusterName, cluster.Provisioner(), "--addr", cluster.Address(env), "--cacert", certFiles["cacert"], "--clientcert", certFiles["clientcert"], "--clientkey", certFiles["clientkey"], "--pool", poolName).Run(env)
			c.Assert(res, ResultOk)
			T("cluster-list").Run(env)
			regex := regexp.MustCompile(cluster.IP(env) + `.*?Ready`)
			ok := retry(time.Minute, func() bool {
				res = T("node-list").Run(env)
				return regex.MatchString(res.Stdout.String())
			})
			c.Assert(ok, check.Equals, true, check.Commentf("node not ready after 1 minute: %v", res))
			res = T("node-update", cluster.IP(env), "pool="+poolName).Run(env)
			c.Assert(res, ResultOk)
			res = T("event-list").Run(env)
			c.Assert(res, ResultOk)
			nodeopts := env.All("nodeopts")
			env.Set("nodeopts", append(nodeopts[1:], nodeopts[0])...)
			regex = regexp.MustCompile(`node.update.*?node:\s+` + cluster.IP(env))
			c.Assert(regex.MatchString(res.Stdout.String()), check.Equals, true)
			regex = regexp.MustCompile(cluster.IP(env) + `.*?Ready`)
			ok = retry(time.Minute, func() bool {
				res = T("node-list").Run(env)
				return regex.MatchString(res.Stdout.String())
			})
			c.Assert(ok, check.Equals, true, check.Commentf("node not ready after 1 minute: %v", res))
		}
	}
	flow.backward = func(c *check.C, env *Environment) {
		for _, cluster := range clusterProvisioners {
			res := T("cluster-remove", "icluster-"+cluster.Provisioner()).Run(env)
			c.Check(res, ResultOk)
			res = cluster.Delete(env)
			c.Check(res, ResultOk)
		}
		for _, node := range env.All("nodeaddrs") {
			res := T("node-remove", "-y", "--no-rebalance", node).Run(env)
			c.Check(res, ResultOk)
		}
		for _, prov := range allProvisioners {
			poolName := "ipool-" + prov
			res := T("pool-constraint-set", poolName, "team", "{{.team}} --blacklist").Run(env)
			c.Check(res, ResultOk)
			res = T("pool-remove", "-y", poolName).Run(env)
			c.Check(res, ResultOk)
		}
	}
	return flow
}

func platformAdd() ExecFlow {
	flow := ExecFlow{
		provides: []string{"platforms"},
		matrix: map[string]string{
			"platimg": "platformimages",
		},
		parallel: true,
	}
	flow.forward = func(c *check.C, env *Environment) {
		img := env.Get("platimg")
		suffix := img[strings.LastIndex(img, "/")+1:]
		platName := "iplat-" + suffix
		res := T("platform-add", platName, "-i", img).WithTimeout(15 * time.Minute).Run(env)
		c.Assert(res, ResultOk)
		env.Add("platforms", platName)
		res = T("platform-list").Run(env)
		c.Assert(res, ResultOk)
		c.Assert(res, ResultMatches, Expected{Stdout: "(?s).*- " + platName + ".*"})
	}
	flow.backward = func(c *check.C, env *Environment) {
		img := env.Get("platimg")
		suffix := img[strings.LastIndex(img, "/")+1:]
		platName := "iplat-" + suffix
		res := T("platform-remove", "-y", platName).Run(env)
		c.Check(res, ResultOk)
	}
	return flow
}

func exampleApps() ExecFlow {
	flow := ExecFlow{
		matrix: map[string]string{
			"pool": "poolnames",
			"plat": "platforms",
		},
		parallel: true,
	}
	appName := "iapp-{{.plat}}-{{.pool}}"
	flow.forward = func(c *check.C, env *Environment) {
		res := T("app-create", appName, "{{.plat}}", "-t", "{{.team}}", "-o", "{{.pool}}").Run(env)
		c.Assert(res, ResultOk)
		res = T("app-info", "-a", appName).Run(env)
		c.Assert(res, ResultOk)
		platRE := regexp.MustCompile(`(?s)Platform: (.*?)\n`)
		parts := platRE.FindStringSubmatch(res.Stdout.String())
		c.Assert(parts, check.HasLen, 2)
		lang := strings.Replace(parts[1], "iplat-", "", -1)
		res = T("app-deploy", "-a", appName, "{{.examplesdir}}/"+lang+"/").Run(env)
		c.Assert(res, ResultOk)
		regex := regexp.MustCompile("started")
		ok := retry(time.Minute, func() bool {
			res = T("app-info", "-a", appName).Run(env)
			c.Assert(res, ResultOk)
			return regex.MatchString(res.Stdout.String())
		})
		c.Assert(ok, check.Equals, true, check.Commentf("app not ready after 1 minute: %v", res))
		addrRE := regexp.MustCompile(`(?s)Address: (.*?)\n`)
		parts = addrRE.FindStringSubmatch(res.Stdout.String())
		c.Assert(parts, check.HasLen, 2)
		cmd := NewCommand("curl", "-sSf", "http://"+parts[1])
		ok = retry(15*time.Minute, func() bool {
			res = cmd.Run(env)
			return res.ExitCode == 0
		})
		c.Assert(ok, check.Equals, true, check.Commentf("invalid result: %v", res))
	}
	flow.backward = func(c *check.C, env *Environment) {
		res := T("app-remove", "-y", "-a", appName).Run(env)
		c.Check(res, ResultOk)
	}
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
