// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import "context"

var _ PoolStorage = &MockPoolStorage{}
var _ PoolService = &MockPoolService{}

type MockPoolStorage struct {
	OnFindAll    func() ([]Pool, error)
	OnFindByName func(string) (*Pool, error)
}

func (m *MockPoolStorage) FindAll(ctx context.Context) ([]Pool, error) {
	if m.OnFindAll != nil {
		return m.OnFindAll()
	}
	return nil, nil
}

func (m *MockPoolStorage) FindByName(ctx context.Context, name string) (*Pool, error) {
	if m.OnFindByName != nil {
		return m.OnFindByName(name)
	}
	return nil, nil
}

type MockPoolService struct {
	OnList       func() ([]Pool, error)
	OnFindByName func(string) (*Pool, error)
}

func (m *MockPoolService) List(ctx context.Context) ([]Pool, error) {
	if m.OnList != nil {
		return m.OnList()
	}
	return nil, nil
}

func (m *MockPoolService) FindByName(ctx context.Context, name string) (*Pool, error) {
	if m.OnFindByName != nil {
		return m.OnFindByName(name)
	}
	return nil, nil
}
