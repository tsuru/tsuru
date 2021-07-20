// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracker

import "context"

var _ InstanceService = (*MockInstanceService)(nil)

// MockInstanceService implements InstanceService interface
type MockInstanceService struct {
	OnLiveInstances   func() ([]TrackedInstance, error)
	OnCurrentInstance func() (TrackedInstance, error)
}

func (m *MockInstanceService) LiveInstances(ctx context.Context) ([]TrackedInstance, error) {
	if m.OnLiveInstances != nil {
		return m.OnLiveInstances()
	}
	return []TrackedInstance{}, nil
}

func (m *MockInstanceService) CurrentInstance(ctx context.Context) (TrackedInstance, error) {
	if m.OnCurrentInstance != nil {
		return m.OnCurrentInstance()
	}
	return TrackedInstance{Name: "hostname", Addresses: []string{"127.0.0.1"}}, nil
}
