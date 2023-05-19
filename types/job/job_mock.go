// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"context"

	authTypes "github.com/tsuru/tsuru/types/auth"
)

var _ JobService = &MockJobService{}

type MockJobService struct {
	OnCreateJob             func(*Job, *authTypes.User, bool) error
	OnDeleteFromProvisioner func(*Job) error
	OnGetByName             func(string) (*Job, error)
	OnList                  func(*Filter) ([]Job, error)
	OnRemoveJobFromDb       func(string) error
	OnTrigger               func(*Job) error
	OnUpdateJob             func(*Job, *Job, *authTypes.User) error
	OnAddInstance           func(*Job, AddInstanceArgs) error
	OnRemoveInstance        func(*Job, RemoveInstanceArgs) error
	OnUpdateJobProv         func(*Job) error
}

func (m *MockJobService) CreateJob(ctx context.Context, job *Job, user *authTypes.User, trigger bool) error {
	if m.OnCreateJob == nil {
		return nil
	}
	return m.OnCreateJob(job, user, trigger)
}

func (m *MockJobService) DeleteFromProvisioner(ctx context.Context, job *Job) error {
	if m.OnDeleteFromProvisioner == nil {
		return nil
	}
	return m.OnDeleteFromProvisioner(job)
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

func (m *MockJobService) RemoveJobFromDb(jobName string) error {
	if m.OnRemoveJobFromDb == nil {
		return nil
	}
	return m.OnRemoveJobFromDb(jobName)
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

func (m *MockJobService) AddInstance(ctx context.Context, job *Job, addArgs AddInstanceArgs) error {
	if m.OnAddInstance == nil {
		return nil
	}
	return m.OnAddInstance(job, addArgs)
}

func (m *MockJobService) RemoveInstance(ctx context.Context, job *Job, removeArgs RemoveInstanceArgs) error {
	if m.OnRemoveInstance == nil {
		return nil
	}
	return m.OnRemoveInstance(job, removeArgs)
}

func (m *MockJobService) UpdateJobProv(ctx context.Context, job *Job) error {
	if m.OnUpdateJobProv == nil {
		return nil
	}
	return m.OnUpdateJobProv(job)
}
