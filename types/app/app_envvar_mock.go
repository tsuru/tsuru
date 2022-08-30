// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import "context"

var _ AppEnvVarService = &MockAppEnvVarService{}
var _ AppEnvVarStorage = &MockAppEnvVarStorage{}

type MockAppEnvVarService struct {
	OnGet   func(a App, envName string) (EnvVar, error)
	OnList  func(a App) ([]EnvVar, error)
	OnSet   func(a App, envs []EnvVar, args SetEnvArgs) error
	OnUnset func(a App, envName []string, args UnsetEnvArgs) error
}

func (s *MockAppEnvVarService) Get(ctx context.Context, a App, envName string) (EnvVar, error) {
	if s.OnGet == nil {
		return EnvVar{}, nil
	}
	return s.OnGet(a, envName)
}

func (s *MockAppEnvVarService) List(ctx context.Context, a App) ([]EnvVar, error) {
	if s.OnList == nil {
		return nil, nil
	}
	return s.OnList(a)
}

func (s *MockAppEnvVarService) Set(ctx context.Context, a App, envs []EnvVar, args SetEnvArgs) error {
	if s.OnSet == nil {
		return nil
	}
	return s.OnSet(a, envs, args)
}

func (s *MockAppEnvVarService) Unset(ctx context.Context, a App, envNames []string, args UnsetEnvArgs) error {
	if s.OnUnset == nil {
		return nil
	}
	return s.OnUnset(a, envNames, args)
}

type MockAppEnvVarStorage struct {
	OnFindAll func(a App) ([]EnvVar, error)
	OnRemove  func(a App, envNames []string) error
	OnUpsert  func(a App, envs []EnvVar) error
}

func (s *MockAppEnvVarStorage) FindAll(ctx context.Context, a App) ([]EnvVar, error) {
	if s.OnFindAll == nil {
		return nil, nil
	}
	return s.OnFindAll(a)
}

func (s *MockAppEnvVarStorage) Remove(ctx context.Context, a App, envNames []string) error {
	if s.OnRemove == nil {
		return nil
	}
	return s.OnRemove(a, envNames)
}

func (s *MockAppEnvVarStorage) Upsert(ctx context.Context, a App, envs []EnvVar) error {
	if s.OnUpsert == nil {
		return nil
	}
	return s.OnUpsert(a, envs)
}

var (
	_ AppServiceEnvVarService = (*MockAppServiceEnvVarService)(nil)
	_ AppServiceEnvVarStorage = (*MockServiceEnvVarStorage)(nil)
)

type MockAppServiceEnvVarService struct {
	OnList     func(a App) ([]ServiceEnvVar, error)
	OnSet      func(a App, envs []ServiceEnvVar, args SetEnvArgs) error
	OnUnset    func(a App, envs []ServiceEnvVarIdentifier, args UnsetEnvArgs) error
	OnUnsetAll func(a App, args UnsetAllArgs) error
}

func (m *MockAppServiceEnvVarService) List(ctx context.Context, a App) ([]ServiceEnvVar, error) {
	if m.OnList == nil {
		return nil, nil
	}
	return m.OnList(a)
}

func (m *MockAppServiceEnvVarService) Set(ctx context.Context, a App, envs []ServiceEnvVar, args SetEnvArgs) error {
	if m.OnSet == nil {
		return nil
	}

	return m.OnSet(a, envs, args)
}

func (m *MockAppServiceEnvVarService) Unset(ctx context.Context, a App, envs []ServiceEnvVarIdentifier, args UnsetEnvArgs) error {
	if m.OnUnset == nil {
		return nil
	}
	return m.OnUnset(a, envs, args)
}

func (m *MockAppServiceEnvVarService) UnsetAll(ctx context.Context, a App, args UnsetAllArgs) error {
	if m.OnUnsetAll == nil {
		return nil
	}
	return m.OnUnsetAll(a, args)
}

type MockServiceEnvVarStorage struct {
	OnFindAll func(a App) ([]ServiceEnvVar, error)
	OnRemove  func(a App, envNames []ServiceEnvVarIdentifier) error
	OnUpsert  func(a App, envs []ServiceEnvVar) error
}

func (m *MockServiceEnvVarStorage) FindAll(ctx context.Context, a App) ([]ServiceEnvVar, error) {
	if m.OnFindAll == nil {
		return nil, nil
	}
	return m.OnFindAll(a)
}

func (m *MockServiceEnvVarStorage) Remove(ctx context.Context, a App, envNames []ServiceEnvVarIdentifier) error {
	if m.OnRemove == nil {
		return nil
	}
	return m.OnRemove(a, envNames)
}

func (m *MockServiceEnvVarStorage) Upsert(ctx context.Context, a App, envs []ServiceEnvVar) error {
	if m.OnUpsert == nil {
		return nil
	}
	return m.OnUpsert(a, envs)
}
