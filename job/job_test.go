// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/builder"
	tsuruEnvs "github.com/tsuru/tsuru/envs"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/types/auth"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/types/bind"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	jobTypes "github.com/tsuru/tsuru/types/job"
	provisionTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/types/quota"
	"gopkg.in/check.v1"
)

func (s *S) TestCreateJobWithInvalidName(c *check.C) {
	newJob := jobTypes.Job{
		Name:      "123-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			ActiveDeadlineSeconds: func() *int64 {
				i := int64(300)
				return &i
			}(),
		},
		DeployOptions: &jobTypes.DeployOptions{
			Kind:  provisionTypes.DeployImage,
			Image: "alpine:latest",
		},
	}
	err := servicemanager.Job.CreateJob(context.TODO(), &newJob, s.user)
	c.Assert(err, check.NotNil)
	jobCreationErr := &jobTypes.JobCreationError{Job: newJob.Name}
	jobCreationErr.Err = jobTypes.ErrInvalidJobName
	c.Assert(err, check.DeepEquals, jobCreationErr)
}

func (s *S) TestGetByName(c *check.C) {
	newJob := jobTypes.Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			ActiveDeadlineSeconds: func() *int64 {
				i := int64(300)
				return &i
			}(),
		},
		DeployOptions: &jobTypes.DeployOptions{
			Kind:  provisionTypes.DeployImage,
			Image: "alpine:latest",
		},
	}
	err := servicemanager.Job.CreateJob(context.TODO(), &newJob, s.user)
	c.Assert(err, check.IsNil)
	myJob, err := servicemanager.Job.GetByName(context.TODO(), newJob.Name)
	c.Assert(err, check.IsNil)
	c.Assert(newJob.Name, check.DeepEquals, myJob.Name)
	c.Assert(*myJob.Spec.ActiveDeadlineSeconds, check.Equals, int64(300))
}

func (s *S) TestCreateCronjob(c *check.C) {
	newCron := jobTypes.Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
				Command:          []string{"echo", "hello!"},
			},
		},
	}
	err := servicemanager.Job.CreateJob(context.TODO(), &newCron, s.user)
	c.Assert(err, check.IsNil)
	myJob, err := servicemanager.Job.GetByName(context.TODO(), newCron.Name)
	c.Assert(err, check.IsNil)
	c.Assert(newCron.Name, check.DeepEquals, myJob.Name)
	c.Assert(s.provisioner.ProvisionedJob(newCron.Name), check.Equals, true)
	c.Assert(*myJob.Spec.ActiveDeadlineSeconds, check.Equals, int64(0))
}

func (s *S) TestCreateCronjobWithK8sBuilder(c *check.C) {
	s.builder.OnBuildJob = func(job *jobTypes.Job, opts builder.BuildOpts) (string, error) {
		return fmt.Sprintf("fake.registry.io/job-%s:latest", job.Name), nil
	}
	newCron := jobTypes.Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
				Command:          []string{"echo", "hello!"},
			},
		},
	}
	err := servicemanager.Job.CreateJob(context.TODO(), &newCron, s.user)
	c.Assert(err, check.IsNil)
	myJob, err := servicemanager.Job.GetByName(context.TODO(), newCron.Name)
	c.Assert(err, check.IsNil)
	c.Assert(newCron.Name, check.DeepEquals, myJob.Name)
	c.Assert(s.provisioner.ProvisionedJob(newCron.Name), check.Equals, true)

	c.Assert(myJob.Spec.Container.OriginalImageSrc, check.Equals, "alpine:latest")
	c.Assert(myJob.Spec.Container.InternalRegistryImage, check.Equals, "fake.registry.io/job-some-job:latest")
}

func (s *S) TestCreateManualJob(c *check.C) {
	newCron := jobTypes.Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Spec: jobTypes.JobSpec{
			Manual: true,
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
				Command:          []string{"echo", "hello!"},
			},
		},
	}
	err := servicemanager.Job.CreateJob(context.TODO(), &newCron, s.user)
	c.Assert(err, check.IsNil)
	myJob, err := servicemanager.Job.GetByName(context.TODO(), newCron.Name)
	c.Assert(err, check.IsNil)
	c.Assert(myJob.Spec.Manual, check.Equals, true)
	c.Assert(myJob.Spec.Schedule, check.DeepEquals, "* * 31 2 *")
	c.Assert(s.provisioner.ProvisionedJob(newCron.Name), check.Equals, true)
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
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
				Command:          []string{"echo", "hello!"},
			},
		},
	}
	err := servicemanager.Job.CreateJob(context.TODO(), &newJob, s.user)
	c.Assert(err, check.IsNil)
	job, err := servicemanager.Job.GetByName(context.TODO(), newJob.Name)
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.ProvisionedJob(job.Name), check.Equals, true)
	err = servicemanager.Job.RemoveJobProv(context.TODO(), job)
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.ProvisionedJob(job.Name), check.Equals, false)
}

func (s *S) TestDeleteJobFromDB(c *check.C) {
	newJob := jobTypes.Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
				Command:          []string{"echo", "hello!"},
			},
		},
	}
	err := servicemanager.Job.CreateJob(context.TODO(), &newJob, s.user)
	c.Assert(err, check.IsNil)
	job, err := servicemanager.Job.GetByName(context.TODO(), newJob.Name)
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.ProvisionedJob(job.Name), check.Equals, true)
	err = servicemanager.Job.RemoveJob(context.TODO(), job)
	c.Assert(err, check.IsNil)
	_, err = servicemanager.Job.GetByName(context.TODO(), job.Name)
	c.Assert(err, check.Equals, jobTypes.ErrJobNotFound)
}

func (s *S) TestIncreaseDecreaseQuotaForJob(c *check.C) {
	var userinUseNow *int
	var teaminUseNow *int
	userinUseNow = new(int)
	teaminUseNow = new(int)
	s.mockService.UserQuota.OnInc = func(item quota.QuotaItem, quantity int) error {
		*userinUseNow += quantity
		c.Assert(item.GetName(), check.Equals, s.user.Email)
		return nil
	}
	s.mockService.TeamQuota.OnInc = func(item *auth.Team, quantity int) error {
		*teaminUseNow += quantity
		c.Assert(item.Name, check.Equals, s.team.Name)
		return nil
	}
	newJob := jobTypes.Job{
		Name:      "some-job",
		Pool:      s.Pool,
		TeamOwner: s.team.Name,
		Owner:     s.user.Email,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
		},
		DeployOptions: &jobTypes.DeployOptions{
			Kind:  provisionTypes.DeployImage,
			Image: "alpine:latest",
		},
	}
	c.Assert(*teaminUseNow, check.Equals, 0)
	c.Assert(*userinUseNow, check.Equals, 0)
	err := servicemanager.Job.CreateJob(context.TODO(), &newJob, s.user)
	c.Assert(err, check.IsNil)
	c.Assert(*userinUseNow, check.Equals, 1)
	c.Assert(*teaminUseNow, check.Equals, 1)
	err = servicemanager.Job.RemoveJob(context.TODO(), &newJob)
	c.Assert(err, check.IsNil)
	c.Assert(*teaminUseNow, check.Equals, 0)
	c.Assert(*userinUseNow, check.Equals, 0)
}

func (s *S) TestJobUnits(c *check.C) {
	newJob := jobTypes.Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
				Command:          []string{"echo", "hello!"},
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
	var updateTests = []struct {
		name        string
		oldJob      jobTypes.Job
		newJob      jobTypes.Job
		expectedJob jobTypes.Job
		expectedErr error
		beforeFunc  func()
	}{
		{
			name: "update job with new schedule",
			oldJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc: "alpine:latest",
						Command:          []string{"echo", "hello!"},
					},
					ActiveDeadlineSeconds: func() *int64 { i := int64(300); return &i }(),
				},
				DeployOptions: &jobTypes.DeployOptions{
					Kind:  provisionTypes.DeployImage,
					Image: "alpine:latest",
				},
			},
			newJob: jobTypes.Job{
				Name: "some-job",
				Spec: jobTypes.JobSpec{
					Schedule: "0 0 * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc: "alpine:latest",
						Command:          []string{"echo", "hello!"},
					},
				},
			},
			expectedJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Plan:      *s.defaultPlan,
				Owner:     s.user.Email,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "0 0 * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc:      "alpine:latest",
						Command:               []string{"echo", "hello!"},
						InternalRegistryImage: "fake.registry.io/job-some-job:latest",
					},
					ActiveDeadlineSeconds: func() *int64 { i := int64(300); return &i }(),
					ServiceEnvs:           []bind.ServiceEnvVar{}, Envs: []bind.EnvVar{},
				},
				Metadata: app.Metadata{Labels: []app.MetadataItem{}, Annotations: []app.MetadataItem{}},
				DeployOptions: &jobTypes.DeployOptions{
					Kind:  provisionTypes.DeployImage,
					Image: "alpine:latest",
				},
			},
		},
		{
			name: "update job with new plan",
			oldJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc: "alpine:latest",
						Command:          []string{"echo", "hello!"},
					},
				},
				DeployOptions: &jobTypes.DeployOptions{
					Kind:  provisionTypes.DeployImage,
					Image: "alpine:latest",
				},
			},
			newJob: jobTypes.Job{
				Name: "some-job",
				Plan: app.Plan{Name: "c2m1"},
				Spec: jobTypes.JobSpec{
					Schedule: "0 0 * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc: "alpine:latest",
						Command:          []string{"echo", "hello!"},
					},
				},
			},
			expectedJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Plan:      app.Plan{Name: "c2m1", Memory: 1024, CPUMilli: 2000},
				Owner:     s.user.Email,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "0 0 * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc:      "alpine:latest",
						InternalRegistryImage: "fake.registry.io/job-some-job:latest",
						Command:               []string{"echo", "hello!"},
					},
					ServiceEnvs: []bind.ServiceEnvVar{}, Envs: []bind.EnvVar{},
					ActiveDeadlineSeconds: func() *int64 { i := int64(0); return &i }(),
				},
				Metadata: app.Metadata{Labels: []app.MetadataItem{}, Annotations: []app.MetadataItem{}},
				DeployOptions: &jobTypes.DeployOptions{
					Kind:  provisionTypes.DeployImage,
					Image: "alpine:latest",
				},
			},
		},
		{
			name: "change scheduled job to manual",
			oldJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc: "alpine:latest",
						Command:          []string{"echo", "hello!"},
					},
				},
			},
			newJob: jobTypes.Job{
				Name: "some-job",
				Spec: jobTypes.JobSpec{
					Manual: true,
				},
			},
			expectedJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Plan:      *s.defaultPlan,
				Owner:     s.user.Email,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * 31 2 *",
					Manual:   true,
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc:      "alpine:latest",
						InternalRegistryImage: "fake.registry.io/job-some-job:latest",
						Command:               []string{"echo", "hello!"},
					},
					ServiceEnvs: []bind.ServiceEnvVar{}, Envs: []bind.EnvVar{},
					ActiveDeadlineSeconds: func() *int64 { i := int64(0); return &i }(),
				},
				Metadata: app.Metadata{Labels: []app.MetadataItem{}, Annotations: []app.MetadataItem{}},
				DeployOptions: &jobTypes.DeployOptions{
					Kind:  provisionTypes.DeployImage,
					Image: "alpine:latest",
				},
			},
		},
		{
			name: "change manual job to scheduled",
			oldJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * 31 2 *",
					Manual:   true,
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc: "alpine:latest",
						Command:          []string{"echo", "hello!"},
					},
				},
				DeployOptions: &jobTypes.DeployOptions{
					Kind:  provisionTypes.DeployImage,
					Image: "alpine:latest",
				},
			},
			newJob: jobTypes.Job{
				Name: "some-job",
				Spec: jobTypes.JobSpec{
					Schedule: "*/5 * * * *",
				},
			},
			expectedJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Plan:      *s.defaultPlan,
				Owner:     s.user.Email,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "*/5 * * * *",
					Manual:   false,
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc:      "alpine:latest",
						InternalRegistryImage: "fake.registry.io/job-some-job:latest",
						Command:               []string{"echo", "hello!"},
					},
					ServiceEnvs: []bind.ServiceEnvVar{}, Envs: []bind.EnvVar{},
					ActiveDeadlineSeconds: func() *int64 { i := int64(0); return &i }(),
				},
				Metadata: app.Metadata{Labels: []app.MetadataItem{}, Annotations: []app.MetadataItem{}},
				DeployOptions: &jobTypes.DeployOptions{
					Kind:  provisionTypes.DeployImage,
					Image: "alpine:latest",
				},
			},
		},
		{
			name: "update job with new metadata",
			oldJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "*/5 * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc: "alpine:latest",
						Command:          []string{"echo", "hello!"},
					},
				},
				Metadata: app.Metadata{Labels: []app.MetadataItem{{Name: "foo", Value: "bar"}}},
				DeployOptions: &jobTypes.DeployOptions{
					Kind:  provisionTypes.DeployImage,
					Image: "alpine:latest",
				},
			},
			newJob: jobTypes.Job{
				Name:     "some-job",
				Metadata: app.Metadata{Labels: []app.MetadataItem{{Name: "xxx", Value: "yyy"}}},
			},
			expectedJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Plan:      *s.defaultPlan,
				Owner:     s.user.Email,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "*/5 * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc:      "alpine:latest",
						InternalRegistryImage: "fake.registry.io/job-some-job:latest",
						Command:               []string{"echo", "hello!"},
					},
					ServiceEnvs: []bind.ServiceEnvVar{}, Envs: []bind.EnvVar{},
					ActiveDeadlineSeconds: func() *int64 { i := int64(0); return &i }(),
				},
				Metadata: app.Metadata{Labels: []app.MetadataItem{{Name: "foo", Value: "bar"}, {Name: "xxx", Value: "yyy"}}, Annotations: []app.MetadataItem{}},
				DeployOptions: &jobTypes.DeployOptions{
					Kind:  provisionTypes.DeployImage,
					Image: "alpine:latest",
				},
			},
		},
		{
			name: "remove foo label metadata",
			oldJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc: "alpine:latest",
						Command:          []string{"echo", "hello!"},
					},
				},
				Metadata: app.Metadata{Labels: []app.MetadataItem{{Name: "foo", Value: "bar"}, {Name: "xxx", Value: "yyy"}}},
			},
			newJob: jobTypes.Job{
				Name:     "some-job",
				Metadata: app.Metadata{Labels: []app.MetadataItem{{Name: "foo", Delete: true}}},
			},
			expectedJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Plan:      *s.defaultPlan,
				Owner:     s.user.Email,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc:      "alpine:latest",
						InternalRegistryImage: "fake.registry.io/job-some-job:latest",
						Command:               []string{"echo", "hello!"},
					},
					ActiveDeadlineSeconds: func() *int64 { i := int64(0); return &i }(),
					ServiceEnvs:           []bind.ServiceEnvVar{}, Envs: []bind.EnvVar{},
				},
				Metadata: app.Metadata{Labels: []app.MetadataItem{{Name: "xxx", Value: "yyy"}}, Annotations: []app.MetadataItem{}},
				DeployOptions: &jobTypes.DeployOptions{
					Kind:  provisionTypes.DeployImage,
					Image: "alpine:latest",
				},
			},
		},
		{
			name: "update active deadline to 0 should set to default value as 0",
			oldJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc: "alpine:latest",
						Command:          []string{"echo", "hello!"},
					},
				},
			},
			newJob: jobTypes.Job{
				Name: "some-job",
				Spec: jobTypes.JobSpec{ActiveDeadlineSeconds: func() *int64 { i := int64(0); return &i }()},
			},
			expectedJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Plan:      *s.defaultPlan,
				Owner:     s.user.Email,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc:      "alpine:latest",
						InternalRegistryImage: "fake.registry.io/job-some-job:latest",
						Command:               []string{"echo", "hello!"},
					},
					ServiceEnvs: []bind.ServiceEnvVar{}, Envs: []bind.EnvVar{},
					ActiveDeadlineSeconds: func() *int64 { i := int64(0); return &i }(),
				},
				Metadata: app.Metadata{Labels: []app.MetadataItem{}, Annotations: []app.MetadataItem{}},
				DeployOptions: &jobTypes.DeployOptions{
					Kind:  provisionTypes.DeployImage,
					Image: "alpine:latest",
				},
			},
		},
		{
			name: "update to team owner with invalid team",
			oldJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc: "alpine:latest",
						Command:          []string{"echo", "hello!"},
					},
				},
			},
			newJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: "some-other-team",
			},
			expectedErr: &tsuruErrors.ValidationError{Message: "team not found"},
		},
		{
			name: "update job to unknown pool",
			oldJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc:      "alpine:latest",
						InternalRegistryImage: "fake.registry.io/job-some-job:latest",
						Command:               []string{"echo", "hello!"},
					},
				},
			},
			newJob: jobTypes.Job{
				Name: "some-job",
				Pool: "some-other-pool",
			},
			expectedErr: pool.ErrPoolNotFound,
		},
		{
			name: "update job to invalid pool",
			oldJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc: "alpine:latest",
						Command:          []string{"echo", "hello!"},
					},
				},
			},
			newJob: jobTypes.Job{
				Name: "some-job",
				Pool: "pool2",
			},
			expectedErr: &tsuruErrors.ValidationError{Message: "Job team owner \"tsuruteam\" has no access to pool \"pool2\""},
		},
		{
			name: "update job to invalid plan to pool",
			oldJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc: "alpine:latest",
						Command:          []string{"echo", "hello!"},
					},
				},
			},
			newJob: jobTypes.Job{
				Name: "some-job",
				Plan: *s.plan,
			},
			expectedErr: &tsuruErrors.ValidationError{Message: "Job plan \"c4m2\" is not allowed on pool \"pool1\""},
		},
		{
			name: "update job to invalid cronjob",
			oldJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc: "alpine:latest",
						Command:          []string{"echo", "hello!"},
					},
				},
			},
			newJob: jobTypes.Job{
				Name: "some-job",
				Spec: jobTypes.JobSpec{
					Schedule: "120 30 * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc: "alpine:latest",
						Command:          []string{"echo", "hello!"},
					},
				},
			},
			expectedErr: &tsuruErrors.ValidationError{Message: "invalid schedule"},
		},
		{
			name: "update job should use deploy agent with container info on both newJob and oldJob",
			oldJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc: "alpine:latest",
						Command:          []string{"echo", "hello!"},
					},
				},
			},
			newJob: jobTypes.Job{
				Name: "some-job",
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc: "alpine:v1",
						Command:          []string{"echo", "hello!"},
					},
				},
			},
			expectedJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Plan:      *s.defaultPlan,
				Owner:     s.user.Email,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc:      "alpine:v1",
						InternalRegistryImage: "fake.registry.io/job-some-job:latest",
						Command:               []string{"echo", "hello!"},
					},
					ServiceEnvs: []bind.ServiceEnvVar{}, Envs: []bind.EnvVar{},
					ActiveDeadlineSeconds: func() *int64 { i := int64(0); return &i }(),
				},
				Metadata: app.Metadata{Labels: []app.MetadataItem{}, Annotations: []app.MetadataItem{}},
				DeployOptions: &jobTypes.DeployOptions{
					Kind:  provisionTypes.DeployImage,
					Image: "alpine:v1",
				},
			},
			expectedErr: nil,
		},
		{
			name: "update job should use deploy agent with deploy options populated on newJob and legacy way on oldJob",
			oldJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					// legacy way
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc: "alpine:latest",
						Command:          []string{"echo", "hello!"},
					},
				},
			},
			newJob: jobTypes.Job{
				Name: "some-job",
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
				},
				DeployOptions: &jobTypes.DeployOptions{
					Kind:  provisionTypes.DeployImage,
					Image: "alpine:v1",
				},
			},
			expectedJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Plan:      *s.defaultPlan,
				Owner:     s.user.Email,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc:      "alpine:v1",
						InternalRegistryImage: "fake.registry.io/job-some-job:latest",
						Command:               []string{"echo", "hello!"},
					},
					ServiceEnvs: []bind.ServiceEnvVar{}, Envs: []bind.EnvVar{},
					ActiveDeadlineSeconds: func() *int64 { i := int64(0); return &i }(),
				},
				Metadata: app.Metadata{Labels: []app.MetadataItem{}, Annotations: []app.MetadataItem{}},
				DeployOptions: &jobTypes.DeployOptions{
					Kind:  provisionTypes.DeployImage,
					Image: "alpine:v1",
				},
			},
			expectedErr: nil,
		},
		{
			name: "update job should use deploy agent with deploy options populated on newJob and oldJob",
			oldJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
				},
				DeployOptions: &jobTypes.DeployOptions{
					Kind:  provisionTypes.DeployImage,
					Image: "alpine:latest",
				},
			},
			newJob: jobTypes.Job{
				Name: "some-job",
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
				},
				DeployOptions: &jobTypes.DeployOptions{
					Kind:  provisionTypes.DeployImage,
					Image: "alpine:v1",
				},
			},
			expectedJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Plan:      *s.defaultPlan,
				Owner:     s.user.Email,
				Pool:      s.Pool,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc:      "alpine:v1",
						InternalRegistryImage: "fake.registry.io/job-some-job:latest",
						Command:               []string{},
					},
					ServiceEnvs: []bind.ServiceEnvVar{}, Envs: []bind.EnvVar{},
					ActiveDeadlineSeconds: func() *int64 { i := int64(0); return &i }(),
				},
				Metadata: app.Metadata{Labels: []app.MetadataItem{}, Annotations: []app.MetadataItem{}},
				DeployOptions: &jobTypes.DeployOptions{
					Kind:  provisionTypes.DeployImage,
					Image: "alpine:v1",
				},
			},
			expectedErr: nil,
		},
		{
			name: "update concurrency policy",
			oldJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Pool:      s.Pool,
				Plan:      *s.defaultPlan,
				Owner:     s.user.Email,
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc: "alpine:v1",
						Command:          []string{"echo", "hello!"},
					},
				},
			},
			newJob: jobTypes.Job{
				Name: "some-job",
				Spec: jobTypes.JobSpec{
					ConcurrencyPolicy: func() *string { s := "Forbid"; return &s }(),
				},
			},
			expectedJob: jobTypes.Job{
				Name:      "some-job",
				TeamOwner: s.team.Name,
				Plan:      *s.defaultPlan,
				Owner:     s.user.Email,
				Pool:      s.Pool,
				Metadata:  app.Metadata{Labels: []app.MetadataItem{}, Annotations: []app.MetadataItem{}},
				DeployOptions: &jobTypes.DeployOptions{
					Kind:  provisionTypes.DeployImage,
					Image: "alpine:v1",
				},
				Spec: jobTypes.JobSpec{
					Schedule: "* * * * *",
					Container: jobTypes.ContainerInfo{
						OriginalImageSrc:      "alpine:v1",
						InternalRegistryImage: "fake.registry.io/job-some-job:latest",
						Command:               []string{"echo", "hello!"},
					},
					ConcurrencyPolicy: func() *string { s := "Forbid"; return &s }(),
					ServiceEnvs:       []bind.ServiceEnvVar{}, Envs: []bind.EnvVar{},
					ActiveDeadlineSeconds: func() *int64 { i := int64(0); return &i }(),
				},
			},
		},
	}
	for _, t := range updateTests {
		if t.beforeFunc != nil {
			t.beforeFunc()
		}
		c.Logf("test %q", t.name)
		err := servicemanager.Job.CreateJob(context.TODO(), &t.oldJob, s.user)
		c.Assert(err, check.IsNil)
		err = servicemanager.Job.UpdateJob(context.TODO(), &t.newJob, &t.oldJob, s.user)
		if t.expectedErr != nil {
			c.Assert(err, check.DeepEquals, t.expectedErr)
			servicemanager.Job.RemoveJob(context.TODO(), &t.newJob)
			continue
		}
		c.Assert(err, check.IsNil)
		updatedJob, err := servicemanager.Job.GetByName(context.TODO(), t.newJob.Name)
		c.Assert(err, check.IsNil)
		c.Assert(updatedJob, check.DeepEquals, &t.expectedJob)
		servicemanager.Job.RemoveJob(context.TODO(), &t.newJob)
	}
}

func (s *S) TestUpdateJobFailedShouldRollback(c *check.C) {
	oldJob := jobTypes.Job{
		Name:      "rollback-test-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Spec: jobTypes.JobSpec{
			Schedule: "10 * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
				Command:          []string{"echo", "original!"},
			},
		},
	}
	newJob := jobTypes.Job{
		Name:      "rollback-test-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Spec: jobTypes.JobSpec{
			Schedule: "20 * * * *", // Changed schedule time
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
				Command:          []string{"echo", "updated!"},
			},
		},
	}

	err := servicemanager.Job.CreateJob(context.TODO(), &oldJob, s.user)
	c.Assert(err, check.IsNil)

	initialJob, err := servicemanager.Job.GetByName(context.TODO(), oldJob.Name)
	c.Assert(err, check.IsNil)
	c.Assert(initialJob.Spec.Schedule, check.Equals, "10 * * * *")
	c.Assert(initialJob.Spec.Container.Command, check.DeepEquals, []string{"echo", "original!"})

	provisionerCalled := false
	originalJobService := servicemanager.Job

	s.mockService.JobService.OnUpdateJob = func(newJob, oldJob *jobTypes.Job, user *authTypes.User) error {
		return originalJobService.UpdateJob(context.TODO(), newJob, oldJob, user)
	}
	s.mockService.JobService.OnUpdateJobProv = func(job *jobTypes.Job) error {
		provisionerCalled = true
		c.Assert(job.Spec.Schedule, check.Equals, "20 * * * *") // Verify we got the new job
		return errors.New("provisioner failure")
	}
	s.mockService.JobService.OnGetByName = func(name string) (*jobTypes.Job, error) {
		return originalJobService.GetByName(context.TODO(), name)
	}

	servicemanager.Job = s.mockService.JobService
	defer func() {
		servicemanager.Job = originalJobService
	}()

	err = servicemanager.Job.UpdateJob(context.TODO(), &newJob, &oldJob, s.user)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "provisioner failure")

	c.Assert(provisionerCalled, check.Equals, true)
	rolledBackJob, err := servicemanager.Job.GetByName(context.TODO(), oldJob.Name)
	c.Assert(err, check.IsNil)
	c.Assert(rolledBackJob.Spec.Schedule, check.Equals, "10 * * * *")                               // Original schedule
	c.Assert(rolledBackJob.Spec.Container.Command, check.DeepEquals, []string{"echo", "original!"}) // Original command

	err = servicemanager.Job.RemoveJob(context.TODO(), &oldJob)
	c.Assert(err, check.IsNil)
}

func (s *S) TestTriggerCronShouldExecuteJob(c *check.C) {
	j1 := jobTypes.Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Spec: jobTypes.JobSpec{
			Schedule: "@yearly",
			Manual:   true,
			Container: jobTypes.ContainerInfo{
				Command: []string{"echo", "hello world!"},
			},
		},
		DeployOptions: &jobTypes.DeployOptions{
			Kind:  provisionTypes.DeployImage,
			Image: "alpine:latest",
		},
	}
	err := servicemanager.Job.CreateJob(context.TODO(), &j1, s.user)
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.ProvisionedJob(j1.Name), check.Equals, true)
	c.Assert(s.provisioner.JobExecutions(j1.Name), check.Equals, 0)
	err = servicemanager.Job.Trigger(context.TODO(), &j1)
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.JobExecutions(j1.Name), check.Equals, 1)
}

func (s *S) TestList(c *check.C) {
	j1 := jobTypes.Job{
		Name:      "j1",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
				Command:          []string{"echo", "hello!"},
			},
		},
	}
	j2 := jobTypes.Job{
		Name:      "j2",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Spec: jobTypes.JobSpec{
			Schedule: "* */2 * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
				Command:          []string{"echo", "hello world!"},
			},
		},
	}
	err := servicemanager.Job.CreateJob(context.TODO(), &j1, s.user)
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.CreateJob(context.TODO(), &j2, s.user)
	c.Assert(err, check.IsNil)
	jobs, err := servicemanager.Job.List(context.TODO(), &jobTypes.Filter{})
	c.Assert(err, check.IsNil)
	c.Assert(len(jobs), check.Equals, 2)
}

func (s *S) TestGetEnvs(c *check.C) {
	job := &jobTypes.Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Envs: []bindTypes.EnvVar{
				{
					Name:   "MY_VAR",
					Value:  "my-value",
					Public: true,
				},
			},
		},
	}
	expected := map[string]bindTypes.EnvVar{
		"MY_VAR": {
			Name:   "MY_VAR",
			Value:  "my-value",
			Public: true,
		},
		"TSURU_SERVICES": {
			Name:      "TSURU_SERVICES",
			Value:     "{}",
			ManagedBy: "tsuru",
		},
	}
	env := servicemanager.Job.GetEnvs(context.TODO(), job)
	c.Assert(env, check.DeepEquals, expected)
}

func (s *S) TestGetEnvsWithServiceEnvs(c *check.C) {
	job := &jobTypes.Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Envs: []bindTypes.EnvVar{
				{
					Name:   "MY_VAR",
					Value:  "my-value",
					Public: true,
				},
			},
			ServiceEnvs: []bindTypes.ServiceEnvVar{{
				EnvVar: bindTypes.EnvVar{
					Name:   "MY_SERVICE_VAR",
					Value:  "my-service-value",
					Public: true,
				},
				ServiceName:  "my-service",
				InstanceName: "my-instance",
			}},
		},
	}
	expected := map[string]bindTypes.EnvVar{
		"MY_VAR": {
			Name:   "MY_VAR",
			Value:  "my-value",
			Public: true,
		},
		"MY_SERVICE_VAR": {
			Name:      "MY_SERVICE_VAR",
			Value:     "my-service-value",
			ManagedBy: "my-service/my-instance",
			Public:    true,
		},
		"TSURU_SERVICES": {
			Name:      "TSURU_SERVICES",
			Value:     "{\"my-service\":[{\"instance_name\":\"my-instance\",\"envs\":{\"MY_SERVICE_VAR\":\"my-service-value\"}}]}",
			ManagedBy: "tsuru",
		},
	}
	envs := servicemanager.Job.GetEnvs(context.TODO(), job)
	c.Assert(envs, check.DeepEquals, expected)
}

func (s *S) TestJobEnvsWithServiceEnvConflict(c *check.C) {
	job := &jobTypes.Job{
		Name:      "some-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Envs: []bindTypes.EnvVar{
				{
					Name:   "MY_VAR",
					Value:  "my-value",
					Public: true,
				},
				{
					Name:   "DB_HOST",
					Value:  "fake.host",
					Public: true,
				},
			},
			ServiceEnvs: []bindTypes.ServiceEnvVar{{
				EnvVar: bindTypes.EnvVar{
					Name:   "DB_HOST",
					Value:  "fake.host1",
					Public: true,
				},
				ServiceName:  "my-service",
				InstanceName: "my-instance-1",
			}, {
				EnvVar: bindTypes.EnvVar{
					Name:   "DB_HOST",
					Value:  "fake.host2",
					Public: false,
				},
				ServiceName:  "my-service",
				InstanceName: "my-instance-2",
			}},
		},
	}

	expected := map[string]bindTypes.EnvVar{
		"MY_VAR": {
			Name:   "MY_VAR",
			Value:  "my-value",
			Public: true,
		},
		"DB_HOST": {
			Name:      "DB_HOST",
			Value:     "fake.host2",
			Public:    false,
			ManagedBy: "my-service/my-instance-2",
		},
	}
	env := servicemanager.Job.GetEnvs(context.TODO(), job)
	serviceEnvsRaw := env[tsuruEnvs.TsuruServicesEnvVar]
	delete(env, tsuruEnvs.TsuruServicesEnvVar)
	c.Assert(env, check.DeepEquals, expected)

	var serviceEnvVal map[string]interface{}
	err := json.Unmarshal([]byte(serviceEnvsRaw.Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{
		"my-service": []interface{}{
			map[string]interface{}{"instance_name": "my-instance-1", "envs": map[string]interface{}{
				"DB_HOST": "fake.host1",
			}},
			map[string]interface{}{"instance_name": "my-instance-2", "envs": map[string]interface{}{
				"DB_HOST": "fake.host2",
			}},
		},
	})
}

func (s *S) TestAddServiceEnvToJobs(c *check.C) {
	job1 := jobTypes.Job{
		Name:      "job1",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
				Command:          []string{"echo", "hello!"},
			},
		},
	}
	cronjob1 := jobTypes.Job{
		Name:      "cronjob1",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
				Command:          []string{"echo", "hello!"},
			},
		},
	}

	err := servicemanager.Job.CreateJob(context.TODO(), &job1, s.user)
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.CreateJob(context.TODO(), &cronjob1, s.user)
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
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
				Command:          []string{"echo", "hello!"},
			},
		},
	}

	err := servicemanager.Job.CreateJob(context.TODO(), &job1, s.user)
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
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
				Command:          []string{"echo", "hello!"},
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
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
				Command:          []string{"echo", "hello!"},
			},
			ServiceEnvs: []bindTypes.ServiceEnvVar{
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "srv1"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_PORT", Value: "3306"}, InstanceName: "myinstance", ServiceName: "srv1"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root"}, InstanceName: "myinstance", ServiceName: "srv1"},
			},
		},
	}

	err := servicemanager.Job.CreateJob(context.TODO(), &job1, s.user)
	c.Assert(err, check.IsNil)
	err = servicemanager.Job.CreateJob(context.TODO(), &cronjob1, s.user)
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
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
				Command:          []string{"echo", "hello!"},
			},
			ServiceEnvs: []bindTypes.ServiceEnvVar{
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "srv1"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_PORT", Value: "3306"}, InstanceName: "myinstance", ServiceName: "srv1"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root"}, InstanceName: "myinstance", ServiceName: "srv1"},
			},
		},
	}

	err := servicemanager.Job.CreateJob(context.TODO(), &job1, s.user)
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
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
				Command:          []string{"echo", "hello!"},
			},
			ServiceEnvs: []bindTypes.ServiceEnvVar{
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "srv1"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_PORT", Value: "3306"}, InstanceName: "myinstance", ServiceName: "srv1"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root"}, InstanceName: "myinstance", ServiceName: "srv1"},
			},
		},
	}

	err := servicemanager.Job.CreateJob(context.TODO(), &job1, s.user)
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
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
				Command:          []string{"echo", "hello!"},
			},
			ServiceEnvs: []bindTypes.ServiceEnvVar{
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance1", ServiceName: "myservice"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root"}, InstanceName: "myinstance1", ServiceName: "myservice"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "remotehost"}, InstanceName: "myinstance2", ServiceName: "myservice"},
				{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "myhost"}, InstanceName: "ourinstance1", ServiceName: "ourservice"},
			},
		},
	}

	err := servicemanager.Job.CreateJob(context.TODO(), &job1, s.user)
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
