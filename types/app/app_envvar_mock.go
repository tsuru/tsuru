// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import "context"

var _ AppEnvVarService = &MockAppEnvVarService{}

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
