// Copyright 2024 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"time"

	check "gopkg.in/check.v1"

	jobTypes "github.com/tsuru/tsuru/types/job"
)

func jobCreate() ExecFlow {
	flow := ExecFlow{
		requires: []string{"team", "poolnames"},
		provides: []string{"jobnames"},
		matrix: map[string]string{
			"pool": "poolnames",
		},
		parallel: true,
	}
	flow.forward = func(c *check.C, env *Environment) {
		jobName := fmt.Sprintf("simple-job-%s-ijob", env.Get("pool"))

		// Create a manual job (not scheduled to avoid automatic execution) with a simple docker image
		// Using a custom image that logs and sleeps for testing
		res := T("job", "create", jobName,
			"-t", "{{.team}}",
			"-o", "{{.pool}}",
			"--manual",
		).Run(env)
		c.Assert(res, ResultOk, check.Commentf("Failed to create job: %v", res))

		// Verify job was created
		res = T("job", "info", jobName, "--json").Run(env)
		c.Assert(res, ResultOk, check.Commentf("Failed to get job info: %v", res))

		jobInfo := new(jobTypes.JobInfo)

		err := json.NewDecoder(&res.Stdout).Decode(jobInfo)
		c.Assert(err, check.IsNil)
		c.Assert(jobInfo.Job.Name, check.Equals, jobName)
		c.Assert(jobInfo.Job.Pool, check.Equals, env.Get("pool"))
		c.Assert(jobInfo.Job.TeamOwner, check.Equals, env.Get("team"))

		env.Add("jobnames", jobName)
	}
	flow.backward = func(c *check.C, env *Environment) {
		jobName := fmt.Sprintf("simple-job-%s-ijob", env.Get("pool"))
		res := T("job", "delete", jobName).Run(env)
		c.Check(res, ResultOk)
	}
	return flow
}

func jobDeploy() ExecFlow {
	flow := ExecFlow{
		requires: []string{"jobnames"},
		matrix: map[string]string{
			"job": "jobnames",
		},
		parallel: true,
	}
	flow.forward = func(c *check.C, env *Environment) {
		cwd, err := os.Getwd()
		c.Assert(err, check.IsNil)
		jobDir := path.Join(cwd, "fixtures", "waiting-job")
		res := T("job", "deploy", "-j", env.Get("job"), "--dockerfile", jobDir).Run(env)
		c.Assert(res, ResultOk, check.Commentf("Failed to deploy job: %v", res))
	}
	return flow
}

func jobTrigger() ExecFlow {
	flow := ExecFlow{
		requires: []string{"jobnames"},
		matrix: map[string]string{
			"job": "jobnames",
		},
		parallel: false,
	}
	flow.forward = func(c *check.C, env *Environment) {
		jobName := env.Get("job")

		// Trigger the job manually
		res := T("job", "trigger", jobName).Run(env)
		c.Assert(res, ResultOk, check.Commentf("Failed to trigger job: %v", res))

		// Wait for job to start and create units
		ok := retry(2*time.Minute, func() bool {
			res = T("job", "info", jobName, "--json").Run(env)
			if res.ExitCode != 0 {
				return false
			}
			jobInfo := new(jobTypes.JobInfo)
			err := json.NewDecoder(&res.Stdout).Decode(jobInfo)
			if err != nil {
				return false
			}
			return len(jobInfo.Units) > 0
		})
		c.Assert(ok, check.Equals, true, check.Commentf("Job did not start after 2 minutes"))
	}
	return flow
}

func jobLogs() ExecFlow {
	flow := ExecFlow{
		requires: []string{"jobnames"},
		matrix: map[string]string{
			"job": "jobnames",
		},
		parallel: false,
	}
	flow.forward = func(c *check.C, env *Environment) {
		jobName := env.Get("job")

		checkJobCompletedSuccessfully(c, jobName, env)
		res := T("job", "log", jobName).Run(env)
		c.Assert(res, ResultOk, check.Commentf("Failed to get job logs: %v", res))
		logs := res.Stdout.String()
		c.Assert(logs, check.Matches, "(?s).*Job Started.*")
		c.Assert(logs, check.Matches, "(?s).*Waiting 5 Seconds.*")
		c.Assert(logs, check.Matches, "(?s).*DONE.*")
	}
	return flow
}

func jobUpdate() ExecFlow {
	flow := ExecFlow{
		requires: []string{"jobnames"},
		matrix: map[string]string{
			"job": "jobnames",
		},
		parallel: false,
	}
	flow.forward = func(c *check.C, env *Environment) {
		jobName := env.Get("jobnames")
		// Update job description
		res := T("job", "update", jobName, "-d", `"Updated job description"`).Run(env)
		c.Assert(res, ResultOk, check.Commentf("Failed to update job: %v", res))

		// Verify update
		res = T("job", "info", jobName, "--json").Run(env)
		c.Assert(res, ResultOk)

		jobInfo := new(jobTypes.JobInfo)
		err := json.NewDecoder(&res.Stdout).Decode(jobInfo)
		c.Assert(err, check.IsNil)
		c.Assert(jobInfo.Job.Description, check.Equals, "Updated job description")
	}
	return flow
}

func jobEnvSet() ExecFlow {
	flow := ExecFlow{
		requires: []string{"jobnames"},
		matrix: map[string]string{
			"job": "jobnames",
		},
		parallel: false,
	}
	envName := "TEST_ENV"
	envValue := "integration_test"
	flow.forward = func(c *check.C, env *Environment) {
		jobName := env.Get("jobnames")
		// Set environment variable
		res := T("env", "set", "-j", jobName, fmt.Sprintf("%s=%s", envName, envValue)).Run(env)
		c.Assert(res, ResultOk, check.Commentf("Failed to set env: %v", res))

		// Verify environment variable was set
		res = T("env", "get", "-j", jobName).Run(env)
		c.Assert(res, ResultOk)
		c.Assert(res.Stdout.String(), check.Matches, fmt.Sprintf("(?s).*%s.*", envName))

		var logs string
		ok := retry(3*time.Minute, func() bool {
			res = T("job", "trigger", jobName).Run(env)
			if res.ExitCode != 0 {
				return false
			}
			logs = res.Stdout.String()
			return true
		})
		c.Assert(ok, check.Equals, true, check.Commentf("Job did not trigger successfully. Logs: %s", logs))

		// Wait for job to complete and check logs
		checkJobCompletedSuccessfully(c, jobName, env)
		res = T("job", "log", jobName).Run(env)
		c.Assert(res, ResultOk, check.Commentf("Failed to get job logs: %v", res))
		logs = res.Stdout.String()
		c.Assert(logs, check.Matches, "(?s).*Job Started.*")
		c.Assert(logs, check.Matches, "(?s).*Waiting 5 Seconds.*")
		c.Assert(logs, check.Matches, fmt.Sprintf("(?s).*Environment variable %s is set to %s.*", envName, envValue))
		c.Assert(logs, check.Matches, "(?s).*DONE.*")
	}
	flow.backward = func(c *check.C, env *Environment) {
		jobName := env.Get("jobnames")
		// Cleanup: try to unset env if it still exists
		T("env", "unset", "-j", jobName, envName).Run(env)
	}
	return flow
}

func jobList() ExecFlow {
	flow := ExecFlow{
		requires: []string{"jobnames"},
		matrix: map[string]string{
			"job": "jobnames",
		},
		parallel: false,
	}
	flow.forward = func(c *check.C, env *Environment) {
		// List all jobs
		res := T("job", "list").Run(env)
		c.Assert(res, ResultOk)

		// Verify our jobs are in the list
		jobNames := env.All("jobnames")
		for _, jobName := range jobNames {
			c.Assert(res.Stdout.String(), check.Matches, "(?s).*"+jobName+".*")
		}

		// Test filtering by team
		res = T("job", "list", "-t", "{{.team}}").Run(env)
		c.Assert(res, ResultOk)
		for _, jobName := range jobNames {
			c.Assert(res.Stdout.String(), check.Matches, "(?s).*"+jobName+".*")
		}

		// Test filtering by pool
		if len(jobNames) > 0 {
			poolNames := env.All("poolnames")
			if len(poolNames) > 0 {
				res = T("job", "list", "-o", poolNames[0]).Run(env)
				c.Assert(res, ResultOk)
			}
		}
	}
	return flow
}

func checkJobCompletedSuccessfully(c *check.C, jobName string, env *Environment) {
	jobInfo := new(jobTypes.JobInfo)
	ok := retry(3*time.Minute, func() bool {
		res := T("job", "info", jobName, "--json").Run(env)
		c.Assert(res, ResultOk)
		err := json.NewDecoder(&res.Stdout).Decode(jobInfo)
		c.Assert(err, check.IsNil)
		for _, unit := range jobInfo.Units {
			if unit.Status != "succeeded" {
				fmt.Printf("Unit %s status is not succeeded: %s\n", unit.ID, unit.Status)
				return false
			}
		}
		return true
	})
	c.Assert(ok, check.Equals, true, check.Commentf("Job %s did not complete successfully", jobName))
}
