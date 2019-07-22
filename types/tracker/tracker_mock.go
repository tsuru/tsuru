// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracker

var _ InstanceService = &MockInstanceService{}

// MockInstanceService implements InstanceService interface
type MockInstanceService struct {
	OnLiveInstances   func() ([]TrackedInstance, error)
	OnCurrentInstance func() (TrackedInstance, error)
}

func (m *MockInstanceService) LiveInstances() ([]TrackedInstance, error) {
	if m.OnLiveInstances != nil {
		return m.OnLiveInstances()
	}
	return []TrackedInstance{}, nil
}

func (m *MockInstanceService) CurrentInstance() (TrackedInstance, error) {
	if m.OnCurrentInstance != nil {
		return m.OnCurrentInstance()
	}
	return TrackedInstance{Name: "hostname", Addresses: []string{"127.0.0.1"}}, nil
}
