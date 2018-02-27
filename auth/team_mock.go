// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import authTypes "github.com/tsuru/tsuru/types/auth"

var _ authTypes.TeamService = &MockTeamService{}

type MockTeamService struct {
	OnCreate      func(string, *authTypes.User) error
	OnList        func() ([]authTypes.Team, error)
	OnFindByName  func(string) (*authTypes.Team, error)
	OnFindByNames func([]string) ([]authTypes.Team, error)
	OnRemove      func(string) error
}

func (m *MockTeamService) Create(teamName string, user *authTypes.User) error {
	if m.OnCreate == nil {
		return nil
	}
	return m.OnCreate(teamName, user)
}

func (m *MockTeamService) List() ([]authTypes.Team, error) {
	if m.OnList == nil {
		return nil, nil
	}
	return m.OnList()
}

func (m *MockTeamService) FindByName(teamName string) (*authTypes.Team, error) {
	if m.OnFindByName == nil {
		return nil, nil
	}
	return m.OnFindByName(teamName)
}

func (m *MockTeamService) FindByNames(teamNames []string) ([]authTypes.Team, error) {
	if m.OnFindByNames == nil {
		return nil, nil
	}
	return m.OnFindByNames(teamNames)
}

func (m *MockTeamService) Remove(teamName string) error {
	if m.OnRemove == nil {
		return nil
	}
	return m.OnRemove(teamName)
}
