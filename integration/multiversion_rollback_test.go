// Copyright 2025 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
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

		// Use the new multiversion-python-app fixture
		appDir := path.Join(cwd, "fixtures", "multiversion-python-app")
		appName := slugifyName(fmt.Sprintf("mv-rollback-%s", env.Get("pool")))

		// Define structs for JSON parsing
		type Deploy struct {
			Image       string `json:"Image"`
			CanRollback bool   `json:"CanRollback"`
			Version     int    `json:"Version"`
		}

		type VersionResponse struct {
			App     string `json:"app"`
			Version string `json:"version"`
			Hash    string `json:"hash"`
		}

		// Helper function to generate hash before deployment
		generateHashForDeploy := func() string {
			cmd := exec.Command("bash", "./generate_hash.sh")
			cmd.Dir = appDir
			err := cmd.Run()
			c.Assert(err, check.IsNil)

			hashBytes, err := os.ReadFile(path.Join(appDir, "version_hash.txt"))
			c.Assert(err, check.IsNil)
			return strings.TrimSpace(string(hashBytes))
		}

		// Helper function to deploy and map image to hash
		deployAndMapHash := func(imageToHash map[string]string, deployArgs ...string) string {
			hash := generateHashForDeploy()
			args := append([]string{"app", "deploy"}, deployArgs...)
			res := T(args...).Run(env)
			c.Assert(res, ResultOk)

			// Get the latest deploy and map image to hash
			res = T("app", "deploy", "list", "-a", appName, "--json").Run(env)
			c.Assert(res, ResultOk)
			var deploys []Deploy
			err := json.Unmarshal([]byte(res.Stdout.String()), &deploys)
			c.Assert(err, check.IsNil)
			for _, deploy := range deploys {
				if _, exists := imageToHash[deploy.Image]; !exists {
					imageToHash[deploy.Image] = hash
				}
			}
			return hash
		}

		// Create the test application
		res := T("app", "create", appName, "python-iplat", "-t", "{{.team}}", "-o", "{{.pool}}").Run(env)
		c.Assert(res, ResultOk)

		// Helper function to check application health and version using JSON
		checkAppHealth := func(expectedVersion string, expectedHash string, shouldBeHealthy bool) {

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

				// Verify the version from app info units - only check routable versions
				expectedVersionInt, err := strconv.Atoi(expectedVersion)
				c.Assert(err, check.IsNil)
				c.Assert(len(appInfo.Units), check.Not(check.Equals), 0)

				// Find a unit with the expected version (should be routable)
				foundExpectedVersion := false
				for _, unit := range appInfo.Units {
					if unit.Version == expectedVersionInt && unit.Routable {
						foundExpectedVersion = true
						break
					}
				}
				c.Assert(foundExpectedVersion, check.Equals, true, check.Commentf("Expected version %d not found in routable units", expectedVersionInt))

				// Verify the hash via /version endpoint
				if expectedHash != "" {
					versionCmd := NewCommand("curl", "-m5", "-sSf", "http://"+routerAddr+"/version")
					ok = retryWait(30*time.Second, 2*time.Second, func() bool {
						res = versionCmd.Run(env)
						if res.ExitCode != 0 {
							return false
						}
						var versionResp VersionResponse
						err := json.Unmarshal([]byte(res.Stdout.String()), &versionResp)
						if err != nil {
							return false
						}
						return versionResp.Hash == expectedHash
					})
					c.Assert(ok, check.Equals, true, check.Commentf("hash verification failed, expected: %s", expectedHash))
				}
			}
		}

		// Map to track image -> hash relationship
		imageToHash := make(map[string]string)

		// Step 1: Deploy initial version (version 1)
		hash1 := deployAndMapHash(imageToHash, "-a", appName, appDir)
		checkAppHealth("1", hash1, true)

		// Step 2: Deploy second version (version 2)
		hash2 := deployAndMapHash(imageToHash, "-a", appName, appDir)
		checkAppHealth("2", hash2, true)

		// Step 3: Deploy third version with --new-version to create multiversion scenario
		hash3 := deployAndMapHash(imageToHash, "--new-version", "-a", appName, appDir)

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
		res = T("app", "info", "-a", appName, "--json").Run(env)
		c.Assert(res, ResultOk)

		var appInfoMulti app.AppInfo
		err = json.Unmarshal([]byte(res.Stdout.String()), &appInfoMulti)
		c.Assert(err, check.IsNil)
		c.Assert(len(appInfoMulti.Routers), check.Not(check.Equals), 0)

		routerAddrMulti := appInfoMulti.Routers[0].Address
		cmd := NewCommand("curl", "-m5", "-sSf", "http://"+routerAddrMulti)
		hashRE := regexp.MustCompile(`.* version: (\d+) - hash: (\w+)$`)
		versionsFound := map[string]bool{}
		hashesFound := map[string]bool{}

		// Test multiple requests to ensure we hit both versions
		for i := 0; i < 20; i++ {
			ok := retryWait(30*time.Second, time.Second, func() bool {
				res = cmd.Run(env)
				return res.ExitCode == 0
			})
			c.Assert(ok, check.Equals, true, check.Commentf("app not responding on attempt %d: %v", i, res))

			hashParts := hashRE.FindStringSubmatch(res.Stdout.String())
			c.Assert(hashParts, check.HasLen, 3)
			version := hashParts[1]
			hash := hashParts[2]
			versionsFound[version] = true
			hashesFound[hash] = true

			// Verify hash matches expected version
			if version == "2" {
				c.Assert(hash, check.Equals, hash2)
			} else if version == "3" {
				c.Assert(hash, check.Equals, hash3)
			}

			if len(versionsFound) == 2 {
				break
			}

			time.Sleep(500 * time.Millisecond)
		}

		// We should see both version 2 and version 3 with correct hashes
		c.Assert(versionsFound["2"], check.Equals, true, check.Commentf("Version 2 not found in responses"))
		c.Assert(versionsFound["3"], check.Equals, true, check.Commentf("Version 3 not found in responses"))
		c.Assert(hashesFound[hash2], check.Equals, true, check.Commentf("Hash for version 2 not found"))
		c.Assert(hashesFound[hash3], check.Equals, true, check.Commentf("Hash for version 3 not found"))

		// Step 5: Test rollback scenario - remove one version and verify rollback works
		res = T("app", "router", "version", "remove", "3", "-a", appName).Run(env)
		c.Assert(res, ResultOk)

		// Should now only see version 2
		time.Sleep(10 * time.Second)
		checkAppHealth("2", hash2, true)

		// Step 6: Test unit remove and add
		res = T("unit", "remove", "1", "-a", appName, "--version", "2").Run(env)
		c.Assert(res, ResultOk)
		res = T("unit", "add", "1", "-a", appName, "--version", "2").Run(env)
		c.Assert(res, ResultOk)

		// Step 7: Test the multiversion deployment
		// Deploy with --new-version and then test override-old-versions
		hash4 := deployAndMapHash(imageToHash, "--new-version", "-a", appName, appDir)

		// This should create version 4, but only version 2 should be routable initially
		time.Sleep(10 * time.Second)
		checkAppHealth("2", hash2, true)

		// Add version 4 to router
		time.Sleep(1 * time.Second)
		res = T("app", "router", "version", "add", "4", "-a", appName).Run(env)
		c.Assert(res, ResultOk)

		// Verify multiversion again - check both versions and their hashes
		versionsFound = map[string]bool{}
		hashesFound = map[string]bool{}
		for i := 0; i < 20; i++ {
			ok := retryWait(30*time.Second, time.Second, func() bool {
				res = cmd.Run(env)
				return res.ExitCode == 0
			})
			c.Assert(ok, check.Equals, true, check.Commentf("app not responding: %v", res))

			hashParts := hashRE.FindStringSubmatch(res.Stdout.String())
			c.Assert(hashParts, check.HasLen, 3)
			version := hashParts[1]
			hash := hashParts[2]
			versionsFound[version] = true
			hashesFound[hash] = true

			// Verify hash matches expected version
			if version == "2" {
				c.Assert(hash, check.Equals, hash2)
			} else if version == "4" {
				c.Assert(hash, check.Equals, hash4)
			}

			if len(versionsFound) == 2 {
				break
			}

			time.Sleep(500 * time.Millisecond)
		}

		// Should see both version 2 and 4 with correct hashes
		c.Assert(versionsFound["2"], check.Equals, true, check.Commentf("Version 2 not found after multiversion deployment"))
		c.Assert(versionsFound["4"], check.Equals, true, check.Commentf("Version 4 not found after multiversion deployment"))
		c.Assert(hashesFound[hash2], check.Equals, true, check.Commentf("Hash for version 2 not found"))
		c.Assert(hashesFound[hash4], check.Equals, true, check.Commentf("Hash for version 4 not found"))

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
		var expectedRollbackHash string
		if len(rollbackableDeploys) >= 2 {
			rollbackImage = rollbackableDeploys[1].Image
		} else {
			rollbackImage = rollbackableDeploys[0].Image
		}
		// Get the expected hash for the rollback image
		// Debug: Print all imageToHash contents
		fmt.Printf("DEBUG: imageToHash contents:\n")
		for image, hash := range imageToHash {
			fmt.Printf("  Image: %s -> Hash: %s\n", image, hash)
		}
		fmt.Printf("DEBUG: Looking for rollbackImage: %s\n", rollbackImage)

		expectedRollbackHash = imageToHash[rollbackImage]
		c.Assert(expectedRollbackHash, check.Not(check.Equals), "", check.Commentf("Hash not found for rollback image: %s", rollbackImage))

		// Test rollback using the proper rollback command
		res = T("app", "deploy", "rollback", "-a", appName, "-y", "--new-version", rollbackImage).Run(env)
		c.Assert(res, ResultOk)

		// Verify rollback worked - get app info using JSON to find version 5 (rollback creates new version)
		time.Sleep(5 * time.Second)
		res = T("app", "info", "-a", appName, "--json").Run(env)
		c.Assert(res, ResultOk)

		err = json.Unmarshal([]byte(res.Stdout.String()), &appInfo)
		c.Assert(err, check.IsNil)
		c.Assert(len(appInfo.Routers), check.Not(check.Equals), 0, check.Commentf("No routers found"))

		// Add version 5 (the rollback version) to router to make it routable
		res = T("app", "router", "version", "add", "5", "-a", appName).Run(env)
		c.Assert(res, ResultOk)

		// Test app responsiveness and verify the hash matches the expected rollback hash
		routerAddr := appInfo.Routers[0].Address
		versionCmd := NewCommand("curl", "-m5", "-sSf", "http://"+routerAddr+"/version")
		ok := retryWait(2*time.Minute, 2*time.Second, func() bool {
			res = versionCmd.Run(env)
			if res.ExitCode != 0 {
				return false
			}
			var versionResp VersionResponse
			err := json.Unmarshal([]byte(res.Stdout.String()), &versionResp)
			if err != nil {
				return false
			}
			// Verify the rollback (version 5) has the correct hash from the original image
			return versionResp.Hash == expectedRollbackHash && versionResp.Version == "5"
		})
		c.Assert(ok, check.Equals, true, check.Commentf("rollback hash verification failed, expected hash: %s for version 5", expectedRollbackHash))
	}

	flow.backward = func(c *check.C, env *Environment) {
		appName := slugifyName(fmt.Sprintf("mv-rollback-%s", env.Get("pool")))
		res := T("app", "remove", "-y", "-a", appName).Run(env)
		c.Check(res, ResultOk)
	}

	return flow
}
