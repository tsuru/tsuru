// Copyright 2025 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"time"

	check "gopkg.in/check.v1"
)

func multiversionRoutableStopStartTest() ExecFlow {
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
		appName := slugifyName(fmt.Sprintf("mv-start-stop-%s", env.Get("pool")))

		// Create the test application
		res := T("app", "create", appName, "python-iplat", "-t", "{{.team}}", "-o", "{{.pool}}").Run(env)
		c.Assert(res, ResultOk)

		// Map to track image -> hash relationship
		imageToHash := make(map[string]string)

		// Step 1: Deploy initial version (version 1)
		hash1 := deployAndMapHash(c, appDir, appName, []string{}, imageToHash, env)
		checkAppHealth(c, appName, "1", hash1, env)

		// Step 2: Deploy second version (version 2)
		hash2 := deployAndMapHash(c, appDir, appName, []string{"--new-version"}, imageToHash, env)

		// Step 3: Set version 2 as a routable version
		res, ok := T("app", "router", "version", "add", "2", "-a", appName).Retry(time.Minute, env, RetryOptions{})
		c.Assert(res, ResultOk)
		c.Assert(ok, check.Equals, true)

		// Verify multiversion is working - should see both version 1 and 2
		appInfoMulti := checkAppHealth(c, appName, "2", hash2, env)
		routerAddrMulti := appInfoMulti.Routers[0].Address
		cmd := NewCommand("curl", "-m5", "-sSf", "http://"+routerAddrMulti)
		hashRE := regexp.MustCompile(`.* version: (\d+) - hash: (\w+)$`)
		verifyVersionHashes(c, map[string]string{
			"1": hash1,
			"2": hash2,
		}, cmd, hashRE, env)

		// Step 4: App stop
		res = T("app", "stop", "-a", appName).Run(env)
		c.Assert(res, ResultOk)

		ok = retry(time.Minute*3, func() bool {
			_, appStoped := checkAppStopped(c, appName, env)
			return appStoped
		})
		c.Assert(ok, check.Equals, true)

		// Step 5: App start
		res = T("app", "start", "-a", appName).Run(env)
		c.Assert(res, ResultOk)

		appInfoMulti = checkAppHealth(c, appName, "2", hash2, env)
		routerAddrMulti = appInfoMulti.Routers[0].Address
		cmd = NewCommand("curl", "-m5", "-sSf", "http://"+routerAddrMulti)
		// if this fails, this means that app 2 is not coming back up properly as routable
		verifyVersionHashes(c, map[string]string{
			"1": hash1,
			"2": hash2,
		}, cmd, hashRE, env)
	}

	flow.backward = func(c *check.C, env *Environment) {
		appName := slugifyName(fmt.Sprintf("mv-start-stop-%s", env.Get("pool")))
		res := T("app", "remove", "-y", "-a", appName).Run(env)
		c.Check(res, ResultOk)
	}

	return flow
}
