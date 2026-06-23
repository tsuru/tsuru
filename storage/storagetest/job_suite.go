// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"context"
	"sort"

	bindTypes "github.com/tsuru/tsuru/types/bind"
	jobTypes "github.com/tsuru/tsuru/types/job"
	check "gopkg.in/check.v1"
)

type JobSuite struct {
	SuiteHooks
	JobStorage jobTypes.JobStorage
}

func (s *JobSuite) newJob(name string) jobTypes.Job {
	return jobTypes.Job{
		Name:      name,
		TeamOwner: "team1",
		Owner:     "me@example.com",
		Pool:      "pool1",
		Tags:      []string{"tag1", "tag2"},
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
	}
}

func (s *JobSuite) TestInsertJob(c *check.C) {
	j := s.newJob("myjob")
	err := s.JobStorage.Insert(context.TODO(), j)
	c.Assert(err, check.IsNil)
	got, err := s.JobStorage.FindByName(context.TODO(), "myjob")
	c.Assert(err, check.IsNil)
	c.Assert(got.Name, check.Equals, "myjob")
	c.Assert(got.TeamOwner, check.Equals, "team1")
	c.Assert(got.Pool, check.Equals, "pool1")
	c.Assert(got.Tags, check.DeepEquals, []string{"tag1", "tag2"})
}

func (s *JobSuite) TestInsertDuplicateJob(c *check.C) {
	j := s.newJob("myjob")
	err := s.JobStorage.Insert(context.TODO(), j)
	c.Assert(err, check.IsNil)
	err = s.JobStorage.Insert(context.TODO(), j)
	c.Assert(err, check.Equals, jobTypes.ErrJobAlreadyExists)
}

func (s *JobSuite) TestFindByNameNotFound(c *check.C) {
	got, err := s.JobStorage.FindByName(context.TODO(), "missing")
	c.Assert(err, check.Equals, jobTypes.ErrJobNotFound)
	c.Assert(got, check.IsNil)
}

func (s *JobSuite) TestUpdateJob(c *check.C) {
	j := s.newJob("myjob")
	err := s.JobStorage.Insert(context.TODO(), j)
	c.Assert(err, check.IsNil)
	j.Description = "updated"
	j.Pool = "pool2"
	err = s.JobStorage.Update(context.TODO(), j)
	c.Assert(err, check.IsNil)
	got, err := s.JobStorage.FindByName(context.TODO(), "myjob")
	c.Assert(err, check.IsNil)
	c.Assert(got.Description, check.Equals, "updated")
	c.Assert(got.Pool, check.Equals, "pool2")
}

func (s *JobSuite) TestUpdateJobNotFound(c *check.C) {
	err := s.JobStorage.Update(context.TODO(), s.newJob("missing"))
	c.Assert(err, check.Equals, jobTypes.ErrJobNotFound)
}

func (s *JobSuite) TestDeleteJob(c *check.C) {
	j := s.newJob("myjob")
	err := s.JobStorage.Insert(context.TODO(), j)
	c.Assert(err, check.IsNil)
	err = s.JobStorage.Delete(context.TODO(), "myjob")
	c.Assert(err, check.IsNil)
	_, err = s.JobStorage.FindByName(context.TODO(), "myjob")
	c.Assert(err, check.Equals, jobTypes.ErrJobNotFound)
}

func (s *JobSuite) TestDeleteJobNotFound(c *check.C) {
	err := s.JobStorage.Delete(context.TODO(), "missing")
	c.Assert(err, check.Equals, jobTypes.ErrJobNotFound)
}

func (s *JobSuite) TestUpdateServiceEnvs(c *check.C) {
	j := s.newJob("myjob")
	err := s.JobStorage.Insert(context.TODO(), j)
	c.Assert(err, check.IsNil)
	envs := []bindTypes.ServiceEnvVar{{
		ServiceName:  "mysql",
		InstanceName: "db1",
		EnvVar:       bindTypes.EnvVar{Name: "DB_HOST", Value: "localhost"},
	}}
	err = s.JobStorage.UpdateServiceEnvs(context.TODO(), "myjob", envs)
	c.Assert(err, check.IsNil)
	got, err := s.JobStorage.FindByName(context.TODO(), "myjob")
	c.Assert(err, check.IsNil)
	c.Assert(got.Spec.ServiceEnvs, check.DeepEquals, envs)
}

func (s *JobSuite) TestListAll(c *check.C) {
	err := s.JobStorage.Insert(context.TODO(), s.newJob("job-a"))
	c.Assert(err, check.IsNil)
	err = s.JobStorage.Insert(context.TODO(), s.newJob("job-b"))
	c.Assert(err, check.IsNil)
	jobs, err := s.JobStorage.ListByFilter(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(jobs, check.HasLen, 2)
	names := []string{jobs[0].Name, jobs[1].Name}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"job-a", "job-b"})
}

func (s *JobSuite) TestListFilterByPoolAndName(c *check.C) {
	ja := s.newJob("alpha")
	ja.Pool = "poolX"
	err := s.JobStorage.Insert(context.TODO(), ja)
	c.Assert(err, check.IsNil)
	jb := s.newJob("beta")
	jb.Pool = "poolY"
	err = s.JobStorage.Insert(context.TODO(), jb)
	c.Assert(err, check.IsNil)

	byPool, err := s.JobStorage.ListByFilter(context.TODO(), &jobTypes.Filter{Pool: "poolX"})
	c.Assert(err, check.IsNil)
	c.Assert(byPool, check.HasLen, 1)
	c.Assert(byPool[0].Name, check.Equals, "alpha")

	byName, err := s.JobStorage.ListByFilter(context.TODO(), &jobTypes.Filter{Name: "bet"})
	c.Assert(err, check.IsNil)
	c.Assert(byName, check.HasLen, 1)
	c.Assert(byName[0].Name, check.Equals, "beta")
}
