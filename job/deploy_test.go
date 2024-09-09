// Copyright 2024 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"bytes"
	"context"
	"fmt"

	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/servicemanager"
	jobTypes "github.com/tsuru/tsuru/types/job"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"gopkg.in/check.v1"
)

func (s *S) TestJobDeployWithImage(c *check.C) {
	job := jobTypes.Job{
		Name:      "my-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
			},
		},
	}
	err := servicemanager.Job.CreateJob(context.TODO(), &job, s.user)
	c.Assert(err, check.IsNil)

	s.builder.OnBuildJob = func(job *jobTypes.Job, opts builder.BuildOpts) (string, error) {
		fmt.Fprintf(opts.Output, "building job %s from image", job.Name)
		c.Assert(opts.ImageID, check.Equals, "my-image")
		c.Assert(opts.Dockerfile, check.Equals, "")
		return fmt.Sprintf("fake.registry.io/job-%s:latest", job.Name), nil
	}

	jobDeployOptions := jobTypes.DeployOptions{
		JobName: job.Name,
		Image:   "my-image",
		Kind:    provTypes.DeployImage,
	}

	writer := &bytes.Buffer{}
	imageID, err := servicemanager.Job.Deploy(context.TODO(), jobDeployOptions, &job, writer)
	c.Assert(err, check.IsNil)
	c.Assert(imageID, check.Equals, "fake.registry.io/job-my-job:latest")
	c.Assert(writer.String(), check.Equals, "building job my-job from image")
}

func (s *S) TestJobDeployWithDockerfile(c *check.C) {
	job := jobTypes.Job{
		Name:      "my-job",
		TeamOwner: s.team.Name,
		Pool:      s.Pool,
		Teams:     []string{s.team.Name},
		Spec: jobTypes.JobSpec{
			Schedule: "* * * * *",
			Container: jobTypes.ContainerInfo{
				OriginalImageSrc: "alpine:latest",
			},
		},
	}
	err := servicemanager.Job.CreateJob(context.TODO(), &job, s.user)
	c.Assert(err, check.IsNil)

	s.builder.OnBuildJob = func(job *jobTypes.Job, opts builder.BuildOpts) (string, error) {
		fmt.Fprintf(opts.Output, "building job %s from dockerfile", job.Name)
		c.Assert(opts.ImageID, check.Equals, "")
		c.Assert(opts.Dockerfile, check.Equals, "FROM alpine:latest\nRUN echo hello")
		return fmt.Sprintf("fake.registry.io/job-%s:latest", job.Name), nil
	}

	jobDeployOptions := jobTypes.DeployOptions{
		JobName:    job.Name,
		Dockerfile: "FROM alpine:latest\nRUN echo hello",
		Kind:       provTypes.DeployImage,
	}

	writer := &bytes.Buffer{}
	imageID, err := servicemanager.Job.Deploy(context.TODO(), jobDeployOptions, &job, writer)
	c.Assert(err, check.IsNil)
	c.Assert(imageID, check.Equals, "fake.registry.io/job-my-job:latest")
	c.Assert(writer.String(), check.Equals, "building job my-job from dockerfile")
}
