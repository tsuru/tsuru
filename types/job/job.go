// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"context"
	"io"

	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	provisionTypes "github.com/tsuru/tsuru/types/provision"
)

var _ provisionTypes.ResourceGetter = &Job{}

// Job is another main type in tsuru as of version 1.13
// a job currently represents a Kubernetes Job object or a Cronjob object
// this struct carries some tsuru metadata as is the case with the app object
// it also holds a JobSpec value that defines how the Job is supposed to be run
type Job struct {
	Name        string            `json:"name"`
	Teams       []string          `json:"teams"`
	TeamOwner   string            `json:"teamOwner"`
	Owner       string            `json:"owner"`
	Plan        appTypes.Plan     `json:"plan"`
	Metadata    appTypes.Metadata `json:"metadata"`
	Pool        string            `json:"pool"`
	Description string            `json:"description"`

	Spec JobSpec `json:"spec"`
}

func (job *Job) GetName() string {
	return job.Name
}

func (job *Job) GetMemory() int64 {
	if job.Plan.Override.Memory != nil {
		return *job.Plan.Override.Memory
	}
	return job.Plan.Memory
}

func (job *Job) GetMilliCPU() int {
	if job.Plan.Override.CPUMilli != nil {
		return *job.Plan.Override.CPUMilli
	}
	return job.Plan.CPUMilli
}

func (job *Job) GetCPUBurst() float64 {
	if job.Plan.Override.CPUBurst != nil {
		return *job.Plan.Override.CPUBurst
	}
	return job.Plan.CPUBurst.Default
}

func (job *Job) GetPool() string {
	return job.Pool
}

type ContainerInfo struct {
	Image   string   `json:"image"`
	Command []string `json:"command"`
}

type JobSpec struct {
	Completions           *int32                    `json:"completions,omitempty"`
	Parallelism           *int32                    `json:"parallelism,omitempty"`
	ActiveDeadlineSeconds *int64                    `json:"activeDeadlineSeconds,omitempty"`
	BackoffLimit          *int32                    `json:"backoffLimit,omitempty"`
	Schedule              string                    `json:"schedule"`
	Manual                bool                      `json:"manual"`
	Container             ContainerInfo             `json:"container"`
	ServiceEnvs           []bindTypes.ServiceEnvVar `json:"-"`
	Envs                  []bindTypes.EnvVar        `json:"envs"`
}

type Filter struct {
	Name      string
	TeamOwner string
	UserOwner string
	Pool      string
	Pools     []string
	Extra     map[string][]string
}

type AddInstanceArgs struct {
	Envs   []bindTypes.ServiceEnvVar
	Writer io.Writer
}

type RemoveInstanceArgs struct {
	ServiceName  string
	InstanceName string
	Writer       io.Writer
}

type JobService interface {
	CreateJob(ctx context.Context, job *Job, user *authTypes.User) error
	RemoveJobProv(ctx context.Context, job *Job) error
	GetByName(ctx context.Context, name string) (*Job, error)
	List(ctx context.Context, filter *Filter) ([]Job, error)
	RemoveJob(ctx context.Context, job *Job) error
	Trigger(ctx context.Context, job *Job) error
	UpdateJob(ctx context.Context, newJob, oldJob *Job, user *authTypes.User) error
	AddServiceEnv(ctx context.Context, job *Job, addArgs AddInstanceArgs) error
	RemoveServiceEnv(ctx context.Context, job *Job, removeArgs RemoveInstanceArgs) error
	UpdateJobProv(ctx context.Context, job *Job) error
	GetEnvs(ctx context.Context, job *Job) map[string]bindTypes.EnvVar
	BaseImageName(ctx context.Context, job *Job) (string, error)
}
