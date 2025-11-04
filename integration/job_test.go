// Copyright 2024 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	check "gopkg.in/check.v1"

	jobTypes "github.com/tsuru/tsuru/types/job"
)

var jobFlows = []ExecFlow{
	platformsToInstall(),
	loginTest(),
	quotaTest(),
	teamTest(),
	poolAdd(),
	jobCreate(),
	jobTrigger(),
	jobLogs(),
	jobUpdate(),
	jobEnvSet(),
	jobList(),
}

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
		fmt.Println("jobName:", jobName)

		// Create a manual job (not scheduled to avoid automatic execution) with a simple docker image
		// Using a custom image that logs and sleeps for testing
		res := T("job", "create", jobName,
			"-t", "{{.team}}",
			"-o", "{{.pool}}",
			"--manual",
			// "bash:5.2",
			// `"/bin/bash -c echo 'Job started' && echo 'Waiting 5 seconds...' && sleep 5 && echo 'DONE'"`,
		).Run(env)
		c.Assert(res, ResultOk, check.Commentf("Failed to create job: %v", res))

		// Verify job was created
		res = T("job", "info", jobName, "--json").Run(env)
		c.Assert(res, ResultOk, check.Commentf("Failed to get job info: %v", res))

		jobInfo := new(jobTypes.JobInfo)
		fmt.Println(jobInfo, res.Stdout.String())

		err := json.NewDecoder(&res.Stdout).Decode(jobInfo)
		fmt.Println(jobInfo)
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

func jobTrigger() ExecFlow {
	flow := ExecFlow{
		requires: []string{"jobnames"},
		matrix: map[string]string{
			"job": "jobnames",
		},
		parallel: true,
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
		parallel: true,
	}
	flow.forward = func(c *check.C, env *Environment) {
		jobName := env.Get("job")

		// Wait for job to complete and check logs
		var logs string
		ok := retry(3*time.Minute, func() bool {
			res := T("job", "log", jobName, "--lines", "100").Run(env)
			if res.ExitCode != 0 {
				return false
			}
			logs = res.Stdout.String()
			return strings.Contains(logs, "DONE")
		})
		c.Assert(ok, check.Equals, true, check.Commentf("Job did not complete successfully. Logs: %s", logs))
		c.Assert(logs, check.Matches, "(?s).*Job started.*")
		c.Assert(logs, check.Matches, "(?s).*Waiting 5 seconds.*")
		c.Assert(logs, check.Matches, "(?s).*DONE.*")
	}
	return flow
}

func jobUpdate() ExecFlow {
	flow := ExecFlow{
		requires: []string{"jobnames"},
	}
	flow.forward = func(c *check.C, env *Environment) {
		jobNames := env.All("jobnames")
		if len(jobNames) == 0 {
			return
		}
		jobName := jobNames[0]

		// Update job description
		res := T("job", "update", jobName, "-d", "Updated job description").Run(env)
		c.Assert(res, ResultOk, check.Commentf("Failed to update job: %v", res))

		// Verify update
		res = T("job", "info", jobName, "--json").Run(env)
		c.Assert(res, ResultOk)

		jobInfo := new(jobTypes.Job)
		err := json.NewDecoder(&res.Stdout).Decode(jobInfo)
		c.Assert(err, check.IsNil)
		c.Assert(jobInfo.Description, check.Equals, "Updated job description")
	}
	return flow
}

func jobEnvSet() ExecFlow {
	flow := ExecFlow{
		requires: []string{"jobnames"},
	}
	flow.forward = func(c *check.C, env *Environment) {
		jobNames := env.All("jobnames")
		if len(jobNames) == 0 {
			return
		}
		jobName := jobNames[0]

		// Set environment variable
		res := T("env", "set", "-j", jobName, "TEST_ENV=integration_test").Run(env)
		c.Assert(res, ResultOk, check.Commentf("Failed to set env: %v", res))

		// Verify environment variable was set
		res = T("env", "get", "-j", jobName).Run(env)
		c.Assert(res, ResultOk)
		c.Assert(res.Stdout.String(), check.Matches, "(?s).*TEST_ENV.*")

		// Unset environment variable
		res = T("env", "unset", "-j", jobName, "TEST_ENV").Run(env)
		c.Assert(res, ResultOk, check.Commentf("Failed to unset env: %v", res))
	}
	flow.backward = func(c *check.C, env *Environment) {
		jobNames := env.All("jobnames")
		if len(jobNames) == 0 {
			return
		}
		jobName := jobNames[0]

		// Cleanup: try to unset env if it still exists
		T("env", "unset", "-j", jobName, "TEST_ENV").Run(env)
	}
	return flow
}

func jobList() ExecFlow {
	flow := ExecFlow{
		requires: []string{"jobnames"},
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

func (s *S) TestJob(c *check.C) {
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

	for i := range jobFlows {
		f := &jobFlows[i]
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
