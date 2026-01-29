// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	check "gopkg.in/check.v1"

	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
)

var (
	T            = NewCommand("tsuru").WithArgs
	K            = NewCommand("kubectl").WithArgs("--context=minikube").WithArgs
	platforms    = []string{}
	provisioners = []string{"kubernetes"}
	flows        = []ExecFlow{
		platformsToInstall(),
		targetTest(),
		loginTest(),
		quotaTest(),
		teamTest(),
		poolAdd(),
		platformAdd(),
		jobCreate(),
		jobDeploy(),
		jobTrigger(),
		jobLogs(),
		jobUpdate(),
		jobEnvSet(),
		jobList(),
		exampleApps(),
		unitAddRemove(),
		deployWithAutoscale(),
		testCases(),
		testApps(),
		updateAppPools(),
		serviceImageSetup(),
		serviceCreate(),
		serviceBind(),
		appRouters(),
		appVersions(),
		multiversionRollbackTest(),
		multiversionRollbackOverrideTest(),
		multiversionRoutableStopStartTest(),
		swapAutoScaleTest(),
	}
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
		res := T("target", "add", targetName, "{{.targetaddr}}").Run(env)
		c.Assert(res, ResultOk)
		res = T("target", "list").Run(env)
		c.Assert(res, ResultMatches, Expected{Stdout: `\s+` + targetName + ` .*`})
		res = T("target", "set", targetName).Run(env)
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
		res := T("user", "quota", "change", "{{.adminuser}}", "100").Run(env)
		c.Assert(res, ResultOk)
		res = T("user", "quota", "view", "{{.adminuser}}").Run(env)
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
		res := T("team", "create", teamName).Run(env)
		c.Assert(res, ResultOk)
		env.Set("team", teamName)
	}
	flow.backward = func(c *check.C, env *Environment) {
		res := T("team", "remove", "-y", teamName).Run(env)
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
			res := T("pool", "add", "--provisioner", prov, poolName).Run(env)
			c.Assert(res, ResultOk)
			env.Add("poolnames", poolName)
			res = T("pool", "constraint", "set", poolName, "team", "{{.team}}").Run(env)
			c.Assert(res, ResultOk)
		}
	}
	flow.backward = func(c *check.C, env *Environment) {
		for _, prov := range provisioners {
			poolName := "ipool-" + prov
			res := T("pool", "remove", "-y", poolName).Run(env)
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
		res := T("platform", "add", platName, "-i", img).WithTimeout(15 * time.Minute).Run(env)
		c.Assert(res, ResultOk)
		env.Add("installedplatforms", platName)
		res = T("platform", "list").Run(env)
		c.Assert(res, ResultOk)
		c.Assert(res, ResultMatches, Expected{Stdout: "(?s).*" + platName + ".*enabled.*"})
	}
	flow.backward = func(c *check.C, env *Environment) {
		img := env.Get("platimg")
		prefix := img[strings.LastIndex(img, "/")+1:]
		platName := prefix + "-iplat"
		res := T("platform", "remove", "-y", platName).Run(env)
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
		res := T("app", "create", appName, "{{.plat}}", "-t", "{{.team}}", "-o", "{{.pool}}").Run(env)
		c.Assert(res, ResultOk)
		appInfo := getAppInfo(c, appName, env)
		lang := strings.ReplaceAll(appInfo.Platform, "-iplat", "")
		res = T("app", "deploy", "-a", appName, ".").WithPWD(env.Get("examplesdir") + "/" + lang).Run(env)
		c.Assert(res, ResultOk)
		ok := retry(5*time.Minute, func() (ready bool) {
			appInfo, ready = checkAppExternallyAddressable(c, appName, env)
			return ready
		})
		c.Assert(ok, check.Equals, true, check.Commentf("app not ready after 5 minutes: %v", res))
		externalAddress := appInfo.Routers[0].Addresses[0]
		cmd := NewCommand("curl", "-sSf", "http://"+externalAddress)
		ok = retry(15*time.Minute, func() bool {
			res = cmd.Run(env)
			return res.ExitCode == 0
		})
		c.Assert(ok, check.Equals, true, check.Commentf("invalid result: %v", res))
		env.Add("appnames", appName)
	}
	flow.backward = func(c *check.C, env *Environment) {
		appName := "{{.plat}}-{{.pool}}-iapp"
		res := T("app", "remove", "-y", "-a", appName).Run(env)
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

			casesDir := path.Join(pwd, "testapps")
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
		res := T("unit", "add", "-a", appName, "2").Run(env)
		c.Assert(res, ResultOk)

		ok := retry(5*time.Minute, func() bool {
			_, ok := checkReadyUnits(c, appName, env, 3)
			return ok
		})
		c.Assert(ok, check.Equals, true, check.Commentf("new units not ready after 5 minutes: %v", res))
		res = T("unit", "remove", "-a", appName, "2").Run(env)
		c.Assert(res, ResultOk)
		ok = retry(5*time.Minute, func() bool {
			_, ok := checkReadyUnits(c, appName, env, 1)
			return ok
		})
		c.Assert(ok, check.Equals, true, check.Commentf("new units not removed after 5 minutes: %v", res))
	}
	return flow
}

func deployWithAutoscale() ExecFlow {
	flow := ExecFlow{
		matrix: map[string]string{
			"pool": "poolnames",
		},
		requires: []string{"team", "poolnames"},
	}
	flow.forward = func(c *check.C, env *Environment) {
		cwd, err := os.Getwd()
		c.Assert(err, check.IsNil)

		appDir := path.Join(cwd, "fixtures", "multiversion-python-app")
		appName := slugifyName(fmt.Sprintf("deploy-with-autoscale-%s", env.Get("pool")))

		res := T("app", "create", appName, "python-iplat", "-o", "{{.pool}}", "-t", "{{.team}}").Run(env)
		c.Assert(res, ResultOk)

		imageToHash := make(map[string]string)
		deployAndMapHash(c, appDir, appName, []string{}, imageToHash, env)

		res = T("autoscale", "set", "-a", appName, "--cpu", "30", "--min", "3", "--max", "10").Run(env)
		c.Assert(res, ResultOk)
		ok := retry(5*time.Minute, func() bool {
			_, ok := checkReadyUnits(c, appName, env, 3)
			return ok
		})
		c.Assert(ok, check.Equals, true, check.Commentf("new units not ready after 5 minutes: %v", res))

		deployAndMapHash(c, appDir, appName, []string{}, imageToHash, env)

		appInfo := new(appTypes.AppInfo)
		ok = retry(5*time.Minute, func() (ready bool) {
			appInfo, ready = checkAppExternallyAddressable(c, appName, env)
			return ready
		})
		c.Assert(ok, check.Equals, true, check.Commentf("app not ready after 5 minutes: %v", res))
		externalAddress := appInfo.Routers[0].Addresses[0]
		cmd := NewCommand("curl", "-sSf", "http://"+externalAddress)
		ok = retry(15*time.Minute, func() bool {
			res = cmd.Run(env)
			return res.ExitCode == 0
		})
		c.Assert(ok, check.Equals, true, check.Commentf("invalid result: %v", res))

		res = T("autoscale", "unset", "-a", appName).Run(env)
		c.Assert(res, ResultOk)
	}
	return flow
}

func appRouters() ExecFlow {
	return ExecFlow{
		provides: []string{"routers"},
		forward: func(c *check.C, env *Environment) {
			res := T("router", "list").Run(env)
			c.Assert(res, ResultOk)
			table := resultTable{raw: res.Stdout.String()}
			table.parse()
			for _, row := range table.rows {
				env.Add("routers", row[0])
			}
		},
	}
}

func appVersions() ExecFlow {
	flow := ExecFlow{
		matrix: map[string]string{
			"pool": "poolnames",
		},
		parallel: true,
	}
	flow.forward = func(c *check.C, env *Environment) {
		cwd, err := os.Getwd()
		c.Assert(err, check.IsNil)

		appDir := path.Join(cwd, "fixtures", "versions-app")
		appName := slugifyName(fmt.Sprintf("versions-%s-iapp", env.Get("pool")))
		res := T("app", "create", appName, "python-iplat", "-t", "{{.team}}", "-o", "{{.pool}}").Run(env)
		c.Assert(res, ResultOk)

		appInfo := new(appTypes.AppInfo)
		checkVersion := func(expectedVersions ...string) {
			ok := retry(5*time.Minute, func() (ready bool) {
				appInfo, ready = checkAppExternallyAddressable(c, appName, env)
				if !ready {
					return false
				}
				// Check that all pods are running and not being deleted
				res := K("get", "pods", "-l", fmt.Sprintf("tsuru.io/app-name=%s", appName), "-o", `"jsonpath={range .items[*]}{.status.phase},{.metadata.deletionTimestamp}{'\\n'}{end}"`).Run(env)
				c.Assert(res, ResultOk)
				podStatuses := strings.Split(strings.TrimSpace(res.Stdout.String()), "\n")
				for _, status := range podStatuses {
					if status == "" {
						continue
					}
					parts := strings.Split(status, ",")
					phase := parts[0]
					deletionTimestamp := ""
					if len(parts) > 1 {
						deletionTimestamp = parts[1]
					}
					if phase != "Running" || deletionTimestamp != "" {
						return false
					}
				}
				return true
			})
			c.Assert(ok, check.Equals, true, check.Commentf("app not ready after 5 minutes: %v", res))
			externalAddress := appInfo.Routers[0].Addresses[0]
			cmd := NewCommand("curl", "-m5", "-sSf", "http://"+externalAddress)
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
			for range 15 {
				ok = retryWait(30*time.Second, time.Second, func() bool {
					res = cmd.Run(env)
					return res.ExitCode == 0
				})
				c.Assert(ok, check.Equals, true, check.Commentf("invalid result: %v", res))
				parts := versionRE.FindStringSubmatch(res.Stdout.String())
				c.Assert(parts, check.HasLen, 2)
				versionsCounter[parts[1]]++
			}
			for _, v := range expectedVersions {
				c.Assert(versionsCounter[v] > 0, check.Equals, true)
			}
		}

		res = T("app", "deploy", "-a", appName, appDir).Run(env)
		c.Assert(res, ResultOk)
		checkVersion("1")
		res = T("app", "deploy", "-a", appName, appDir).Run(env)
		c.Assert(res, ResultOk)
		checkVersion("2")
		res = T("app", "deploy", "--new-version", "-a", appName, appDir).Run(env)
		c.Assert(res, ResultOk)
		checkVersion("2")

		time.Sleep(1 * time.Second)
		res, ok := T("app", "router", "version", "add", "3", "-a", appName).Retry(time.Minute, env, RetryOptions{})
		c.Assert(res, ResultOk)
		c.Assert(ok, check.Equals, true)
		checkVersion("2", "3")
		time.Sleep(1 * time.Second)
		res, ok = T("app", "router", "version", "remove", "2", "-a", appName).Retry(time.Minute, env, RetryOptions{})
		c.Assert(res, ResultOk)
		c.Assert(ok, check.Equals, true)
		checkVersion("3")

		res = T("unit", "add", "1", "--version", "1", "-a", appName).Run(env)
		c.Assert(res, ResultOk)
		checkVersion("3")

		time.Sleep(1 * time.Second)
		res, ok = T("app", "router", "version", "add", "1", "-a", appName).Retry(time.Minute, env, RetryOptions{})
		c.Assert(res, ResultOk)
		checkVersion("1", "3")
		c.Assert(ok, check.Equals, true)
		time.Sleep(1 * time.Second)
		res, ok = T("app", "router", "version", "remove", "3", "-a", appName).Retry(time.Minute, env, RetryOptions{})
		c.Assert(res, ResultOk)
		c.Assert(ok, check.Equals, true)
		checkVersion("1")

		res = T("app", "deploy", "--override-old-versions", "-a", appName, appDir).Run(env)
		c.Assert(res, ResultOk)
		checkVersion("4")
	}
	flow.backward = func(c *check.C, env *Environment) {
		appName := slugifyName(fmt.Sprintf("versions-%s-iapp", env.Get("pool")))
		res := T("app", "remove", "-y", "-a", appName).Run(env)
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
		platformPath := path.Join(env.Get("testcasesdir"), env.Get("case"), "platform")
		plat, err := os.ReadFile(platformPath)
		c.Assert(err, check.IsNil)
		appName := fmt.Sprintf("%s-%s-iapp", env.Get("case"), env.Get("pool"))
		res := T("app", "create", appName, string(plat)+"-iplat", "-t", "{{.team}}", "-o", "{{.pool}}").Run(env)
		c.Assert(res, ResultOk)
		res = T("app", "info", "-a", appName).Run(env)
		c.Assert(res, ResultOk)
		res = T("app", "deploy", "-a", appName, "{{.testcasesdir}}/{{.case}}/").Run(env)
		c.Assert(res, ResultOk)
		ok := retry(5*time.Minute, func() bool {
			_, ready := checkAppReady(c, appName, env)
			return ready
		})
		c.Assert(ok, check.Equals, true, check.Commentf("app not ready after 5 minutes: %v", res))

		if env.Get("case") == "multi-health" {
			res := T("app", "info", "-a", appName, "--json").Run(env)
			c.Assert(res, ResultOk)

			appInfo := new(appTypes.AppInfo)
			err := json.NewDecoder(&res.Stdout).Decode(appInfo)
			c.Assert(err, check.IsNil)

			c.Assert(appInfo.InternalAddresses, check.HasLen, 2)
			c.Assert("multi-health-ipool-kubernetes-iapp-web.default.svc.cluster.local", check.Equals, appInfo.InternalAddresses[0].Domain)
			c.Assert("multi-health-ipool-kubernetes-iapp-web-secondary.default.svc.cluster.local", check.Equals, appInfo.InternalAddresses[1].Domain)
			c.Assert("web", check.Equals, appInfo.InternalAddresses[0].Process)
			c.Assert("web-secondary", check.Equals, appInfo.InternalAddresses[1].Process)

			for _, internalAddress := range appInfo.InternalAddresses {
				c.Assert(internalAddress.Protocol, check.Equals, "TCP")
				c.Assert(internalAddress.Port, check.Equals, int32(80))
			}
		}
	}
	flow.backward = func(c *check.C, env *Environment) {
		appName := "{{.case}}-{{.pool}}-iapp"
		res := T("app", "remove", "-y", "-a", appName).Run(env)
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
			res := T("app", "update", "-a", appName, "-o", destPool).Run(env)
			c.Assert(res, ResultOk)
			appInfo := new(appTypes.AppInfo)
			ok := retry(5*time.Minute, func() (ready bool) {
				appInfo, ready = checkAppExternallyAddressable(c, appName, env)
				return ready
			})
			c.Assert(ok, check.Equals, true, check.Commentf("app not ready after 5 minutes: %v", res))
			externalAddress := appInfo.Routers[0].Addresses[0]
			cmd := NewCommand("curl", "-sSf", "http://"+externalAddress)
			ok = retry(15*time.Minute, func() bool {
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
		res := T("app", "create", appName, env.Get("installedplatforms"), "-t", "{{.team}}", "-o", "{{.pool}}").Run(env)
		c.Assert(res, ResultOk)
		res = T("app", "info", "-a", appName).Run(env)
		c.Assert(res, ResultOk)
		res = T("env", "set", "-a", appName, "EVI_ENVIRONS='{\"INTEGRATION_ENV\":\"TRUE\"}'").Run(env)
		c.Assert(res, ResultOk)
		res = T("app", "deploy", "-a", appName, "-i", "{{.serviceimage}}").Run(env)
		c.Assert(res, ResultOk)

		appInfo := new(appTypes.AppInfo)
		ok := retry(5*time.Minute, func() (ready bool) {
			appInfo, ready = checkAppExternallyAddressable(c, appName, env)
			return ready
		})
		c.Assert(ok, check.Equals, true, check.Commentf("app not ready after 5 minutes: %v", res))
		c.Assert(appInfo.InternalAddresses, check.HasLen, 1)

		externalAddress := appInfo.Routers[0].Addresses[0]
		cmd := NewCommand("curl", "-sS", "-o", "/dev/null", "--write-out", "%{http_code}", "http://"+externalAddress)
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
		res = T("service", "template").Run(env)
		c.Assert(res, ResultOk)
		getServiceAddress := func() string {
			if env.Get("local") == "true" {
				fmt.Println("DEBUG: Using external address for service in local mode")
				return appInfo.Routers[0].Addresses[0]
			}
			fmt.Println("DEBUG: Using internal address for service")
			internalAddress := appInfo.InternalAddresses[0]
			return fmt.Sprintf("%s:%d", internalAddress.Domain, internalAddress.Port)
		}
		serviceAddress := getServiceAddress()
		replaces := map[string]string{
			"team_responsible_to_provide_service": "integration-team",
			"production-endpoint.com":             fmt.Sprintf("http://%s", serviceAddress),
			"servicename":                         "integration-service-" + env.Get("pool"),
		}
		for k, v := range replaces {
			res = NewCommand("sed", "-i", "-e", "'s~"+k+"~"+v+"~'", "manifest.yaml").Run(env)
			c.Assert(res, ResultOk)
		}
		res = T("service", "list").Run(env)
		c.Assert(res, ResultOk)

		res = T("service", "create", "manifest.yaml").Run(env)
		c.Assert(res, ResultOk)

		ok = retry(time.Minute, func() bool {
			res = T("service", "info", "integration-service-{{.pool}}").Run(env)
			return res.ExitCode == 0
		})

		c.Assert(ok, check.Equals, true, check.Commentf("invalid result: %v", res))

		env.Add("servicename", "integration-service-"+env.Get("pool"))
	}
	flow.backward = func(c *check.C, env *Environment) {
		res := T("service", "destroy", "integration-service-{{.pool}}", "-y").Run(env)
		c.Check(res, ResultOk)
		res = T("app", "remove", "-y", "-a", appName).Run(env)
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
		res := T("service", "instance", "add", "{{.servicename}}", bindName, "-t", "integration-team").Run(env)
		c.Assert(res, ResultOk)
		res = T("service", "instance", "bind", "{{.servicename}}", bindName, "-a", "{{.app}}").Run(env)
		c.Assert(res, ResultOk)
		ok := retry(15*time.Minute, func() bool {
			res = T("event", "list", "-k", "app.update.bind", "-v", "{{.app}}", "-r").Run(env)
			c.Assert(res, ResultOk)
			return res.Stdout.String() == ""
		})
		c.Assert(ok, check.Equals, true, check.Commentf("bind did not complete after 15 minutes: %v", res))
		res = T("event", "list", "-k", "app.update.bind", "-v", "{{.app}}").Run(env)
		c.Assert(res, ResultOk)
		c.Assert(res, ResultMatches, Expected{Stdout: `.*true.*`}, check.Commentf("event did not succeed"))
		ok = retry(time.Minute, func() bool {
			res = T("env", "get", "-a", "{{.app}}").Run(env)
			c.Check(res, ResultOk)
			return strings.Contains(res.Stdout.String(), "INTEGRATION_ENV=")
		})
		c.Assert(ok, check.Equals, true, check.Commentf("env not gettable after 1 minute: %v", res))
		cmd := T("app", "run", "-a", "{{.app}}", "env")
		ok = retry(time.Minute, func() bool {
			res = cmd.Run(env)
			return strings.Contains(res.Stdout.String(), "INTEGRATION_ENV=TRUE")
		})
		c.Assert(ok, check.Equals, true, check.Commentf("env not injected after 1 minute: %v", res))
		env.Add("bindnames", bindName)
	}
	flow.backward = func(c *check.C, env *Environment) {
		res := T("service", "instance", "remove", "{{.servicename}}", bindName, "-f", "-y").Run(env)
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
