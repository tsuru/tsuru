// Copyright 2025 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strconv"
	"time"

	appType "github.com/tsuru/tsuru/types/app"
	provType "github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
)

func swapAutoScaleTest() ExecFlow {
	flow := ExecFlow{
		matrix: map[string]string{
			"pool": "poolnames",
		},
		requires: []string{"team", "poolnames"},
	}

	flow.forward = func(c *check.C, env *Environment) {
		cwd, err := os.Getwd()
		c.Assert(err, check.IsNil)
		// Use the new multiversion-python-app fixture
		appDir := path.Join(cwd, "fixtures", "multiversion-python-app")
		appName := slugifyName(fmt.Sprintf("mv-swap-autoscale-%s", env.Get("pool")))

		// Create the test application
		res := T("app", "create", appName, "python-iplat", "-t", "{{.team}}", "-o", "{{.pool}}").Run(env)
		c.Assert(res, ResultOk)

		// Map to track image -> hash relationship
		imageToHash := make(map[string]string)

		// Step 1: Deploy initial version (version 1)
		deployAndMapHash(c, appDir, appName, []string{}, imageToHash, env)

		// Step 2: Deploy initial version (version 2)
		deployAndMapHash(c, appDir, appName, []string{"--new-version"}, imageToHash, env)

		// Step 3: Set Autoscale
		var stabWindow int32 = 10
		var percPolicyValue int32 = 100
		var unitPolicyValue int32 = 4
		autoscaleSpec := provType.AutoScaleSpec{
			Process:  "web",
			MinUnits: 1,
			MaxUnits: 5,
			Schedules: []provType.AutoScaleSchedule{{
				MinReplicas: 5,
				Start:       "1-59/2 * * * *",
				End:         "2-58/2 * * * *",
				Timezone:    "UTC",
			},
			},
			Version: 1,
			Behavior: provType.BehaviorAutoScaleSpec{
				ScaleDown: &provType.ScaleDownPolicy{
					StabilizationWindow:   &stabWindow,
					PercentagePolicyValue: &percPolicyValue,
					UnitsPolicyValue:      &unitPolicyValue,
				},
			},
		}
		schdl := `'{"minReplicas": ` + strconv.Itoa(autoscaleSpec.Schedules[0].MinReplicas) +
			`, "start": "` + autoscaleSpec.Schedules[0].Start + `"` +
			`, "end": "` + autoscaleSpec.Schedules[0].End + `"}'`
		res = T("unit", "autoscale", "set", "-a", appName,
			"--min", strconv.Itoa(int(autoscaleSpec.MinUnits)),
			"--max", strconv.Itoa(int(autoscaleSpec.MaxUnits)),
			"--schedule", schdl,
			"--scale-down-units", strconv.Itoa(int(*autoscaleSpec.Behavior.ScaleDown.UnitsPolicyValue)),
			"--sdp", strconv.Itoa(int(*autoscaleSpec.Behavior.ScaleDown.PercentagePolicyValue)),
			"--sdsw", strconv.Itoa(int(*autoscaleSpec.Behavior.ScaleDown.StabilizationWindow))).Run(env)
		c.Assert(res, ResultOk)

		retryOpt := func(version string) RetryOptions {
			return RetryOptions{CheckResult: func(r *Result) bool {
				data := make(map[string]string)
				err = json.Unmarshal([]byte(r.Stdout.String()), &data)
				if err != nil {
					fmt.Printf("DEBUG: Failed Unmarshal JSON: %s\n", err)
				}
				if _, ok := data["scaledobject.keda.sh/name"]; !ok {
					return false
				}
				v, ok := data["tsuru.io/app-version"]
				return ok && version == v
			}}
		}

		//debug
		res, ok := K("get", "hpa", "-l", "app="+appName+"-web", "-o", "jsonpath='{.items[?(@.metadata.labels)].metadata.labels}'").Retry(time.Minute, env, retryOpt("1"))
		c.Assert(res, ResultOk)
		c.Assert(ok, check.Equals, true)

		// Step 4: Check Autoscale
		var appInfo appType.AppInfo
		res = T("app", "info", "-a", appName, "--json").Run(env)
		c.Assert(res, ResultOk)
		err = json.Unmarshal([]byte(res.Stdout.String()), &appInfo)
		c.Assert(err, check.IsNil)

		c.Assert(len(appInfo.Autoscale), check.Equals, 1)
		c.Assert(appInfo.Autoscale[0].Process, check.Equals, autoscaleSpec.Process)
		c.Assert(appInfo.Autoscale[0].MinUnits, check.Equals, autoscaleSpec.MinUnits)
		c.Assert(appInfo.Autoscale[0].MaxUnits, check.Equals, autoscaleSpec.MaxUnits)
		c.Assert(appInfo.Autoscale[0].Version, check.Equals, autoscaleSpec.Version)

		// Step 5: Swap Autoscale
		autoscaleSpec.Version = 2
		res = T("unit", "autoscale", "swap", "-a", appName, "--version", "2").Run(env)
		c.Assert(res, ResultOk)

		//debug
		res, ok = K("get", "hpa", "-l", "app="+appName+"-web", "-o", "jsonpath='{.items[?(@.metadata.labels)].metadata.labels}'").Retry(time.Minute, env, retryOpt("2"))
		c.Assert(res, ResultOk)
		c.Assert(ok, check.Equals, true)

		// Step 6: Check Autoscale
		res = T("app", "info", "-a", appName, "--json").Run(env)
		c.Assert(res, ResultOk)
		err = json.Unmarshal([]byte(res.Stdout.String()), &appInfo)
		c.Assert(err, check.IsNil)

		c.Assert(len(appInfo.Autoscale), check.Equals, 1)
		c.Assert(appInfo.Autoscale[0].Process, check.Equals, autoscaleSpec.Process)
		c.Assert(appInfo.Autoscale[0].MinUnits, check.Equals, autoscaleSpec.MinUnits)
		c.Assert(appInfo.Autoscale[0].MaxUnits, check.Equals, autoscaleSpec.MaxUnits)
		c.Assert(appInfo.Autoscale[0].Version, check.Equals, autoscaleSpec.Version)
	}
	flow.backward = func(c *check.C, env *Environment) {
		appName := slugifyName(fmt.Sprintf("mv-swap-autoscale-%s", env.Get("pool")))
		res := T("app", "remove", "-y", "-a", appName).Run(env)
		c.Check(res, ResultOk)
	}

	return flow
}
