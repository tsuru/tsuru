// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
)

var _ AppEnvVarService = (*MockAppEnvVarService)(nil)
var _ AppEnvVarStorage = (*MockAppEnvVarStorage)(nil)

type MockAppEnvVarService struct {
	OnGet   func(a App, envName string) (EnvVar, error)
	OnList  func(a App) ([]EnvVar, error)
	OnSet   func(a App, envs []EnvVar, args SetEnvArgs) error
	OnUnset func(a App, envName []string, args UnsetEnvArgs) error

	State map[string][]EnvVar // env vars by app name
}

func (s *MockAppEnvVarService) Get(ctx context.Context, a App, envName string) (EnvVar, error) {
	if s.OnGet != nil {
		return s.OnGet(a, envName)
	}

	for _, env := range s.State[a.GetName()] {
		if env.Name == envName {
			return env, nil
		}
	}

	return EnvVar{}, nil
}

func (s *MockAppEnvVarService) List(ctx context.Context, a App) ([]EnvVar, error) {
	if s.OnList != nil {
		return s.OnList(a)
	}

	return s.State[a.GetName()], nil
}

func (s *MockAppEnvVarService) Set(ctx context.Context, a App, envs []EnvVar, args SetEnvArgs) error {
	if s.OnSet != nil {
		return s.OnSet(a, envs, args)
	}

	if s.State == nil {
		s.State = make(map[string][]EnvVar)
	}

	for _, env := range envs {
		i, found := findAppEnvVar(s.State[a.GetName()], env.Name)
		if !found {
			s.State[a.GetName()] = append(s.State[a.GetName()], env)
			continue
		}

		s.State[a.GetName()][i] = env
	}

	return nil
}

func (s *MockAppEnvVarService) Unset(ctx context.Context, a App, envNames []string, args UnsetEnvArgs) error {
	if s.OnUnset != nil {
		return s.OnUnset(a, envNames, args)
	}

	for _, envName := range envNames {
		envs := s.State[a.GetName()]

		i, found := findAppEnvVar(envs, envName)
		if !found {
			continue
		}

		s.State[a.GetName()] = append(envs[:i], envs[i+1:]...)
	}

	return nil
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

	State map[string][]ServiceEnvVar // service env vars by app name
}

func (m *MockAppServiceEnvVarService) List(ctx context.Context, a App) ([]ServiceEnvVar, error) {
	if m.OnList != nil {
		return m.OnList(a)
	}

	return m.State[a.GetName()], nil
}

func (m *MockAppServiceEnvVarService) Set(ctx context.Context, a App, envs []ServiceEnvVar, args SetEnvArgs) error {
	if m.OnSet != nil {
		return m.OnSet(a, envs, args)
	}

	if m.State == nil {
		m.State = make(map[string][]ServiceEnvVar)
	}

	for _, env := range envs {
		i, found := findServiceEnvVar(m.State[a.GetName()], env.ServiceName, env.InstanceName, env.Name)
		if !found {
			m.State[a.GetName()] = append(m.State[a.GetName()], env)
			continue
		}

		m.State[a.GetName()][i] = env
	}

	return nil
}

func (m *MockAppServiceEnvVarService) Unset(ctx context.Context, a App, envs []ServiceEnvVarIdentifier, args UnsetEnvArgs) error {
	if m.OnUnset != nil {
		return m.OnUnset(a, envs, args)
	}

	for _, env := range envs {
		i, found := findServiceEnvVar(m.State[a.GetName()], env.GetServiceName(), env.GetInstanceName(), env.GetEnvVarName())
		if !found {
			continue
		}

		m.State[a.GetName()] = append(m.State[a.GetName()][:i], m.State[a.GetName()][i+1:]...)
	}

	return nil
}

func (m *MockAppServiceEnvVarService) UnsetAll(ctx context.Context, a App, args UnsetAllArgs) error {
	if m.OnUnsetAll != nil {
		return m.OnUnsetAll(a, args)
	}

	envs := m.State[a.GetName()]

	for i, env := range envs {
		if env.ServiceName == args.Service && env.InstanceName == args.Instance {
			m.State[a.GetName()] = append(m.State[a.GetName()][:i], m.State[a.GetName()][i+1:]...)
		}
	}

	return nil
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

func findServiceEnvVar(envs []ServiceEnvVar, service, instance, name string) (int, bool) {
	for i, env := range envs {
		if env.ServiceName == service && env.InstanceName == instance && env.Name == name {
			return i, true
		}
	}

	return -1, false
}

func findAppEnvVar(envs []EnvVar, name string) (int, bool) {
	for i, env := range envs {
		if env.Name == name {
			return i, true
		}
	}

	return -1, false
}
