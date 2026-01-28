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
	"strings"
	"time"

	"github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

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

		// Create the test application
		res := T("app", "create", appName, "python-iplat", "-t", "{{.team}}", "-o", "{{.pool}}").Run(env)
		c.Assert(res, ResultOk)

		// Map to track image -> hash relationship
		imageToHash := make(map[string]string)

		// Step 1: Deploy initial version (version 1)
		hash1 := deployAndMapHash(c, appDir, appName, []string{}, imageToHash, env)
		checkAppHealth(c, appName, "1", hash1, env)

		// Step 2: Deploy second version (version 2)
		hash2 := deployAndMapHash(c, appDir, appName, []string{}, imageToHash, env)
		checkAppHealth(c, appName, "2", hash2, env)

		// Step 3: Deploy third version with --new-version to create multiversion scenario
		hash3 := deployAndMapHash(c, appDir, appName, []string{"--new-version"}, imageToHash, env)
		checkAppHealth(c, appName, "2", hash2, env)

		// Step 4: Add version 3 to router to create true multiversion deployment
		res, ok := T("app", "router", "version", "add", "3", "-a", appName).Retry(time.Minute, env, RetryOptions{})
		c.Assert(res, ResultOk)
		c.Assert(ok, check.Equals, true)

		// Verify multiversion is working - should see both version 2 and 3
		appInfoMulti := checkAppHealth(c, appName, "3", hash3, env)
		routerAddrMulti := appInfoMulti.Routers[0].Address
		cmd := NewCommand("curl", "-m5", "-sSf", "http://"+routerAddrMulti)
		hashRE := regexp.MustCompile(`.* version: (\d+) - hash: (\w+)$`)

		// Test multiple requests to ensure we hit both versions
		verifyVersionHashes(c, map[string]string{
			"2": hash2,
			"3": hash3,
		}, cmd, hashRE, env)

		// Step 5: Test rollback scenario - remove one version and verify rollback works
		res, ok = T("app", "router", "version", "remove", "3", "-a", appName).Retry(time.Minute, env, RetryOptions{})
		c.Assert(res, ResultOk)
		c.Assert(ok, check.Equals, true)

		// Should now only see version 2
		checkAppHealth(c, appName, "2", hash2, env)

		// Step 6: Test unit remove and add
		res = T("unit", "remove", "1", "-a", appName, "--version", "2").Run(env)
		c.Assert(res, ResultOk)
		res = T("unit", "add", "1", "-a", appName, "--version", "2").Run(env)
		c.Assert(res, ResultOk)

		// Step 7: Test the multiversion deployment
		// Deploy with --new-version and then test override-old-versions
		hash4 := deployAndMapHash(c, appDir, appName, []string{"--new-version"}, imageToHash, env)

		// This should create version 4, but only version 2 should be routable initially
		checkAppHealth(c, appName, "2", hash2, env)

		// Add version 4 to router
		res, ok = T("app", "router", "version", "add", "4", "-a", appName).Retry(time.Minute, env, RetryOptions{})
		c.Assert(res, ResultOk)
		c.Assert(ok, check.Equals, true)
		checkAppHealth(c, appName, "4", hash4, env)

		// Verify multiversion again - check both versions and their hashes
		verifyVersionHashes(c, map[string]string{
			"2": hash2,
			"4": hash4,
		}, cmd, hashRE, env)

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

		// Test rollback using the proper rollback command to override old versions
		res = T("app", "deploy", "rollback", "-a", appName, "-y", "--override-old-versions", rollbackImage).Run(env)
		c.Assert(res, ResultOk)

		// Verify rollback worked - get app info using JSON to find version 3
		appInfo := checkAppHealth(c, appName, "3", expectedRollbackHash, env)

		// Test app responsiveness and verify the hash matches the expected rollback hash
		routerAddr := appInfo.Routers[0].Address
		versionCmd := NewCommand("curl", "-m5", "-sSf", "http://"+routerAddr+"/version")
		retryWait(2*time.Minute, 2*time.Second, func() bool {
			res = versionCmd.Run(env)
			if res.ExitCode != 0 {
				return false
			}
			var versionResp VersionResponse
			err := json.Unmarshal([]byte(res.Stdout.String()), &versionResp)
			if err != nil {
				return false
			}
			// Verify the rollback (version 3) has the correct hash from the original image
			return versionResp.Hash == expectedRollbackHash && versionResp.Version == "3"
		})

		// Find and verify version 4 image is available for rollback
		var version4Image string
		for _, deploy := range rollbackableDeploys {
			if deploy.Image != "" {
				if hash, exists := imageToHash[deploy.Image]; exists && hash == hash4 {
					version4Image = deploy.Image
					break
				}
			}
		}
		c.Assert(version4Image, check.Not(check.Equals), "", check.Commentf("Version 4 image not found in rollbackable deployments"))

		// Get deploy list again for debugging
		res = T("app", "deploy", "list", "-a", appName, "--json").Run(env)
		c.Assert(res, ResultOk)

		// wait k8s sync
		ok = retry(2*time.Minute, func() (ready bool) {
			res = K("get", "deployments", "-l", fmt.Sprintf("tsuru.io/app-name=%s", appName), "-o", "json").Run(env)
			c.Assert(res, ResultOk)

			// Count deployments by parsing JSON output
			var deploymentList struct {
				Items []struct{} `json:"items"`
			}
			err := json.Unmarshal([]byte(res.Stdout.String()), &deploymentList)
			c.Assert(err, check.IsNil)
			count := len(deploymentList.Items)

			c.Assert(count, check.Not(check.Equals), 0, check.Commentf("No deployment found for app"))
			fmt.Println("DEBUG: Matches found for app deployment:", count)
			if count > 1 {
				fmt.Printf("DEBUG: Multiple deployments found for app deployment: %v\n", count)
				return false
			}
			return true
		})
		c.Assert(ok, check.Equals, true, check.Commentf("Kubernetes sync did not happen within 2 minutes"))

		// Test rollback to run old version alongside currnent one
		res = T("app", "deploy", "rollback", "-a", appName, "-y", "--new-version", version4Image).Run(env)
		c.Assert(res, ResultOk)

		// Verify that only version 3 is routable and version 4 is not routable
		time.Sleep(10 * time.Second)
		res = T("app", "info", "-a", appName, "--json").Run(env)
		c.Assert(res, ResultOk)

		err = json.Unmarshal([]byte(res.Stdout.String()), &appInfoMulti)
		c.Assert(err, check.IsNil)
		c.Assert(len(appInfoMulti.Units), check.Not(check.Equals), 0)

		// Verify version 3 is routable and version 4 is not routable
		foundVersion3Routable := false
		foundVersion4NotRoutable := false
		for _, unit := range appInfoMulti.Units {
			if unit.Version == 3 && unit.Routable {
				foundVersion3Routable = true
			}
			if unit.Version == 4 && !unit.Routable {
				foundVersion4NotRoutable = true
			}
		}
		c.Assert(foundVersion3Routable, check.Equals, true, check.Commentf("Version 3 should be routable after rollback with --new-version"))
		c.Assert(foundVersion4NotRoutable, check.Equals, true, check.Commentf("Version 4 should not be routable after rollback with --new-version"))

		// Add version 4 to router to test multiversion with versions 3 and 4
		res, ok = T("app", "router", "version", "add", "4", "-a", appName).Retry(time.Minute, env, RetryOptions{})
		c.Assert(res, ResultOk)
		c.Assert(ok, check.Equals, true)

		// Verify multiversion is working - should see both version 3 and 4
		time.Sleep(10 * time.Second)
		res = T("app", "info", "-a", appName, "--json").Run(env)
		c.Assert(res, ResultOk)

		err = json.Unmarshal([]byte(res.Stdout.String()), &appInfoMulti)
		c.Assert(err, check.IsNil)
		c.Assert(len(appInfoMulti.Routers), check.Not(check.Equals), 0)

		// Test multiple requests to ensure we hit both versions
		verifyVersionHashes(c, map[string]string{
			"4": hash4,
			"3": expectedRollbackHash,
		}, cmd, hashRE, env)
	}

	flow.backward = func(c *check.C, env *Environment) {
		appName := slugifyName(fmt.Sprintf("mv-rollback-%s", env.Get("pool")))
		res := T("app", "remove", "-y", "-a", appName).Run(env)
		c.Check(res, ResultOk)
	}

	return flow
}

func verifyVersionHashes(c *check.C, expectedVersions map[string]string, testCmd *Command, hashRE *regexp.Regexp, env *Environment) {
	versionsFound := map[string]bool{}
	hashesFound := map[string]bool{}

	for i := range 20 {
		var res *Result
		ok := retryWait(30*time.Second, time.Second, func() bool {
			res = testCmd.Run(env)
			return res.ExitCode == 0
		})
		c.Assert(ok, check.Equals, true, check.Commentf("app not responding on attempt %d", i))

		hashParts := hashRE.FindStringSubmatch(res.Stdout.String())
		c.Assert(hashParts, check.HasLen, 3)
		version := hashParts[1]
		hash := hashParts[2]
		versionsFound[version] = true
		hashesFound[hash] = true

		// Verify hash matches expected version
		for expectedVersion, expectedHash := range expectedVersions {
			if version == expectedVersion {
				c.Assert(hash, check.Equals, expectedHash)
			}
		}

		if len(versionsFound) == len(expectedVersions) {
			break
		}

		time.Sleep(500 * time.Millisecond)
	}

	// Verify all expected versions were found
	for version, hash := range expectedVersions {
		c.Assert(versionsFound[version], check.Equals, true, check.Commentf("Version %s not found", version))
		c.Assert(hashesFound[hash], check.Equals, true, check.Commentf("Hash for version %s not found", version))
	}
}

func generateHashForDeploy(c *check.C, appDir string, env *Environment) string {
	cmd := NewCommand("bash", "./generate_hash.sh").WithPWD(appDir)
	result := cmd.Run(env)
	c.Assert(result.ExitCode, check.Equals, 0)

	hashBytes, err := os.ReadFile(path.Join(appDir, "version_hash.txt"))
	c.Assert(err, check.IsNil)
	return strings.TrimSpace(string(hashBytes))
}

func deployAndMapHash(c *check.C, appDir, appName string, deployArgs []string, imageToHash map[string]string, env *Environment) string {
	hash := generateHashForDeploy(c, appDir, env)
	fmt.Println("DEBUG: Deploying with hash:", hash)

	res := T("app", "update", "-a", appName, "-i").Run(env)
	c.Assert(res, ResultOk)

	args := append([]string{"app", "deploy", "-a", appName, appDir}, deployArgs...)
	res = T(args...).Run(env)
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

func checkAppHealth(c *check.C, appName, expectedVersion, expectedHash string, env *Environment) *app.AppInfo {
	res := new(Result)
	appInfo := new(app.AppInfo)
	ok := retry(3*time.Minute, func() (ready bool) {
		appInfo, ready = checkAppExternallyAddressable(c, appName, env)
		if !ready {
			return false
		}
		// Check that all pods are running and not being deleted
		res := K("get", "pods", "-l", fmt.Sprintf("tsuru.io/app-name=%s", appName), "-o", `"jsonpath={range .items[*]}{.status.phase},{.metadata.deletionTimestamp}{'\\n'}{end}"`).Run(env)
		c.Assert(res, ResultOk)
		podStatuses := strings.SplitSeq(strings.TrimSpace(res.Stdout.String()), "\n")
		for status := range podStatuses {
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
	c.Assert(ok, check.Equals, true, check.Commentf("app not ready after 3 minutes: %v", appInfo))

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

	if expectedHash == "" {
		return appInfo
	}
	// Verify the hash via /version endpoint
	versionCmd := NewCommand("curl", "-m5", "-sSf", "http://"+routerAddr+"/version")
	ok = retryWait(30*time.Second, 2*time.Second, func() bool {
		res = versionCmd.Run(env)
		if res.ExitCode != 0 {
			return false
		}
		var versionResp VersionResponse
		err := json.Unmarshal([]byte(res.Stdout.String()), &versionResp)
		if err != nil {
			fmt.Printf("DEBUG: Failed to parse version response: %s\n", err.Error())
			return false
		}
		if versionResp.Hash != expectedHash {
			fmt.Printf("DEBUG: Hash mismatch: expected %s (version %s), got %s (version %s)\n",
				expectedHash, expectedVersion, versionResp.Hash, versionResp.Version)
			return false
		}
		return true
	})
	c.Assert(ok, check.Equals, true, check.Commentf("hash verification failed, expected: %s", expectedHash))
	return appInfo
}
