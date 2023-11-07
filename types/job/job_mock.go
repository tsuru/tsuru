// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"context"

	authTypes "github.com/tsuru/tsuru/types/auth"
	bindTypes "github.com/tsuru/tsuru/types/bind"
)

var _ JobService = &MockJobService{}

type MockJobService struct {
	OnCreateJob        func(*Job, *authTypes.User) error
	OnGetByName        func(string) (*Job, error)
	OnList             func(*Filter) ([]Job, error)
	OnRemoveJob        func(*Job) error
	OnRemoveJobProv    func(*Job) error
	OnTrigger          func(*Job) error
	OnAddServiceEnv    func(*Job, AddInstanceArgs) error
	OnRemoveServiceEnv func(*Job, RemoveInstanceArgs) error
	OnUpdateJob        func(*Job, *Job, *authTypes.User) error
	OnUpdateJobProv    func(*Job) error
	OnGetEnvs          func(*Job) map[string]bindTypes.EnvVar
	OnBaseImageName    func(context.Context, *Job) (string, error)
}

func (m *MockJobService) CreateJob(ctx context.Context, job *Job, user *authTypes.User) error {
	if m.OnCreateJob == nil {
		return nil
	}
	return m.OnCreateJob(job, user)
}

func (m *MockJobService) RemoveJobProv(ctx context.Context, job *Job) error {
	if m.OnRemoveJobProv == nil {
		return nil
	}
	return m.OnRemoveJobProv(job)
}

func (m *MockJobService) GetByName(ctx context.Context, name string) (*Job, error) {
	if m.OnGetByName == nil {
		return nil, nil
	}
	return m.OnGetByName(name)
}

func (m *MockJobService) List(ctx context.Context, filter *Filter) ([]Job, error) {
	if m.OnList == nil {
		return nil, nil
	}
	return m.OnList(filter)
}

func (m *MockJobService) RemoveJob(ctx context.Context, job *Job) error {
	if m.OnRemoveJob == nil {
		return nil
	}
	return m.OnRemoveJob(job)
}

func (m *MockJobService) Trigger(ctx context.Context, job *Job) error {
	if m.OnTrigger == nil {
		return nil
	}
	return m.OnTrigger(job)
}

func (m *MockJobService) UpdateJob(ctx context.Context, newJob, oldJob *Job, user *authTypes.User) error {
	if m.OnUpdateJob == nil {
		return nil
	}
	return m.OnUpdateJob(newJob, oldJob, user)
}

func (m *MockJobService) AddServiceEnv(ctx context.Context, job *Job, addArgs AddInstanceArgs) error {
	if m.OnAddServiceEnv == nil {
		return nil
	}
	return m.OnAddServiceEnv(job, addArgs)
}

func (m *MockJobService) RemoveServiceEnv(ctx context.Context, job *Job, removeArgs RemoveInstanceArgs) error {
	if m.OnRemoveServiceEnv == nil {
		return nil
	}
	return m.OnRemoveServiceEnv(job, removeArgs)
}

func (m *MockJobService) UpdateJobProv(ctx context.Context, job *Job) error {
	if m.OnUpdateJobProv == nil {
		return nil
	}
	return m.OnUpdateJobProv(job)
}

func (m *MockJobService) GetEnvs(ctx context.Context, job *Job) map[string]bindTypes.EnvVar {
	if m.OnGetEnvs == nil {
		return nil
	}
	return m.OnGetEnvs(job)
}

func (m *MockJobService) BaseImageName(ctx context.Context, job *Job) (string, error) {
	if m.OnBaseImageName == nil {
		return "", nil
	}
	return m.OnBaseImageName(ctx, job)
}
