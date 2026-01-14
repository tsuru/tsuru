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
	"time"

	"gopkg.in/check.v1"
)

func multiversionRollbackOverrideTest() ExecFlow {
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
		appName := slugifyName(fmt.Sprintf("mv-rollback-override-%s", env.Get("pool")))

		// Create the test application
		res := T("app", "create", appName, "python-iplat", "-t", "{{.team}}", "-o", "{{.pool}}").Run(env)
		c.Assert(res, ResultOk)

		// Map to track image -> hash relationship
		imageToHash := make(map[string]string)

		// Step 1: Deploy initial version (version 1)
		hash1 := deployAndMapHash(c, appDir, appName, []string{}, imageToHash, env)
		checkAppHealth(c, appName, "1", hash1, env)

		// Step 2: Deploy second version with --new-version (version 2)
		hash2 := deployAndMapHash(c, appDir, appName, []string{"--new-version"}, imageToHash, env)
		checkAppHealth(c, appName, "1", hash1, env)

		// Step 3: Add version 2 to router to create true multiversion deployment
		res, ok := T("app", "router", "version", "add", "2", "-a", appName).Retry(time.Minute, env, RetryOptions{})
		c.Assert(res, ResultOk)
		c.Assert(ok, check.Equals, true)

		// Verify multiversion is working - should see both version 1 and 2
		appInfoMulti := checkAppHealth(c, appName, "2", hash2, env)
		routerAddrMulti := appInfoMulti.Routers[0].Address
		cmd := NewCommand("curl", "-m5", "-sSf", "http://"+routerAddrMulti)
		hashRE := regexp.MustCompile(`.* version: (\d+) - hash: (\w+)$`)

		// Test multiple requests to ensure we hit both versions
		verifyVersionHashes(c, map[string]string{"1": hash1, "2": hash2}, cmd, hashRE, env)

		// Step 4: Deploy third version with --new-version (version 3)
		hash3 := deployAndMapHash(c, appDir, appName, []string{"--new-version"}, imageToHash, env)
		checkAppHealth(c, appName, "1", hash1, env)
		checkAppHealth(c, appName, "2", hash2, env)

		// Step 5: Add version 3 to router to create true multiversion deployment
		res, ok = T("app", "router", "version", "add", "3", "-a", appName).Retry(time.Minute, env, RetryOptions{})
		c.Assert(res, ResultOk)
		c.Assert(ok, check.Equals, true)

		// Verify multiversion is working - should see all versions 1, 2 and 3
		appInfoMulti = checkAppHealth(c, appName, "3", hash3, env)
		routerAddrMulti = appInfoMulti.Routers[0].Address
		cmd = NewCommand("curl", "-m5", "-sSf", "http://"+routerAddrMulti)

		// Test multiple requests to ensure we hit both versions
		verifyVersionHashes(c, map[string]string{"1": hash1, "2": hash2, "3": hash3}, cmd, hashRE, env)

		// Step 6: Rollback to version 2 with override
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
		c.Assert(len(rollbackableDeploys), check.Equals, 3, check.Commentf("Expected 3 rollbackable deployments, found %d", len(rollbackableDeploys)))
		rollbackImage := rollbackableDeploys[1].Image // Image for version 2
		// Get the expected hash for the rollback image
		fmt.Printf("DEBUG: imageToHash contents:\n")
		for image, hash := range imageToHash {
			fmt.Printf("  Image: %s -> Hash: %s\n", image, hash)
		}
		fmt.Printf("DEBUG: Looking for rollbackImage: %s\n", rollbackImage)
		expectedRollbackHash, ok := imageToHash[rollbackImage]
		c.Assert(ok, check.Equals, true, check.Commentf("Hash not found for rollback image: %s", rollbackImage))
		c.Assert(expectedRollbackHash, check.Not(check.Equals), "", check.Commentf("Hash not found for rollback image: %s", rollbackImage))

		// Test rollback using the proper rollback command to override old versions
		res = T("app", "deploy", "rollback", "-a", appName, "-y", "--override-old-versions", rollbackImage).Run(env)
		c.Assert(res, ResultOk)
		checkAppHealth(c, appName, "2", expectedRollbackHash, env)
		verifyVersionHashes(c, map[string]string{"2": expectedRollbackHash}, cmd, hashRE, env)

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

		// Step 7: Ensure only one deployment exists after rollback with override
		res = K("get", "deployments", "-l", fmt.Sprintf("tsuru.io/app-name=%s", appName), "-o", "json").Run(env)
		c.Assert(res, ResultOk)
		var deploymentList struct {
			Items []struct{} `json:"items"`
		}
		err = json.Unmarshal([]byte(res.Stdout.String()), &deploymentList)
		c.Assert(err, check.IsNil)
		count := len(deploymentList.Items)
		c.Assert(count, check.Equals, 1, check.Commentf("Expected only one deployment after rollback, found %d", count))

		// Step 8: Ensure user is able to deploy new version after rollback with override
		hash4 := deployAndMapHash(c, appDir, appName, []string{}, imageToHash, env)
		checkAppHealth(c, appName, "4", hash4, env)
	}

	flow.backward = func(c *check.C, env *Environment) {
		appName := slugifyName(fmt.Sprintf("mv-rollback-override-%s", env.Get("pool")))
		res := T("app", "remove", "-y", "-a", appName).Run(env)
		c.Check(res, ResultOk)
	}
	return flow
}
