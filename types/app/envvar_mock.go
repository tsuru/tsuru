// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import "context"

var _ AppServiceEnvVarStorage = (*MockServiceEnvVarStorage)(nil)

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
