// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"context"

	"github.com/tsuru/tsuru/servicemanager"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	jobTypes "github.com/tsuru/tsuru/types/job"
	"gopkg.in/check.v1"
)

func (s *S) TestGetByName(c *check.C) {
	newJob := jobTypes.Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
	}
	err := servicemanager.Job.CreateJob(context.TODO(), &newJob, s.user, false)
	c.Assert(err, check.IsNil)
	myJob, err := servicemanager.Job.GetByName(context.TODO(), newJob.Name)
	c.Assert(err, check.IsNil)
	c.Assert(newJob.Name, check.DeepEquals, myJob.Name)
}

func (s *S) TestCreateCronjob(c *check.C) {
	newCron := jobTypes.Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello!"},
			},
		},
	}
	err := servicemanager.Job.CreateJob(context.TODO(), &newCron, s.user, false)
	c.Assert(err, check.IsNil)
	myJob, err := servicemanager.Job.GetByName(context.TODO(), newCron.Name)
	c.Assert(err, check.IsNil)
	c.Assert(newCron.Name, check.DeepEquals, myJob.Name)
	c.Assert(s.provisioner.ProvisionedJob(&newCron), check.Equals, true)
}

func (s *S) TestGetJobByNameNotFound(c *check.C) {
	job, err := servicemanager.Job.GetByName(context.TODO(), "404")
	c.Assert(err, check.Equals, jobTypes.ErrJobNotFound)
	c.Assert(job, check.IsNil)
}

func (s *S) TestDeleteJobFromProvisioner(c *check.C) {
	newJob := jobTypes.Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello!"},
			},
		},
	}
	err := servicemanager.Job.CreateJob(context.TODO(), &newJob, s.user, false)
	c.Assert(err, check.IsNil)
	job, err := servicemanager.Job.GetByName(context.TODO(), newJob.Name)
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.ProvisionedJob(job), check.Equals, true)
	err = servicemanager.Job.DeleteFromProvisioner(context.TODO(), job)
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.ProvisionedJob(job), check.Equals, false)
}

func (s *S) TestDeleteJobFromDB(c *check.C) {
	newJob := jobTypes.Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello!"},
			},
		},
	}
	err := servicemanager.Job.CreateJob(context.TODO(), &newJob, s.user, false)
	c.Assert(err, check.IsNil)
	job, err := servicemanager.Job.GetByName(context.TODO(), newJob.Name)
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.ProvisionedJob(job), check.Equals, true)
	err = servicemanager.Job.RemoveJobFromDb(job.Name)
	c.Assert(err, check.IsNil)
	_, err = servicemanager.Job.GetByName(context.TODO(), job.Name)
	c.Assert(err, check.Equals, jobTypes.ErrJobNotFound)
}

func (s *S) TestJobUnits(c *check.C) {
	newJob := jobTypes.Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello!"},
			},
		},
	}
	_, err := s.provisioner.NewJobWithUnits(context.TODO(), &newJob)
	c.Assert(err, check.IsNil)
	units, err := Units(context.TODO(), &newJob)
	c.Assert(err, check.IsNil)
	c.Assert(len(units), check.Equals, 2)
}

func (s *S) TestUpdateJob(c *check.C) {
	j1 := jobTypes.Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello!"},
			},
		},
	}
	j2 := jobTypes.Job{
		Name: "some-job",
		Spec: jobTypes.JobSpec{
			Schedule: "* */2 * * *",
			Container: jobTypes.ContainerInfo{
				Command: []string{"echo", "hello world!"},
			},
		},
	}
	err := servicemanager.Job.CreateJob(context.TODO(), &j1, s.user, false)
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.UpdateJob(context.TODO(), &j2, &j1, s.user)
	c.Assert(err, check.IsNil)
	updatedJob, err := servicemanager.Job.GetByName(context.TODO(), j2.Name)
	c.Assert(err, check.IsNil)
	c.Assert(updatedJob.TeamOwner, check.Equals, j1.TeamOwner)
	c.Assert(updatedJob.Pool, check.Equals, j1.Pool)
	c.Assert(updatedJob.Teams, check.DeepEquals, j1.Teams)
	c.Assert(updatedJob.Spec.Container, check.DeepEquals, j2.Spec.Container)
	c.Assert(updatedJob.Spec.Schedule, check.DeepEquals, j2.Spec.Schedule)
}

func (s *S) TestTriggerJobShouldProvisionNewJob(c *check.C) {
	j1 := jobTypes.Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: jobTypes.JobSpec{
			Container: jobTypes.ContainerInfo{
				Command: []string{"echo", "hello world!"},
			},
		},
	}
	err := servicemanager.Job.CreateJob(context.TODO(), &j1, s.user, false)
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.ProvisionedJob(&j1), check.Equals, false)
	err = servicemanager.Job.Trigger(context.TODO(), &j1)
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.ProvisionedJob(&j1), check.Equals, true)
}

func (s *S) TestList(c *check.C) {
	j1 := jobTypes.Job{
		Name:      "j1",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: jobTypes.JobSpec{
			Container: jobTypes.ContainerInfo{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello!"},
			},
		},
	}
	j2 := jobTypes.Job{
		Name:      "j2",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: jobTypes.JobSpec{
			Schedule: "* */2 * * *",
			Container: jobTypes.ContainerInfo{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello world!"},
			},
		},
	}
	err := servicemanager.Job.CreateJob(context.TODO(), &j1, s.user, false)
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.CreateJob(context.TODO(), &j2, s.user, false)
	c.Assert(err, check.IsNil)
	jobs, err := servicemanager.Job.List(context.TODO(), &jobTypes.Filter{})
	c.Assert(err, check.IsNil)
	c.Assert(len(jobs), check.Equals, 2)
}

func (s *S) TestAddServiceEnvToJobs(c *check.C) {
	job1 := jobTypes.Job{
		Name:      "job1",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: jobTypes.JobSpec{
			Container: jobTypes.ContainerInfo{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello!"},
			},
		},
	}
	cronjob1 := jobTypes.Job{
		Name:      "cronjob1",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello!"},
			},
		},
	}

	err := servicemanager.Job.CreateJob(context.TODO(), &job1, s.user, false)
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.CreateJob(context.TODO(), &cronjob1, s.user, false)
	c.Assert(err, check.IsNil)

	serviceEnvsToAdd := []bindTypes.ServiceEnvVar{
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "srv1"},
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_PORT", Value: "3306"}, InstanceName: "myinstance", ServiceName: "srv1"},
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root"}, InstanceName: "myinstance", ServiceName: "srv1"},
	}
	err = servicemanager.Job.AddServiceEnv(context.TODO(), &job1, jobTypes.AddInstanceArgs{
		Envs: serviceEnvsToAdd,
	})
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.AddServiceEnv(context.TODO(), &cronjob1, jobTypes.AddInstanceArgs{
		Envs: serviceEnvsToAdd,
	})
	c.Assert(err, check.IsNil)

	createdJob1, err := servicemanager.Job.GetByName(context.TODO(), job1.Name)
	c.Assert(err, check.IsNil)
	createdCronJob1, err := servicemanager.Job.GetByName(context.TODO(), cronjob1.Name)
	c.Assert(err, check.IsNil)
	c.Assert(createdJob1.Spec.ServiceEnvs, check.DeepEquals, serviceEnvsToAdd)
	c.Assert(createdCronJob1.Spec.ServiceEnvs, check.DeepEquals, serviceEnvsToAdd)
}

func (s *S) TestAddMultipleServiceInstancesEnvsToJob(c *check.C) {
	job1 := jobTypes.Job{
		Name:      "job1",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: jobTypes.JobSpec{
			Container: jobTypes.ContainerInfo{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello!"},
			},
		},
	}

	err := servicemanager.Job.CreateJob(context.TODO(), &job1, s.user, false)
	c.Assert(err, check.IsNil)

	err = servicemanager.Job.AddServiceEnv(context.TODO(), &job1, jobTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost1"}, InstanceName: "instance1", ServiceName: "mysql"},
		},
	})
	c.Assert(err, check.IsNil)

	err = servicemanager.Job.AddServiceEnv(context.TODO(), &job1, jobTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost2"}, InstanceName: "instance2", ServiceName: "mysql"},
		},
	})
	c.Assert(err, check.IsNil)

	err = servicemanager.Job.AddServiceEnv(context.TODO(), &job1, jobTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost3"}, InstanceName: "instance3", ServiceName: "mongodb"},
		},
	})
	c.Assert(err, check.IsNil)

	createdJob, err := servicemanager.Job.GetByName(context.TODO(), job1.Name)
	c.Assert(err, check.IsNil)

	c.Assert(createdJob.Spec.ServiceEnvs, check.DeepEquals, []bindTypes.ServiceEnvVar{
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost1"}, InstanceName: "instance1", ServiceName: "mysql"},
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost2"}, InstanceName: "instance2", ServiceName: "mysql"},
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost3"}, InstanceName: "instance3", ServiceName: "mongodb"},
	})
}

func (s *S) TestRemoveServiceInstanceEnvsFromJobs(c *check.C) {
	job1 := jobTypes.Job{
		Name:      "job1",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: jobTypes.JobSpec{
			Container: jobTypes.ContainerInfo{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello!"},
			},
			ServiceEnvs: []bindTypes.ServiceEnvVar{
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "srv1"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_PORT", Value: "3306"}, InstanceName: "myinstance", ServiceName: "srv1"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root"}, InstanceName: "myinstance", ServiceName: "srv1"},
			},
		},
	}
	cronjob1 := jobTypes.Job{
		Name:      "cronjob1",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello!"},
			},
			ServiceEnvs: []bindTypes.ServiceEnvVar{
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "srv1"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_PORT", Value: "3306"}, InstanceName: "myinstance", ServiceName: "srv1"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root"}, InstanceName: "myinstance", ServiceName: "srv1"},
			},
		},
	}

	err := servicemanager.Job.CreateJob(context.TODO(), &job1, s.user, false)
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.CreateJob(context.TODO(), &cronjob1, s.user, false)
	c.Assert(err, check.IsNil)

	err = servicemanager.Job.RemoveServiceEnv(context.TODO(), &job1, jobTypes.RemoveInstanceArgs{
		ServiceName:  "srv1",
		InstanceName: "myinstance",
	})
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.RemoveServiceEnv(context.TODO(), &cronjob1, jobTypes.RemoveInstanceArgs{
		ServiceName:  "srv1",
		InstanceName: "myinstance",
	})
	c.Assert(err, check.IsNil)

	createdJob1, err := servicemanager.Job.GetByName(context.TODO(), job1.Name)
	c.Assert(err, check.IsNil)
	createdCronJob1, err := servicemanager.Job.GetByName(context.TODO(), cronjob1.Name)
	c.Assert(err, check.IsNil)
	c.Assert(createdJob1.Spec.ServiceEnvs, check.DeepEquals, []bindTypes.ServiceEnvVar{})
	c.Assert(createdCronJob1.Spec.ServiceEnvs, check.DeepEquals, []bindTypes.ServiceEnvVar{})
}

func (s *S) TestRemoveServiceInstanceEnvsNotFound(c *check.C) {
	job1 := jobTypes.Job{
		Name:      "job1",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: jobTypes.JobSpec{
			Container: jobTypes.ContainerInfo{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello!"},
			},
			ServiceEnvs: []bindTypes.ServiceEnvVar{
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "srv1"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_PORT", Value: "3306"}, InstanceName: "myinstance", ServiceName: "srv1"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root"}, InstanceName: "myinstance", ServiceName: "srv1"},
			},
		},
	}

	err := servicemanager.Job.CreateJob(context.TODO(), &job1, s.user, false)
	c.Assert(err, check.IsNil)

	err = servicemanager.Job.RemoveServiceEnv(context.TODO(), &job1, jobTypes.RemoveInstanceArgs{
		ServiceName:  "srv1",
		InstanceName: "mynonexistentinstance",
	})
	c.Assert(err, check.IsNil)

	createdJob1, err := servicemanager.Job.GetByName(context.TODO(), job1.Name)
	c.Assert(err, check.IsNil)
	c.Assert(createdJob1.Spec.ServiceEnvs, check.DeepEquals, []bindTypes.ServiceEnvVar{
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "srv1"},
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_PORT", Value: "3306"}, InstanceName: "myinstance", ServiceName: "srv1"},
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root"}, InstanceName: "myinstance", ServiceName: "srv1"},
	})
}

func (s *S) TestRemoveServiceEnvsNotFound(c *check.C) {
	job1 := jobTypes.Job{
		Name:      "job1",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: jobTypes.JobSpec{
			Container: jobTypes.ContainerInfo{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello!"},
			},
			ServiceEnvs: []bindTypes.ServiceEnvVar{
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "srv1"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_PORT", Value: "3306"}, InstanceName: "myinstance", ServiceName: "srv1"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root"}, InstanceName: "myinstance", ServiceName: "srv1"},
			},
		},
	}

	err := servicemanager.Job.CreateJob(context.TODO(), &job1, s.user, false)
	c.Assert(err, check.IsNil)

	err = servicemanager.Job.RemoveServiceEnv(context.TODO(), &job1, jobTypes.RemoveInstanceArgs{
		ServiceName:  "srv2",
		InstanceName: "myinstance",
	})
	c.Assert(err, check.IsNil)

	createdJob1, err := servicemanager.Job.GetByName(context.TODO(), job1.Name)
	c.Assert(err, check.IsNil)
	c.Assert(createdJob1.Spec.ServiceEnvs, check.DeepEquals, []bindTypes.ServiceEnvVar{
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "srv1"},
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_PORT", Value: "3306"}, InstanceName: "myinstance", ServiceName: "srv1"},
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root"}, InstanceName: "myinstance", ServiceName: "srv1"},
	})
}

func (s *S) TestRemoveInstanceMultipleServicesEnvs(c *check.C) {
	job1 := jobTypes.Job{
		Name:      "job1",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: jobTypes.JobSpec{
			Container: jobTypes.ContainerInfo{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello!"},
			},
			ServiceEnvs: []bindTypes.ServiceEnvVar{
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance1", ServiceName: "myservice"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root"}, InstanceName: "myinstance1", ServiceName: "myservice"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "remotehost"}, InstanceName: "myinstance2", ServiceName: "myservice"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "myhost"}, InstanceName: "ourinstance1", ServiceName: "ourservice"},
			},
		},
	}

	err := servicemanager.Job.CreateJob(context.TODO(), &job1, s.user, false)
	c.Assert(err, check.IsNil)

	err = servicemanager.Job.RemoveServiceEnv(context.TODO(), &job1, jobTypes.RemoveInstanceArgs{
		ServiceName:  "myservice",
		InstanceName: "myinstance2",
	})
	c.Assert(err, check.IsNil)

	createdJob1, err := servicemanager.Job.GetByName(context.TODO(), job1.Name)
	c.Assert(err, check.IsNil)
	c.Assert(createdJob1.Spec.ServiceEnvs, check.DeepEquals, []bindTypes.ServiceEnvVar{
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance1", ServiceName: "myservice"},
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root"}, InstanceName: "myinstance1", ServiceName: "myservice"},
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "myhost"}, InstanceName: "ourinstance1", ServiceName: "ourservice"},
	})

	err = servicemanager.Job.RemoveServiceEnv(context.TODO(), &job1, jobTypes.RemoveInstanceArgs{
		ServiceName:  "myservice",
		InstanceName: "myinstance1",
	})
	c.Assert(err, check.IsNil)

	createdJob1, err = servicemanager.Job.GetByName(context.TODO(), job1.Name)
	c.Assert(err, check.IsNil)
	c.Assert(createdJob1.Spec.ServiceEnvs, check.DeepEquals, []bindTypes.ServiceEnvVar{
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "myhost"}, InstanceName: "ourinstance1", ServiceName: "ourservice"},
	})

	err = servicemanager.Job.RemoveServiceEnv(context.TODO(), &job1, jobTypes.RemoveInstanceArgs{
		ServiceName:  "ourservice",
		InstanceName: "ourinstance1",
	})
	c.Assert(err, check.IsNil)

	createdJob1, err = servicemanager.Job.GetByName(context.TODO(), job1.Name)
	c.Assert(err, check.IsNil)
	c.Assert(createdJob1.Spec.ServiceEnvs, check.DeepEquals, []bindTypes.ServiceEnvVar{})
}
