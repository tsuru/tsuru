// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

var (
	_ GroupService = &MockGroupService{}
)

type MockGroupService struct {
	OnAddRole    func(name, roleName, contextValue string) error
	OnRemoveRole func(name, roleName, contextValue string) error
	OnList       func(filter []string) ([]Group, error)
}

func (m *MockGroupService) AddRole(name string, roleName, contextValue string) error {
	if m.OnAddRole == nil {
		return nil
	}
	return m.OnAddRole(name, roleName, contextValue)
}

func (m *MockGroupService) RemoveRole(name, roleName, contextValue string) error {
	if m.OnRemoveRole == nil {
		return nil
	}
	return m.OnRemoveRole(name, roleName, contextValue)
}

func (m *MockGroupService) List(filter []string) ([]Group, error) {
	if m.OnList == nil {
		return nil, nil
	}
	return m.OnList(filter)
}
