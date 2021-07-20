// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import "context"

var _ PlanStorage = (*MockPlanStorage)(nil)

// MockPlanStorage implements PlanStorage interface
type MockPlanStorage struct {
	OnInsert      func(Plan) error
	OnFindAll     func() ([]Plan, error)
	OnFindDefault func() (*Plan, error)
	OnFindByName  func(string) (*Plan, error)
	OnDelete      func(Plan) error
}

func (m *MockPlanStorage) Insert(ctx context.Context, p Plan) error {
	return m.OnInsert(p)
}

func (m *MockPlanStorage) FindAll(ctx context.Context) ([]Plan, error) {
	return m.OnFindAll()
}

func (m *MockPlanStorage) FindDefault(ctx context.Context) (*Plan, error) {
	return m.OnFindDefault()
}

func (m *MockPlanStorage) FindByName(ctx context.Context, name string) (*Plan, error) {
	return m.OnFindByName(name)
}

func (m *MockPlanStorage) Delete(ctx context.Context, p Plan) error {
	return m.OnDelete(p)
}

// MockPlanService implements PlanService interface
type MockPlanService struct {
	OnCreate      func(Plan) error
	OnList        func() ([]Plan, error)
	OnFindByName  func(string) (*Plan, error)
	OnDefaultPlan func() (*Plan, error)
	OnRemove      func(string) error
}

func (m *MockPlanService) Create(ctx context.Context, plan Plan) error {
	if m.OnCreate == nil {
		return nil
	}
	return m.OnCreate(plan)
}

func (m *MockPlanService) List(ctx context.Context) ([]Plan, error) {
	if m.OnList == nil {
		return nil, nil
	}
	return m.OnList()
}

func (m *MockPlanService) FindByName(ctx context.Context, name string) (*Plan, error) {
	if m.OnFindByName == nil {
		return nil, nil
	}
	return m.OnFindByName(name)
}

func (m *MockPlanService) DefaultPlan(ctx context.Context) (*Plan, error) {
	if m.OnDefaultPlan == nil {
		return nil, nil
	}
	return m.OnDefaultPlan()
}

func (m *MockPlanService) Remove(ctx context.Context, name string) error {
	if m.OnRemove == nil {
		return nil
	}
	return m.OnRemove(name)
}
