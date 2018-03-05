// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

var _ TeamService = &MockTeamService{}

type MockTeamService struct {
	OnCreate      func(string, *User) error
	OnList        func() ([]Team, error)
	OnFindByName  func(string) (*Team, error)
	OnFindByNames func([]string) ([]Team, error)
	OnRemove      func(string) error
}

func (m *MockTeamService) Create(teamName string, user *User) error {
	if m.OnCreate == nil {
		return nil
	}
	return m.OnCreate(teamName, user)
}

func (m *MockTeamService) List() ([]Team, error) {
	if m.OnList == nil {
		return nil, nil
	}
	return m.OnList()
}

func (m *MockTeamService) FindByName(teamName string) (*Team, error) {
	if m.OnFindByName == nil {
		return nil, nil
	}
	return m.OnFindByName(teamName)
}

func (m *MockTeamService) FindByNames(teamNames []string) ([]Team, error) {
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
