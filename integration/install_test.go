// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/tsuru/tsuru/net"
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
		nodeHealer(),
		platformAdd(),
		exampleApps(),
		testCases(),
		testApps(),
		updateAppPools(),
		serviceImageSetup(),
		serviceCreate(),
		serviceBind(),
		appRouters(),
		appSwap(),
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
		provides: []string{"targetaddr", "installerhostname"},
		requires: []string{"installerconfig", "installercompose"},
	}
	flow.forward = func(c *check.C, env *Environment) {
		res := T("install-create", "--config", "{{.installerconfig}}", "--compose", "{{.installercompose}}").WithTimeout(60 * time.Minute).Run(env)
		c.Assert(res, ResultOk)
		regex := regexp.MustCompile(`(?si).*New target (.\S+)`)
		parts := regex.FindStringSubmatch(res.Stdout.String())
		c.Assert(parts, check.HasLen, 2)
		env.Set("installerhostname", parts[1]+"-1")
		regex = regexp.MustCompile(`(?si).*Core Hosts:.*?([\d.]+)\s.*`)
		parts = regex.FindStringSubmatch(res.Stdout.String())
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
			env.Add("noderegisteropts", fmt.Sprintf("--register address=%s --cacert %s/ca.pem --clientcert %s/cert.pem --clientkey %s/key.pem", parts[1], certsDir, certsDir, certsDir))
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

func nodeHealer() ExecFlow {
	flow := ExecFlow{
		requires: []string{"nodeopts", "installerhostname"},
		matrix: map[string]string{
			"pool": "multinodepools",
		},
	}
	flow.forward = func(c *check.C, env *Environment) {
		poolName := env.Get("pool")
		nodeOpts := strings.Join(env.All("nodeopts_"+strings.Replace(poolName, "-", "_", -1)), ",")
		if nodeOpts == "" {
			nodeOpts = strings.Join(env.All("nodeopts"), ",")
		}
		res := T("node-add", nodeOpts, "pool="+poolName).Run(env)
		c.Assert(res, ResultOk)
		nodeAddr := waitNewNode(c, env)
		env.Set("newnode-"+poolName, nodeAddr)
		res = T("node-healing-update", "--enable", "--max-unresponsive", "130").Run(env)
		c.Assert(res, ResultOk)
		res = T("node-container-upgrade", "big-sibling", "-y").Run(env)
		c.Assert(res, ResultOk)
		// Wait BS node status upgrade
		time.Sleep(time.Minute * 1)
		res = T("machine-list").Run(env)
		c.Assert(res, ResultOk)
		table := resultTable{raw: res.Stdout.String()}
		table.parse()
		var machineID string
		for _, row := range table.rows {
			c.Assert(row, check.HasLen, 4)
			if net.URLToHost(nodeAddr) == row[2] {
				machineID = row[0]
				break
			}
		}
		c.Assert(machineID, check.Not(check.Equals), "")
		nodeIP := net.URLToHost(nodeAddr)
		res = T("node-container-add", "big-sibling", "-o", poolName, "--raw", "Config.Entrypoint.0=\"/bin/sh\"",
			"--raw", "Config.Entrypoint.1=\"-c\"", "--raw", fmt.Sprintf("Config.Cmd.0=\"ifconfig | grep %s && sleep 3600 || /bin/bs\"", nodeIP),
		).Run(env)
		c.Assert(res, ResultOk)
		res = T("node-container-upgrade", "big-sibling", "-y").Run(env)
		c.Assert(res, ResultOk)
		ok := retry(15*time.Minute, func() bool {
			res = T("event-list", "-k", "healer", "-t", "node", "-v", nodeAddr, "-r").Run(env)
			c.Assert(res, ResultOk)
			return res.Stdout.String() != ""
		})
		c.Assert(ok, check.Equals, true, check.Commentf("node healing did not start after 15 minutes: %v", res))
		res = T("node-container-delete", "big-sibling", "-p", poolName, "-y").Run(env)
		c.Assert(res, ResultOk)
		res = T("node-container-upgrade", "big-sibling", "-y").Run(env)
		c.Assert(res, ResultOk)
		ok = retry(30*time.Minute, func() bool {
			res = T("event-list", "-k", "healer", "-t", "node", "-v", nodeAddr, "-r").Run(env)
			c.Assert(res, ResultOk)
			return res.Stdout.String() == ""
		})
		c.Assert(ok, check.Equals, true, check.Commentf("node healing did not finish after 30 minutes: %v", res))
		res = T("node-healing-update", "--disable").Run(env)
		c.Assert(res, ResultOk)
		res = T("event-list", "-k", "healer", "-t", "node", "-v", nodeAddr).Run(env)
		c.Assert(res, ResultOk)
		table = resultTable{raw: res.Stdout.String()}
		table.parse()
		c.Assert(len(table.rows) > 0, check.Equals, true)
		c.Assert(table.rows[0][2], check.Equals, "true", check.Commentf("expected success, got: %v - event info: %v", res, T("event-info", table.rows[0][0]).Run(env)))
		eventId := table.rows[0][0]
		res = T("event-info", eventId).Run(env)
		c.Assert(res, ResultOk)
		newAddrRegexp := regexp.MustCompile(`(?s)End Custom Data:.*?_id: (.*?)\s`)
		newAddrParts := newAddrRegexp.FindStringSubmatch(res.Stdout.String())
		newAddr := newAddrParts[1]
		env.Set("newnode-"+poolName, newAddr)
	}
	flow.backward = func(c *check.C, env *Environment) {
		nodeAddr := env.Get("newnode-" + env.Get("pool"))
		if nodeAddr == "" {
			return
		}
		res := T("node-remove", "-y", "--destroy", "--no-rebalance", nodeAddr).Run(env)
		c.Check(res, ResultOk)
	}
	return flow
}

func waitNewNode(c *check.C, env *Environment) string {
	regex := regexp.MustCompile(`node.create.*?node:\s+(.*?)\s+`)
	res := T("event-list").Run(env)
	c.Assert(res, ResultOk)
	parts := regex.FindStringSubmatch(res.Stdout.String())
	c.Assert(parts, check.HasLen, 2)
	nodeAddr := parts[1]
	regex = regexp.MustCompile("(?i)" + nodeAddr + `.*?\|\s+ready`)
	ok := retry(5*time.Minute, func() bool {
		res = T("node-list").Run(env)
		return regex.MatchString(res.Stdout.String())
	})
	c.Assert(ok, check.Equals, true, check.Commentf("node not ready after 5 minutes: %v", res))
	return nodeAddr
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
			env.Add("multinodepools", poolName)
			res = T("pool-constraint-set", poolName, "team", "{{.team}}").Run(env)
			c.Assert(res, ResultOk)
			opts := nodeOrRegisterOpts(c, env)
			res = T("node-add", opts, "pool="+poolName).Run(env)
			c.Assert(res, ResultOk)
			nodeAddr := waitNewNode(c, env)
			env.Add("nodeaddrs", nodeAddr)
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
			clusterParams, nodeCreate := cluster.UpdateParams()
			if nodeCreate || env.Get("nodeopts_"+strings.Replace(poolName, "-", "_", -1)) != "" {
				env.Add("multinodepools", poolName)
			}
			res = T(append(params, clusterParams...)...).Run(env)
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
		prefix := img[strings.LastIndex(img, "/")+1:]
		platName := prefix + "-iplat"
		res := T("platform-add", platName, "-i", img).WithTimeout(15 * time.Minute).Run(env)
		c.Assert(res, ResultOk)
		env.Add("installedplatforms", platName)
		res = T("platform-list").Run(env)
		c.Assert(res, ResultOk)
		c.Assert(res, ResultMatches, Expected{Stdout: "(?s).*- " + platName + ".*"})
	}
	flow.backward = func(c *check.C, env *Environment) {
		img := env.Get("platimg")
		prefix := img[strings.LastIndex(img, "/")+1:]
		platName := prefix + "-iplat"
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
		appName := fmt.Sprintf("%s-%s-iapp", env.Get("plat"), env.Get("pool"))
		res := T("app-create", appName, "{{.plat}}", "-t", "{{.team}}", "-o", "{{.pool}}").Run(env)
		c.Assert(res, ResultOk)
		res = T("app-info", "-a", appName).Run(env)
		c.Assert(res, ResultOk)
		platRE := regexp.MustCompile(`(?s)Platform: (.*?)\n`)
		parts := platRE.FindStringSubmatch(res.Stdout.String())
		c.Assert(parts, check.HasLen, 2)
		lang := strings.Replace(parts[1], "-iplat", "", -1)
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
		appName := "{{.plat}}-{{.pool}}-iapp"
		res := T("app-remove", "-y", "-a", appName).Run(env)
		c.Check(res, ResultOk)
	}
	return flow
}

func testCases() ExecFlow {
	return ExecFlow{
		provides: []string{"testcases", "testcasesdir"},
		forward: func(c *check.C, env *Environment) {
			gopath := os.Getenv("GOPATH")
			if gopath == "" {
				gopath = build.Default.GOPATH
			}
			casesDir := path.Join(gopath, "src", "github.com", "tsuru", "tsuru", "integration", "testapps")
			files, err := ioutil.ReadDir(casesDir)
			c.Assert(err, check.IsNil)
			env.Add("testcasesdir", casesDir)
			for _, f := range files {
				if !f.IsDir() {
					continue
				}
				env.Add("testcases", f.Name())
			}
		},
	}
}

func appRouters() ExecFlow {
	return ExecFlow{
		provides: []string{"routers"},
		forward: func(c *check.C, env *Environment) {
			res := T("router-list").Run(env)
			c.Assert(res, ResultOk)
			table := resultTable{raw: res.Stdout.String()}
			table.parse()
			for _, row := range table.rows {
				env.Add("routers", row[0])
			}
		},
	}
}

func appSwap() ExecFlow {
	flow := ExecFlow{
		matrix: map[string]string{
			"pool":   "poolnames",
			"router": "routers",
		},
		parallel: true,
	}
	flow.forward = func(c *check.C, env *Environment) {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			gopath = build.Default.GOPATH
		}
		swapDir := path.Join(gopath, "src", "github.com", "tsuru", "tsuru", "integration", "fixtures", "swap-app")
		appNames := []string{
			slugifyName(fmt.Sprintf("swap-app1-%s-%s-iapp", env.Get("pool"), env.Get("router"))),
			slugifyName(fmt.Sprintf("swap-app2-%s-%s-iapp", env.Get("pool"), env.Get("router"))),
		}
		appCname := func(appName string) string {
			return fmt.Sprintf("%s.integration.test", appName)
		}
		var res *Result
		var addrs []string
		for _, appName := range appNames {
			res = T("app-create", appName, "python-iplat", "-t", "{{.team}}", "-o", "{{.pool}}", "-r", "{{.router}}").Run(env)
			c.Assert(res, ResultOk)
			res = T("cname-add", "-a", appName, appCname(appName)).Run(env)
			c.Assert(res, ResultOk)
			env.Add(fmt.Sprintf("swap-apps-%s-%s", env.Get("pool"), env.Get("router")), appName)
			res = T("app-deploy", "-a", appName, swapDir).Run(env)
			c.Assert(res, ResultOk)
			regex := regexp.MustCompile("started")
			ok := retry(5*time.Minute, func() bool {
				res = T("app-info", "-a", appName).Run(env)
				c.Assert(res, ResultOk)
				return regex.MatchString(res.Stdout.String())
			})
			c.Assert(ok, check.Equals, true, check.Commentf("app not ready after 5 minutes: %v", res))
			addrRE := regexp.MustCompile(fmt.Sprintf(`\| %s\s+(\|[^|]+){2}\| ([^| ]+)`, env.Get("router")))
			parts := addrRE.FindStringSubmatch(res.Stdout.String())
			c.Assert(parts, check.HasLen, 3)
			addrs = append(addrs, parts[2])
		}
		runTest := func(idx int, expected, cnameExpected string) {
			cmd := NewCommand("curl", "-sSf", "http://"+addrs[idx])
			ok := retry(1*time.Minute, func() bool {
				res = cmd.Run(env)
				return res.ExitCode == 0
			})
			c.Assert(ok, check.Equals, true, check.Commentf("app did not respond after 1 minute: %v", res))
			c.Assert(res.Stdout.String(), check.Matches, `app: `+expected)
			cmd = NewCommand("curl", "-sSf", "-HHost:"+appCname(appNames[idx]), "http://"+addrs[idx])
			ok = retry(1*time.Minute, func() bool {
				res = cmd.Run(env)
				return res.ExitCode == 0
			})
			c.Assert(ok, check.Equals, true, check.Commentf("app did not respond after 1 minute: %v", res))
			c.Assert(res.Stdout.String(), check.Matches, `app: `+cnameExpected)
		}
		runTest(0, appNames[0], appNames[0])
		runTest(1, appNames[1], appNames[1])
		res = T("app-swap", appNames[0], appNames[1]).Run(env)
		c.Assert(res, ResultOk)
		runTest(0, appNames[1], appNames[1])
		runTest(1, appNames[0], appNames[0])
		res = T("app-swap", appNames[0], appNames[1]).Run(env)
		c.Assert(res, ResultOk)
		runTest(0, appNames[0], appNames[0])
		runTest(1, appNames[1], appNames[1])
		res = T("app-swap", "--cname-only", appNames[0], appNames[1]).Run(env)
		c.Assert(res, ResultOk)
		runTest(0, appNames[0], appNames[1])
		runTest(1, appNames[1], appNames[0])
		res = T("app-swap", "--cname-only", appNames[0], appNames[1]).Run(env)
		c.Assert(res, ResultOk)
		runTest(0, appNames[0], appNames[0])
		runTest(1, appNames[1], appNames[1])
	}
	flow.backward = func(c *check.C, env *Environment) {
		appNames := env.All(fmt.Sprintf("swap-apps-%s-%s", env.Get("pool"), env.Get("router")))
		for _, appName := range appNames {
			res := T("app-remove", "-y", "-a", appName).Run(env)
			c.Check(res, ResultOk)
		}
	}
	return flow
}

func testApps() ExecFlow {
	flow := ExecFlow{
		requires: []string{"testcases", "testcasesdir"},
		matrix: map[string]string{
			"pool": "poolnames",
			"case": "testcases",
		},
		parallel: true,
	}
	flow.forward = func(c *check.C, env *Environment) {
		path := path.Join(env.Get("testcasesdir"), env.Get("case"), "platform")
		plat, err := ioutil.ReadFile(path)
		c.Assert(err, check.IsNil)
		appName := fmt.Sprintf("%s-%s-iapp", env.Get("case"), env.Get("pool"))
		res := T("app-create", appName, string(plat)+"-iplat", "-t", "{{.team}}", "-o", "{{.pool}}").Run(env)
		c.Assert(res, ResultOk)
		res = T("app-info", "-a", appName).Run(env)
		c.Assert(res, ResultOk)
		res = T("app-deploy", "-a", appName, "{{.testcasesdir}}/{{.case}}/").Run(env)
		c.Assert(res, ResultOk)
		regex := regexp.MustCompile("started")
		ok := retry(5*time.Minute, func() bool {
			res = T("app-info", "-a", appName).Run(env)
			c.Assert(res, ResultOk)
			return regex.MatchString(res.Stdout.String())
		})
		c.Assert(ok, check.Equals, true, check.Commentf("app not ready after 5 minutes: %v", res))
	}
	flow.backward = func(c *check.C, env *Environment) {
		appName := "{{.case}}-{{.pool}}-iapp"
		res := T("app-remove", "-y", "-a", appName).Run(env)
		c.Check(res, ResultOk)
	}
	return flow
}

func updateAppPools() ExecFlow {
	flow := ExecFlow{
		requires: []string{"poolnames", "installedplatforms"},
	}
	flow.forward = func(c *check.C, env *Environment) {
		poolNames := env.All("poolnames")
		installedPlatforms := env.All("installedplatforms")
		chosenPlatform := installedPlatforms[0]
		var combinations [][]string
		for i := range poolNames {
			for j := range poolNames {
				if i == j {
					continue
				}
				combinations = append(combinations, []string{
					fmt.Sprintf("%s-%s-iapp", chosenPlatform, poolNames[i]),
					poolNames[j],
				})
			}
		}
		for _, combination := range combinations {
			appName := combination[0]
			destPool := combination[1]
			res := T("app-update", "-a", appName, "-o", destPool).Run(env)
			c.Assert(res, ResultOk)
			res = T("app-info", "-a", appName).Run(env)
			c.Assert(res, ResultOk)
			addrRE := regexp.MustCompile(`(?s)Address: (.*?)\n`)
			parts := addrRE.FindStringSubmatch(res.Stdout.String())
			c.Assert(parts, check.HasLen, 2)
			cmd := NewCommand("curl", "-sSf", "http://"+parts[1])
			ok := retry(15*time.Minute, func() bool {
				res = cmd.Run(env)
				return res.ExitCode == 0
			})
			c.Assert(ok, check.Equals, true, check.Commentf("invalid result: %v", res))
		}
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
		matrix:   map[string]string{"pool": "poolnames"},
	}
	appName := "{{.pool}}-integration-service-app"
	flow.forward = func(c *check.C, env *Environment) {
		res := T("app-create", appName, env.Get("installedplatforms"), "-t", "{{.team}}", "-o", "{{.pool}}").Run(env)
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
			"servicename":                         "integration-service-" + env.Get("pool"),
		}
		for k, v := range replaces {
			res = NewCommand("sed", "-i", "-e", "'s~"+k+"~"+v+"~'", "manifest.yaml").Run(env)
			c.Assert(res, ResultOk)
		}
		res = T("service-create", "manifest.yaml").Run(env)
		c.Assert(res, ResultOk)
		res = T("service-info", "integration-service-{{.pool}}").Run(env)
		c.Assert(res, ResultOk)
		env.Add("servicename", "integration-service-"+env.Get("pool"))
	}
	flow.backward = func(c *check.C, env *Environment) {
		res := T("app-remove", "-y", "-a", appName).Run(env)
		c.Check(res, ResultOk)
		res = T("service-destroy", "integration-service-{{.pool}}", "-y").Run(env)
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
	s.config(c)
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
