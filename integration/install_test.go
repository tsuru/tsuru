// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/check.v1"
)

var (
	T               = NewCommand("tsuru").WithArgs
	platforms       = []string{}
	provisioners    = []string{}
	clusterManagers = []ClusterManager{}
	flows           = []ExecFlow{
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
		serviceImageSetup(),
		serviceCreate(),
		serviceBind(),
	}
	installerConfig = ""
)

func platformsToInstall() ExecFlow {
	flow := ExecFlow{
		provides: []string{"platformimages"},
	}
	flow.forward = func(c *check.C, env *Environment) {
		for _, platImg := range platforms {
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
		certsDir := fmt.Sprintf("%s/.tsuru/installs/%s/certs", os.Getenv("HOME"), installerName(env))
		for i, parts := range allParts {
			if i == 0 && len(provisioners) == 0 {
				// Keep the first node when there's no provisioner
				continue
			}
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
		for _, prov := range provisioners {
			poolName := "ipool-" + prov
			res := T("pool-add", "--provisioner", prov, poolName).Run(env)
			c.Assert(res, ResultOk)
			env.Add("poolnames", poolName)
			res = T("pool-constraint-set", poolName, "team", "{{.team}}").Run(env)
			c.Assert(res, ResultOk)
			res = T("node-add", "{{.nodeopts}}", "pool="+poolName).Run(env)
			c.Assert(res, ResultOk)
			nodeopts := env.All("nodeopts")
			env.Set("nodeopts", append(nodeopts[1:], nodeopts[0])...)
			regex := regexp.MustCompile(`node.create.*?node:\s+(.*?)\s+`)
			res = T("event-list").Run(env)
			c.Assert(res, ResultOk)
			parts := regex.FindStringSubmatch(res.Stdout.String())
			c.Assert(parts, check.HasLen, 2)
			env.Add("nodeaddrs", parts[1])
			regex = regexp.MustCompile("(?i)" + parts[1] + `.*?ready`)
			ok := retry(5*time.Minute, func() bool {
				res = T("node-list").Run(env)
				return regex.MatchString(res.Stdout.String())
			})
			c.Assert(ok, check.Equals, true, check.Commentf("node not ready after 5 minutes: %v", res))
		}
		for _, cluster := range clusterManagers {
			poolName := "ipool-" + cluster.Name()
			res := T("pool-add", "--provisioner", cluster.Provisioner(), poolName).Run(env)
			c.Assert(res, ResultOk)
			env.Add("poolnames", poolName)
			res = T("pool-constraint-set", poolName, "team", "{{.team}}").Run(env)
			c.Assert(res, ResultOk)
			res = cluster.Start()
			c.Assert(res, ResultOk)
			clusterName := "icluster-" + cluster.Name()
			params := []string{"cluster-add", clusterName, cluster.Provisioner(), "--pool", poolName}
			params = append(params, cluster.UpdateParams()...)
			res = T(params...).Run(env)
			c.Assert(res, ResultOk)
			T("cluster-list").Run(env)
			regex := regexp.MustCompile("(?i)ready")
			addressRegex := regexp.MustCompile(`(?m)^ *\| *((?:https?:\/\/)?\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(?::\d+)?) *\|`)
			nodeIPs := make([]string, 0)
			ok := retry(time.Minute, func() bool {
				res = T("node-list", "-f", "tsuru.io/cluster="+clusterName).Run(env)
				if regex.MatchString(res.Stdout.String()) {
					parts := addressRegex.FindAllStringSubmatch(res.Stdout.String(), -1)
					for _, part := range parts {
						if len(part) == 2 && len(part[1]) > 0 {
							nodeIPs = append(nodeIPs, part[1])
						}
					}
					return true
				}
				return false
			})
			c.Assert(ok, check.Equals, true, check.Commentf("nodes not ready after 1 minute: %v", res))
			for _, ip := range nodeIPs {
				res = T("node-update", ip, "pool="+poolName).Run(env)
				c.Assert(res, ResultOk)
			}
			res = T("event-list").Run(env)
			c.Assert(res, ResultOk)
			for _, ip := range nodeIPs {
				regex = regexp.MustCompile(`node.update.*?node:\s+` + ip)
				c.Assert(regex.MatchString(res.Stdout.String()), check.Equals, true)
			}
			ok = retry(time.Minute, func() bool {
				res = T("node-list").Run(env)
				for _, ip := range nodeIPs {
					regex = regexp.MustCompile("(?i)" + ip + `.*?ready`)
					if !regex.MatchString(res.Stdout.String()) {
						return false
					}
				}
				return true
			})
			c.Assert(ok, check.Equals, true, check.Commentf("nodes not ready after 1 minute: %v", res))
		}
	}
	flow.backward = func(c *check.C, env *Environment) {
		for _, cluster := range clusterManagers {
			res := T("cluster-remove", "-y", "icluster-"+cluster.Name()).Run(env)
			c.Check(res, ResultOk)
			res = cluster.Delete()
			c.Check(res, ResultOk)
			poolName := "ipool-" + cluster.Name()
			res = T("pool-remove", "-y", poolName).Run(env)
			c.Check(res, ResultOk)
		}
		for _, node := range env.All("nodeaddrs") {
			res := T("node-remove", "-y", "--no-rebalance", node).Run(env)
			c.Check(res, ResultOk)
		}
		for _, prov := range provisioners {
			poolName := "ipool-" + prov
			res := T("pool-remove", "-y", poolName).Run(env)
			c.Check(res, ResultOk)
		}
	}
	return flow
}

func platformAdd() ExecFlow {
	flow := ExecFlow{
		provides: []string{"installedplatforms"},
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
		env.Add("installedplatforms", platName)
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
			"plat": "installedplatforms",
		},
		parallel: true,
		provides: []string{"appnames"},
	}
	flow.forward = func(c *check.C, env *Environment) {
		appName := fmt.Sprintf("iapp-%s-%s", env.Get("plat"), env.Get("pool"))
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
		ok := retry(5*time.Minute, func() bool {
			res = T("app-info", "-a", appName).Run(env)
			c.Assert(res, ResultOk)
			return regex.MatchString(res.Stdout.String())
		})
		c.Assert(ok, check.Equals, true, check.Commentf("app not ready after 5 minutes: %v", res))
		addrRE := regexp.MustCompile(`(?s)Address: (.*?)\n`)
		parts = addrRE.FindStringSubmatch(res.Stdout.String())
		c.Assert(parts, check.HasLen, 2)
		cmd := NewCommand("curl", "-sSf", "http://"+parts[1])
		ok = retry(15*time.Minute, func() bool {
			res = cmd.Run(env)
			return res.ExitCode == 0
		})
		c.Assert(ok, check.Equals, true, check.Commentf("invalid result: %v", res))
		env.Add("appnames", appName)
	}
	flow.backward = func(c *check.C, env *Environment) {
		appName := "iapp-{{.plat}}-{{.pool}}"
		res := T("app-remove", "-y", "-a", appName).Run(env)
		c.Check(res, ResultOk)
	}
	return flow
}

func serviceImageSetup() ExecFlow {
	return ExecFlow{
		provides: []string{"serviceimage"},
		forward: func(c *check.C, env *Environment) {
			env.Add("serviceimage", "tsuru/eviaas")
		},
	}
}

func serviceCreate() ExecFlow {
	flow := ExecFlow{
		provides: []string{"servicename"},
		requires: []string{"poolnames", "installedplatforms", "serviceimage"},
	}
	appName := "integration-service-app"
	flow.forward = func(c *check.C, env *Environment) {
		res := T("app-create", appName, env.Get("installedplatforms"), "-t", "{{.team}}", "-o", env.Get("poolnames")).Run(env)
		c.Assert(res, ResultOk)
		res = T("app-info", "-a", appName).Run(env)
		c.Assert(res, ResultOk)
		res = T("env-set", "-a", appName, "EVI_ENVIRONS='{\"INTEGRATION_ENV\":\"TRUE\"}'").Run(env)
		c.Assert(res, ResultOk)
		res = T("app-deploy", "-a", appName, "-i", "{{.serviceimage}}").Run(env)
		c.Assert(res, ResultOk)
		regex := regexp.MustCompile("started")
		ok := retry(5*time.Minute, func() bool {
			res = T("app-info", "-a", appName).Run(env)
			c.Assert(res, ResultOk)
			return regex.MatchString(res.Stdout.String())
		})
		c.Assert(ok, check.Equals, true, check.Commentf("app not ready after 5 minutes: %v", res))
		addrRE := regexp.MustCompile(`(?s)Address: (.*?)\n`)
		parts := addrRE.FindStringSubmatch(res.Stdout.String())
		c.Assert(parts, check.HasLen, 2)
		dir, err := ioutil.TempDir("", "service")
		c.Assert(err, check.IsNil)
		currDir, err := os.Getwd()
		c.Assert(err, check.IsNil)
		err = os.Chdir(dir)
		c.Assert(err, check.IsNil)
		defer os.Chdir(currDir)
		res = T("service-template").Run(env)
		c.Assert(res, ResultOk)
		replaces := map[string]string{
			"team_responsible_to_provide_service": "integration-team",
			"production-endpoint.com":             "http://" + parts[1],
			"servicename":                         "integration-service",
		}
		for k, v := range replaces {
			res = NewCommand("sed", "-i", "-e", "'s~"+k+"~"+v+"~'", "manifest.yaml").Run(env)
			c.Assert(res, ResultOk)
		}
		res = T("service-create", "manifest.yaml").Run(env)
		c.Assert(res, ResultOk)
		res = T("service-info", "integration-service").Run(env)
		c.Assert(res, ResultOk)
		env.Set("servicename", "integration-service")
	}
	flow.backward = func(c *check.C, env *Environment) {
		res := T("app-remove", "-y", "-a", appName).Run(env)
		c.Check(res, ResultOk)
		res = T("service-destroy", "integration-service", "-y").Run(env)
		c.Check(res, ResultOk)
	}
	return flow
}

func serviceBind() ExecFlow {
	flow := ExecFlow{
		matrix: map[string]string{
			"app": "appnames",
		},
		parallel: true,
		requires: []string{"appnames", "servicename"},
		provides: []string{"bindnames"},
	}
	bindName := "{{.servicename}}-{{.app}}"
	flow.forward = func(c *check.C, env *Environment) {
		res := T("service-instance-add", "{{.servicename}}", bindName, "-t", "integration-team").Run(env)
		c.Assert(res, ResultOk)
		res = T("service-instance-bind", "{{.servicename}}", bindName, "-a", "{{.app}}").Run(env)
		c.Assert(res, ResultOk)
		ok := retry(15*time.Minute, func() bool {
			res = T("event-list", "-k", "app.update.bind", "-v", "{{.app}}", "-r").Run(env)
			c.Assert(res, ResultOk)
			return res.Stdout.String() == ""
		})
		c.Assert(ok, check.Equals, true, check.Commentf("bind did not complete after 15 minutes: %v", res))
		res = T("event-list", "-k", "app.update.bind", "-v", "{{.app}}").Run(env)
		c.Assert(res, ResultOk)
		c.Assert(res, ResultMatches, Expected{Stdout: `.*true.*`}, check.Commentf("event did not succeed"))
		ok = retry(time.Minute, func() bool {
			res = T("env-get", "-a", "{{.app}}").Run(env)
			c.Check(res, ResultOk)
			return strings.Contains(res.Stdout.String(), "INTEGRATION_ENV=")
		})
		c.Assert(ok, check.Equals, true, check.Commentf("env not gettable after 1 minute: %v", res))
		cmd := T("app-run", "-a", "{{.app}}", "env")
		ok = retry(time.Minute, func() bool {
			res = cmd.Run(env)
			return strings.Contains(res.Stdout.String(), "INTEGRATION_ENV=TRUE")
		})
		c.Assert(ok, check.Equals, true, check.Commentf("env not injected after 1 minute: %v", res))
		env.Add("bindnames", bindName)
	}
	flow.backward = func(c *check.C, env *Environment) {
		res := T("service-instance-remove", "{{.servicename}}", bindName, "-f", "-y").Run(env)
		c.Check(res, ResultOk)
	}
	return flow
}

func (s *S) TestBase(c *check.C) {
	s.config()
	if s.env == nil {
		return
	}
	var executedFlows []*ExecFlow
	defer func() {
		for i := len(executedFlows) - 1; i >= 0; i-- {
			executedFlows[i].Rollback(c, s.env)
		}
	}()
	for i := range flows {
		f := &flows[i]
		if len(f.provides) > 0 {
			providesAll := true
			for _, envVar := range f.provides {
				if s.env.Get(envVar) == "" {
					providesAll = false
					break
				}
			}
			if providesAll {
				continue
			}
		}
		executedFlows = append(executedFlows, f)
		f.Run(c, s.env)
	}
}
