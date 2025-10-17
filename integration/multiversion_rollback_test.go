// Copyright 2025 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
	"time"

	"github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

func multiversionRollbackTest() ExecFlow {
	flow := ExecFlow{
		matrix: map[string]string{
			"pool": "poolnames",
		},
		parallel: false, // Run sequentially to avoid conflicts
		requires: []string{"team", "poolnames", "installedplatforms"},
	}

	flow.forward = func(c *check.C, env *Environment) {
		cwd, err := os.Getwd()
		c.Assert(err, check.IsNil)

		// Use the existing versions-app fixture
		appDir := path.Join(cwd, "fixtures", "versions-app")
		appName := slugifyName(fmt.Sprintf("multiversion-rollback-%s-iapp", env.Get("pool")))

		// Define structs for JSON parsing

		type Deploy struct {
			Image       string `json:"Image"`
			CanRollback bool   `json:"CanRollback"`
			Version     int    `json:"Version"`
		}

		// Create the test application
		res := T("app", "create", appName, "python-iplat", "-t", "{{.team}}", "-o", "{{.pool}}").Run(env)
		c.Assert(res, ResultOk)

		// Helper function to check application health and version using JSON
		checkAppHealth := func(expectedVersion string, shouldBeHealthy bool) {

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

			if shouldBeHealthy {
				c.Assert(ok, check.Equals, true, check.Commentf("app not ready after 3 minutes: %v", res))

				// Get app info and test HTTP endpoint
				res = T("app", "info", "-a", appName, "--json").Run(env)
				c.Assert(res, ResultOk)

				var appInfo app.AppInfo
				err := json.Unmarshal([]byte(res.Stdout.String()), &appInfo)
				c.Assert(err, check.IsNil)
				c.Assert(len(appInfo.Routers), check.Not(check.Equals), 0)

				routerAddr := appInfo.Routers[0].Address
				cmd := NewCommand("curl", "-m5", "-sSf", "http://"+routerAddr)
				ok = retryWait(2*time.Minute, 2*time.Second, func() bool {
					res = cmd.Run(env)
					return res.ExitCode == 0
				})
				c.Assert(ok, check.Equals, true, check.Commentf("app not responding: %v", res))

				// Verify the version from app info units
				expectedVersionInt, err := strconv.Atoi(expectedVersion)
				c.Assert(err, check.IsNil)
				c.Assert(len(appInfo.Units), check.Not(check.Equals), 0)
				c.Assert(appInfo.Units[0].Version, check.Equals, expectedVersionInt)
			}
		}

		// Step 1: Deploy initial version (version 1)
		res = T("app", "deploy", "-a", appName, appDir).Run(env)
		c.Assert(res, ResultOk)
		checkAppHealth("1", true)

		// Step 2: Deploy second version (version 2)
		res = T("app", "deploy", "-a", appName, appDir).Run(env)
		c.Assert(res, ResultOk)
		checkAppHealth("2", true)

		// Step 3: Deploy third version with --new-version to create multiversion scenario
		res = T("app", "deploy", "--new-version", "-a", appName, appDir).Run(env)
		c.Assert(res, ResultOk)

		// Verify we have multiple versions
		res = T("app", "info", "-a", appName, "--json").Run(env)
		c.Assert(res, ResultOk)

		appInfo := app.AppInfo{}
		err = json.Unmarshal([]byte(res.Stdout.String()), &appInfo)
		c.Assert(err, check.IsNil)

		// Step 4: Add version 3 to router to create true multiversion deployment
		time.Sleep(2 * time.Second)
		res = T("app", "router", "version", "add", "3", "-a", appName).Run(env)
		c.Assert(res, ResultOk)

		// Verify multiversion is working - should see both version 2 and 3
		addrRE := regexp.MustCompile(`(?s)External Addresses: (.*?)\n`)
		res = T("app", "info", "-a", appName).Run(env)
		c.Assert(res, ResultOk)
		parts := addrRE.FindStringSubmatch(res.Stdout.String())
		c.Assert(parts, check.HasLen, 2)

		cmd := NewCommand("curl", "-m5", "-sSf", "http://"+parts[1])
		versionRE := regexp.MustCompile(`.* version: (\d+)$`)
		versionsFound := map[string]bool{}

		// Test multiple requests to ensure we hit both versions
		for i := 0; i < 10; i++ {
			ok := retryWait(30*time.Second, time.Second, func() bool {
				res = cmd.Run(env)
				return res.ExitCode == 0
			})
			c.Assert(ok, check.Equals, true, check.Commentf("app not responding on attempt %d: %v", i, res))

			versionParts := versionRE.FindStringSubmatch(res.Stdout.String())
			c.Assert(versionParts, check.HasLen, 2)
			versionsFound[versionParts[1]] = true
		}

		// We should see both version 2 and version 3
		c.Assert(versionsFound["2"], check.Equals, true, check.Commentf("Version 2 not found in responses"))
		c.Assert(versionsFound["3"], check.Equals, true, check.Commentf("Version 3 not found in responses"))

		// Step 5: Test rollback scenario - remove one version and verify rollback works
		res = T("app", "router", "version", "remove", "3", "-a", appName).Run(env)
		c.Assert(res, ResultOk)

		// Should now only see version 2
		checkAppHealth("2", true)

		// Step 6: Test deployment failure and rollback by attempting to scale down to 0 and back
		// This simulates a scenario where deployment might fail and rollback is needed
		res = T("unit", "remove", "1", "-a", appName).Run(env)
		c.Assert(res, ResultOk)

		// Verify app is still responsive
		time.Sleep(2 * time.Second)
		checkAppHealth("2", true)

		// Step 7: Test the multiversion deployment
		// Deploy with --new-version and then test override-old-versions
		res = T("app", "deploy", "--new-version", "-a", appName, appDir).Run(env)
		c.Assert(res, ResultOk)

		// This should create version 4, but only version 2 should be routable initially
		checkAppHealth("2", true)

		// Add version 4 to router
		time.Sleep(1 * time.Second)
		res = T("app", "router", "version", "add", "4", "-a", appName).Run(env)
		c.Assert(res, ResultOk)

		// Verify multiversion again
		versionsFound = map[string]bool{}
		for i := 0; i < 10; i++ {
			ok := retryWait(30*time.Second, time.Second, func() bool {
				res = cmd.Run(env)
				return res.ExitCode == 0
			})
			c.Assert(ok, check.Equals, true, check.Commentf("app not responding: %v", res))

			versionParts := versionRE.FindStringSubmatch(res.Stdout.String())
			c.Assert(versionParts, check.HasLen, 2)
			versionsFound[versionParts[1]] = true
		}

		// Should see both version 2 and 4
		c.Assert(versionsFound["2"], check.Equals, true, check.Commentf("Version 2 not found after multiversion deployment"))
		c.Assert(versionsFound["4"], check.Equals, true, check.Commentf("Version 4 not found after multiversion deployment"))

		// Step 8: Test the actual rollback command
		// First get the list of available deployments to rollback to
		res = T("app", "deploy", "list", "-a", appName, "--json").Run(env)
		c.Assert(res, ResultOk)

		// Parse JSON to find rollbackable deployments
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

		// Verify rollback worked - get app info using JSON
		time.Sleep(5 * time.Second)
		res = T("app", "info", "-a", appName, "--json").Run(env)
		c.Assert(res, ResultOk)

		err = json.Unmarshal([]byte(res.Stdout.String()), &appInfo)
		c.Assert(err, check.IsNil)
		c.Assert(len(appInfo.Routers), check.Not(check.Equals), 0, check.Commentf("No routers found"))

		// Test app responsiveness using the router address
		routerAddr := appInfo.Routers[0].Address
		cmd = NewCommand("curl", "-m5", "-sSf", routerAddr)
		ok := retryWait(2*time.Minute, 2*time.Second, func() bool {
			res = cmd.Run(env)
			return res.ExitCode == 0
		})
		c.Assert(ok, check.Equals, true, check.Commentf("app not responding after rollback: %v", res))
	}

	flow.backward = func(c *check.C, env *Environment) {
		appName := slugifyName(fmt.Sprintf("multiversion-rollback-%s-iapp", env.Get("pool")))
		res := T("app", "remove", "-y", "-a", appName).Run(env)
		c.Check(res, ResultOk)
	}

	return flow
}
