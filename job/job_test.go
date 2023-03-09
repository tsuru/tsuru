// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"context"

	jobTypes "github.com/tsuru/tsuru/types/job"
	"gopkg.in/check.v1"
)

func (s *S) TestGetByName(c *check.C) {
	newJob := Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
	}
	err := CreateJob(context.TODO(), &newJob, s.user, false)
	c.Assert(err, check.IsNil)
	myJob, err := GetByName(context.TODO(), newJob.Name)
	c.Assert(err, check.IsNil)
	c.Assert(newJob.Name, check.DeepEquals, myJob.Name)
}

func (s *S) TestCreateCronjob(c *check.C) {
	newCron := Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				Name:    "augustine",
				Image:   "alpine:latest",
				Command: []string{"echo", "hello!"},
			},
		},
	}
	err := CreateJob(context.TODO(), &newCron, s.user, false)
	c.Assert(err, check.IsNil)
	myJob, err := GetByName(context.TODO(), newCron.Name)
	c.Assert(err, check.IsNil)
	c.Assert(newCron.Name, check.DeepEquals, myJob.Name)
	c.Assert(s.provisioner.ProvisionedJob(&newCron), check.Equals, true)
}

func (s *S) TestGetJobByNameNotFound(c *check.C) {
	job, err := GetByName(context.TODO(), "404")
	c.Assert(err, check.Equals, jobTypes.ErrJobNotFound)
	c.Assert(job, check.IsNil)
}

func (s *S) TestDeleteJobFromProvisioner(c *check.C) {
	newJob := Job{
			Name:      "some-job",
			TeamOwner: s.team.Name,
			Pool:      s.Pool,
			Teams:     []string{s.team.Name},
			Spec: JobSpec{
				Schedule: "* * * * *",
				Container: jobTypes.ContainerInfo{
					Name:    "augustine",
					Image:   "alpine:latest",
					Command: []string{"echo", "hello!"},
				},
			},
	}
	err := CreateJob(context.TODO(), &newJob, s.user, false)
	c.Assert(err, check.IsNil)
	job, err := GetByName(context.TODO(), newJob.Name)
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.ProvisionedJob(job), check.Equals, true)
	err = DeleteFromProvisioner(context.TODO(), job)
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.ProvisionedJob(job), check.Equals, false)
}

func (s *S) TestDeleteJobFromDB(c *check.C) {
	newJob := Job{
			Name:      "some-job",
			TeamOwner: s.team.Name,
			Pool:      s.Pool,
			Teams:     []string{s.team.Name},
			Spec: JobSpec{
				Schedule: "* * * * *",
				Container: jobTypes.ContainerInfo{
					Name:    "augustine",
					Image:   "alpine:latest",
					Command: []string{"echo", "hello!"},
				},
			},
	}
	err := CreateJob(context.TODO(), &newJob, s.user, false)
	c.Assert(err, check.IsNil)
	job, err := GetByName(context.TODO(), newJob.Name)
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.ProvisionedJob(job), check.Equals, true)
	err = RemoveJobFromDb(job.Name)
	c.Assert(err, check.IsNil)
	_, err = GetByName(context.TODO(), job.Name)
	c.Assert(err, check.Equals, jobTypes.ErrJobNotFound)
}

func (s *S) TestJobUnits(c *check.C) {
	newJob := Job{
			Name:      "some-job",
			TeamOwner: s.team.Name,
			Pool:      s.Pool,
			Teams:     []string{s.team.Name},
			Spec: JobSpec{
				Schedule: "* * * * *",
				Container: jobTypes.ContainerInfo{
					Name:    "augustine",
					Image:   "alpine:latest",
					Command: []string{"echo", "hello!"},
				},
			},
	}
	_, err := s.provisioner.NewJobWithUnits(context.TODO(), &newJob)
	c.Assert(err, check.IsNil)
	units, err := newJob.Units(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(len(units), check.Equals, 2)
}

func (s *S) TestUpdateJob(c *check.C) {
	j1 := Job{
			Name:      "some-job",
			TeamOwner: s.team.Name,
			Pool:      s.Pool,
			Teams:     []string{s.team.Name},
			Spec: JobSpec{
				Schedule: "* * * * *",
				Container: jobTypes.ContainerInfo{
					Name:    "augustine",
					Image:   "alpine:latest",
					Command: []string{"echo", "hello!"},
				},
			},
	}
	j2 := Job{
			Name: "some-job",
			Spec: JobSpec{
				Schedule: "* */2 * * *",
				Container: jobTypes.ContainerInfo{
					Name:    "betty",
					Command: []string{"echo", "hello world!"},
				},
			},
	}
	err := CreateJob(context.TODO(), &j1, s.user, false)
	c.Assert(err, check.IsNil)
	err = UpdateJob(context.TODO(), &j2, &j1, s.user)
	c.Assert(err, check.IsNil)
	updatedJob, err := GetByName(context.TODO(), j2.Name)
	c.Assert(err, check.IsNil)
	c.Assert(updatedJob.TeamOwner, check.Equals, j1.TeamOwner)
	c.Assert(updatedJob.Pool, check.Equals, j1.Pool)
	c.Assert(updatedJob.Teams, check.DeepEquals, j1.Teams)
	c.Assert(updatedJob.Spec.Container, check.DeepEquals, j2.Spec.Container)
	c.Assert(updatedJob.Spec.Schedule, check.DeepEquals, j2.Spec.Schedule)
}

func (s *S) TestTriggerJobShouldProvisionNewJob(c *check.C) {
	j1 := Job{
			Name:      "some-job",
			TeamOwner: s.team.Name,
			Pool:      s.Pool,
			Teams:     []string{s.team.Name},
			Spec: JobSpec{
				Container: jobTypes.ContainerInfo{
					Name:    "betty",
					Command: []string{"echo", "hello world!"},
				},
			},
	}
	err := CreateJob(context.TODO(), &j1, s.user, false)
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.ProvisionedJob(&j1), check.Equals, false)
	err = Trigger(context.TODO(), &j1)
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.ProvisionedJob(&j1), check.Equals, true)
}

func (s *S) TestList(c *check.C) {
	j1 := Job{
		Name:      "j1",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: JobSpec{
			Container: jobTypes.ContainerInfo{
				Name:    "augustine",
				Image:   "alpine:latest",
				Command: []string{"echo", "hello!"},
			},
		},
	}
	j2 := Job{
		Name:      "j2",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: JobSpec{
			Schedule: "* */2 * * *",
			Container: jobTypes.ContainerInfo{
				Name:    "betty",
				Image:   "alpine:latest",
				Command: []string{"echo", "hello world!"},
			},
		},
	}
	err := CreateJob(context.TODO(), &j1, s.user, false)
	c.Assert(err, check.IsNil)
	err = CreateJob(context.TODO(), &j2, s.user, false)
	c.Assert(err, check.IsNil)
	jobs, err := List(context.TODO(), &Filter{})
	c.Assert(err, check.IsNil)
	c.Assert(len(jobs), check.Equals, 2)
}
