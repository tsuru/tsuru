// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"fmt"
	"go/build"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tsuru/tsuru/net"
	check "gopkg.in/check.v1"
)

var (
	T               = NewCommand("tsuru").WithArgs
	platforms       = []string{}
	provisioners    = []string{}
	clusterManagers = []ClusterManager{}
	flows           = []ExecFlow{
		platformsToInstall(),
		targetTest(),
		loginTest(),
		quotaTest(),
		teamTest(),
		poolAdd(),
		platformAdd(),
		exampleApps(),
		unitAddRemove(),
		testCases(),
		testApps(),
		updateAppPools(),
		serviceImageSetup(),
		serviceCreate(),
		serviceBind(),
		appRouters(),
		appSwap(),
		appVersions(),
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

func waitNewNode(c *check.C, env *Environment) string {
	regex := regexp.MustCompile(`node.create.*?node:\s+(.*?)\s+`)
	res := T("event-list").Run(env)
	c.Assert(res, ResultOk)
	parts := regex.FindStringSubmatch(res.Stdout.String())
	c.Assert(parts, check.HasLen, 2)
	return waitNodeAddr(c, env, parts[1])
}

func waitNodeAddr(c *check.C, env *Environment, nodeAddr string) string {
	regex := regexp.MustCompile("(?i)" + net.URLToHost(nodeAddr) + `.*?\|\s+ready`)
	var res *Result
	ok := retry(5*time.Minute, func() bool {
		res = T("node-list").Run(env)
		return regex.MatchString(res.Stdout.String())
	})
	c.Assert(ok, check.Equals, true, check.Commentf("node not ready after 5 minutes: %v", res))
	// Wait for docker daemon restart due to hostcert node container
	time.Sleep(time.Minute * 1)
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
			res = T("node-add", opts, "pool="+poolName).WithTimeout(20 * time.Minute).Run(env)
			c.Assert(res, ResultOk)
			nodeAddr := waitNewNode(c, env)
			env.Add("nodeaddrs", nodeAddr)
		}
		for _, cluster := range clusterManagers {
			poolName := "ipool-" + cluster.Name()
			res := T("pool-add", "--provisioner", cluster.Provisioner(), poolName).Run(env)
			c.Assert(res, ResultOk)
			env.Add("poolnames", poolName)
			env.Add("clusterpools", poolName)
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
			res = T(append(params, clusterParams...)...).WithNoExpand().WithTimeout(120 * time.Minute).Run(env)
			c.Assert(res, ResultOk)
			T("cluster-list").Run(env)
			readyRegex := regexp.MustCompile("(?i)^ready")
			var nodeIPs []string
			ok := retry(2*time.Minute, func() bool {
				res = T("node-list", "-f", "tsuru.io/cluster="+clusterName).Run(env)
				table := resultTable{raw: res.Stdout.String()}
				table.parse()
				if len(table.rows) == 0 {
					return false
				}
				for _, row := range table.rows {
					c.Assert(len(row) > 2, check.Equals, true)
					if !readyRegex.MatchString(row[2]) {
						nodeIPs = nil
						return false
					}
					nodeIPs = append(nodeIPs, row[0])
				}
				return true
			})
			if noPoolOnKubeMasters, _ := strconv.ParseBool(env.Get("no_pool_on_kube_masters")); noPoolOnKubeMasters {
				nodeIPs = nil
			}
			c.Assert(ok, check.Equals, true, check.Commentf("nodes not ready after 2 minutes: %v - all nodes: %v", res, T("node-list").Run(env)))
			for _, ip := range nodeIPs {
				res = T("node-update", ip, "pool="+poolName).Run(env)
				c.Assert(res, ResultOk)
			}
			res = T("event-list").Run(env)
			c.Assert(res, ResultOk)
			for _, ip := range nodeIPs {
				evtRegex := regexp.MustCompile(`node.update.*?node:\s+` + ip)
				c.Assert(evtRegex.MatchString(res.Stdout.String()), check.Equals, true)
			}
			ok = retry(time.Minute, func() bool {
				res = T("node-list").Run(env)
				table := resultTable{raw: res.Stdout.String()}
				table.parse()
				for _, ip := range nodeIPs {
					for _, row := range table.rows {
						c.Assert(len(row) > 2, check.Equals, true)
						if row[0] == ip && !readyRegex.MatchString(row[2]) {
							return false
						}
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
		c.Assert(res, ResultMatches, Expected{Stdout: "(?s).*" + platName + ".*enabled.*"})
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
		regex := regexp.MustCompile("started|ready")
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
			pwd, err := os.Getwd()
			c.Assert(err, check.IsNil)

			casesDir := path.Join(pwd, "integration", "testapps")
			files, err := readDir(casesDir)
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

func readDir(dirname string) ([]fs.FileInfo, error) {
	f, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}
	list, err := f.Readdir(-1)
	f.Close()
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name() < list[j].Name() })
	return list, nil
}

func unitAddRemove() ExecFlow {
	flow := ExecFlow{
		requires: []string{"appnames"},
	}
	flow.forward = func(c *check.C, env *Environment) {
		appName := env.Get("appnames")
		res := T("unit-add", "-a", appName, "2").Run(env)
		c.Assert(res, ResultOk)
		regex := regexp.MustCompile(" (started|ready) ")
		ok := retry(5*time.Minute, func() bool {
			res = T("app-info", "-a", appName).Run(env)
			c.Assert(res, ResultOk)
			unitsReady := len(regex.FindAllString(res.Stdout.String(), -1))
			return unitsReady == 3
		})
		c.Assert(ok, check.Equals, true, check.Commentf("new units not ready after 5 minutes: %v", res))
		res = T("unit-remove", "-a", appName, "2").Run(env)
		c.Assert(res, ResultOk)
		ok = retry(5*time.Minute, func() bool {
			res = T("app-info", "-a", appName).Run(env)
			c.Assert(res, ResultOk)
			unitsReady := len(regex.FindAllString(res.Stdout.String(), -1))
			return unitsReady == 1
		})
		c.Assert(ok, check.Equals, true, check.Commentf("new units not removed after 5 minutes: %v", res))
	}
	return flow
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
			regex := regexp.MustCompile("started|ready")
			ok := retry(5*time.Minute, func() bool {
				res = T("app-info", "-a", appName).Run(env)
				c.Assert(res, ResultOk)
				return regex.MatchString(res.Stdout.String())
			})
			c.Assert(ok, check.Equals, true, check.Commentf("app not ready after 5 minutes: %v", res))
			addrRE := regexp.MustCompile(fmt.Sprintf(`\| %s\s+(\|[^|]+){1}\| ([^| ]+)`, env.Get("router")))
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

func appVersions() ExecFlow {
	flow := ExecFlow{
		matrix: map[string]string{
			"pool": "clusterpools",
		},
		parallel: true,
	}
	flow.forward = func(c *check.C, env *Environment) {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			gopath = build.Default.GOPATH
		}
		appDir := path.Join(gopath, "src", "github.com", "tsuru", "tsuru", "integration", "fixtures", "versions-app")
		appName := slugifyName(fmt.Sprintf("versions-%s-iapp", env.Get("pool")))
		res := T("app-create", appName, "python-iplat", "-t", "{{.team}}", "-o", "{{.pool}}").Run(env)
		c.Assert(res, ResultOk)

		checkVersion := func(expectedVersions ...string) {
			regex := regexp.MustCompile("started|ready")
			ok := retry(5*time.Minute, func() bool {
				res = T("app-info", "-a", appName).Run(env)
				c.Assert(res, ResultOk)
				return regex.MatchString(res.Stdout.String())
			})
			c.Assert(ok, check.Equals, true, check.Commentf("app not ready after 5 minutes: %v", res))
			addrRE := regexp.MustCompile(`(?s)Address: (.*?)\n`)
			parts := addrRE.FindStringSubmatch(res.Stdout.String())
			c.Assert(parts, check.HasLen, 2)
			cmd := NewCommand("curl", "-m5", "-sSf", "http://"+parts[1])
			successCount := 0
			ok = retryWait(15*time.Minute, 2*time.Second, func() bool {
				res = cmd.Run(env)
				if res.ExitCode == 0 {
					successCount++
				}
				return successCount == 10
			})
			c.Assert(ok, check.Equals, true, check.Commentf("invalid result: %v", res))
			versionRE := regexp.MustCompile(`.* version: (\d+)$`)
			versionsCounter := map[string]int{}
			for i := 0; i < 15; i++ {
				ok = retryWait(30*time.Second, time.Second, func() bool {
					res = cmd.Run(env)
					return res.ExitCode == 0
				})
				c.Assert(ok, check.Equals, true, check.Commentf("invalid result: %v", res))
				parts = versionRE.FindStringSubmatch(res.Stdout.String())
				c.Assert(parts, check.HasLen, 2)
				versionsCounter[parts[1]]++
			}
			for _, v := range expectedVersions {
				c.Assert(versionsCounter[v] > 0, check.Equals, true)
			}
		}

		res = T("app-deploy", "-a", appName, appDir).Run(env)
		c.Assert(res, ResultOk)
		checkVersion("1")
		res = T("app-deploy", "-a", appName, appDir).Run(env)
		c.Assert(res, ResultOk)
		checkVersion("2")
		res = T("app-deploy", "--new-version", "-a", appName, appDir).Run(env)
		c.Assert(res, ResultOk)
		checkVersion("2")

		time.Sleep(1 * time.Second)
		res = T("app-router-version-add", "3", "-a", appName).Run(env)
		c.Assert(res, ResultOk)
		checkVersion("2", "3")
		time.Sleep(1 * time.Second)
		res = T("app-router-version-remove", "2", "-a", appName).Run(env)
		c.Assert(res, ResultOk)
		checkVersion("3")

		res = T("unit-add", "1", "--version", "1", "-a", appName).Run(env)
		c.Assert(res, ResultOk)
		checkVersion("3")

		time.Sleep(1 * time.Second)
		res = T("app-router-version-add", "1", "-a", appName).Run(env)
		c.Assert(res, ResultOk)
		checkVersion("1", "3")
		time.Sleep(1 * time.Second)
		res = T("app-router-version-remove", "3", "-a", appName).Run(env)
		c.Assert(res, ResultOk)
		checkVersion("1")

		res = T("app-deploy", "--override-old-versions", "-a", appName, appDir).Run(env)
		c.Assert(res, ResultOk)
		checkVersion("4")

	}
	flow.backward = func(c *check.C, env *Environment) {
		appName := slugifyName(fmt.Sprintf("versions-%s-iapp", env.Get("pool")))
		res := T("app-remove", "-y", "-a", appName).Run(env)
		c.Check(res, ResultOk)
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
		plat, err := os.ReadFile(path)
		c.Assert(err, check.IsNil)
		appName := fmt.Sprintf("%s-%s-iapp", env.Get("case"), env.Get("pool"))
		res := T("app-create", appName, string(plat)+"-iplat", "-t", "{{.team}}", "-o", "{{.pool}}").Run(env)
		c.Assert(res, ResultOk)
		res = T("app-info", "-a", appName).Run(env)
		c.Assert(res, ResultOk)
		res = T("app-deploy", "-a", appName, "{{.testcasesdir}}/{{.case}}/").Run(env)
		c.Assert(res, ResultOk)
		regex := regexp.MustCompile("started|ready")
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
		regex := regexp.MustCompile("started|ready")
		ok := retry(5*time.Minute, func() bool {
			res = T("app-info", "-a", appName).Run(env)
			c.Assert(res, ResultOk)
			return regex.MatchString(res.Stdout.String())
		})
		c.Assert(ok, check.Equals, true, check.Commentf("app not ready after 5 minutes: %v", res))
		addrRE := regexp.MustCompile(`(?s)Address: (.*?)\n`)
		parts := addrRE.FindStringSubmatch(res.Stdout.String())
		c.Assert(parts, check.HasLen, 2)
		cmd := NewCommand("curl", "-sS", "-o", "/dev/null", "--write-out", "%{http_code}", "http://"+parts[1])
		ok = retry(15*time.Minute, func() bool {
			res = cmd.Run(env)
			code, _ := strconv.Atoi(res.Stdout.String())
			return code >= 200 && code < 500
		})
		c.Assert(ok, check.Equals, true, check.Commentf("invalid result: %v", res))
		dir, err := os.MkdirTemp("", "service")
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
