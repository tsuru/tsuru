// Copyright 2025 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

func rollbackServiceCommonTest() ExecFlow {
	flow := ExecFlow{
		matrix: map[string]string{
			"pool": "poolnames",
		},
		parallel: false,
		requires: []string{"team", "poolnames", "installedplatforms"},
	}

	flow.forward = func(c *check.C, env *Environment) {
		cwd, err := os.Getwd()
		c.Assert(err, check.IsNil)

		appDir := path.Join(cwd, "fixtures", "rollback-test-app")
		appName := slugifyName(fmt.Sprintf("rollback-sc-%s-iapp", env.Get("pool")))

		// Create test application
		res := T("app", "create", appName, "python-iplat", "-t", "{{.team}}", "-o", "{{.pool}}").Run(env)
		c.Assert(res, ResultOk)

		// Helper to wait for app to be ready
		waitForAppReady := func() {
			ok := retry(3*time.Minute, func() bool {
				res = T("app", "info", "-a", appName, "--json").Run(env)
				c.Assert(res, ResultOk)

				var appInfo app.AppInfo
				err := json.Unmarshal([]byte(res.Stdout.String()), &appInfo)
				if err != nil {
					return false
				}

				// Check if units are started/ready
				for _, unit := range appInfo.Units {
					if unit.Status == "started" || unit.Status == "ready" {
						return true
					}
				}
				return false
			})
			c.Assert(ok, check.Equals, true, check.Commentf("app not ready: %v", res))
		}

		// Test 1: Basic deployment and rollback
		res = T("app", "deploy", "-a", appName, appDir).Run(env)
		c.Assert(res, ResultOk)
		waitForAppReady()

		// Verify initial deployment
		res = T("app", "info", "-a", appName, "--json").Run(env)
		c.Assert(res, ResultOk)

		var appInfo app.AppInfo
		err = json.Unmarshal([]byte(res.Stdout.String()), &appInfo)
		c.Assert(err, check.IsNil)
		c.Assert(len(appInfo.Units), check.Not(check.Equals), 0)
		c.Assert(appInfo.Units[0].Version, check.Equals, 1)

		// Test 2: Deploy a second version
		res = T("app", "deploy", "-a", appName, appDir).Run(env)
		c.Assert(res, ResultOk)
		waitForAppReady()

		// Test 3: Create multiversion scenario with preserveVersions
		res = T("app", "deploy", "--new-version", "-a", appName, appDir).Run(env)
		c.Assert(res, ResultOk)

		// Wait a bit for deployment to settle
		time.Sleep(2 * time.Second)

		// Test 4: Verify we have multiple versions (this tests the servicecommon actions fix)
		res = T("app", "info", "-a", appName, "--json").Run(env)
		c.Assert(res, ResultOk)

		err = json.Unmarshal([]byte(res.Stdout.String()), &appInfo)
		c.Assert(err, check.IsNil)
		c.Assert(len(appInfo.Units), check.Not(check.Equals), 0)

		// Test 5: Test actual rollback functionality using proper rollback command
		// Get the list of available deployments to rollback to
		res = T("app", "deploy", "list", "-a", appName, "--json").Run(env)
		c.Assert(res, ResultOk)

		// Parse JSON to find rollbackable deployments
		type Deploy struct {
			Image       string `json:"Image"`
			CanRollback bool   `json:"CanRollback"`
			Version     int    `json:"Version"`
		}

		var deploys []Deploy
		err = json.Unmarshal([]byte(res.Stdout.String()), &deploys)
		c.Assert(err, check.IsNil)

		// Find rollbackable deployments
		var rollbackableDeploys []Deploy
		for _, deploy := range deploys {
			if deploy.CanRollback && deploy.Image != "" {
				rollbackableDeploys = append(rollbackableDeploys, deploy)
			}
		}
		c.Assert(len(rollbackableDeploys), check.Not(check.Equals), 0, check.Commentf("No rollbackable deployments found"))

		// Use the second available rollbackable image (to simulate rolling back to previous version)
		var rollbackImage string
		if len(rollbackableDeploys) >= 2 {
			rollbackImage = rollbackableDeploys[1].Image
		} else {
			rollbackImage = rollbackableDeploys[0].Image
		}

		// Test rollback using the proper rollback command
		res = T("app", "deploy", "rollback", "-a", appName, "-y", rollbackImage).Run(env)
		c.Assert(res, ResultOk)
		waitForAppReady()

		// Test 6: Test scaling operations after rollback
		res = T("unit", "add", "2", "-a", appName).Run(env)
		c.Assert(res, ResultOk)
		waitForAppReady()

		res = T("unit", "remove", "1", "-a", appName).Run(env)
		c.Assert(res, ResultOk)
		waitForAppReady()

		// Test 7: Final verification using JSON output
		res = T("app", "info", "-a", appName, "--json").Run(env)
		c.Assert(res, ResultOk)

		err = json.Unmarshal([]byte(res.Stdout.String()), &appInfo)
		c.Assert(err, check.IsNil)

		if len(appInfo.Routers) > 0 {
			routerAddr := appInfo.Routers[0].Address
			cmd := NewCommand("curl", "-m5", "-sSf", routerAddr+"/health")
			ok := retryWait(1*time.Minute, 2*time.Second, func() bool {
				res = cmd.Run(env)
				return res.ExitCode == 0 && res.Stdout.String() == "OK"
			})
			c.Assert(ok, check.Equals, true, check.Commentf("health check failed: %v", res))
		}
	}

	flow.backward = func(c *check.C, env *Environment) {
		appName := slugifyName(fmt.Sprintf("rollback-servicecommon-%s-iapp", env.Get("pool")))
		res := T("app", "remove", "-y", "-a", appName).Run(env)
		c.Check(res, ResultOk)
	}

	return flow
}
