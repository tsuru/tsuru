// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import "context"

var _ TeamStorage = &MockTeamStorage{}
var _ TeamService = &MockTeamService{}

// MockTeamStorage implements TeamStorage interface
type MockTeamStorage struct {
	OnInsert      func(Team) error
	OnUpdate      func(Team) error
	OnFindAll     func() ([]Team, error)
	OnFindByName  func(string) (*Team, error)
	OnFindByNames func([]string) ([]Team, error)
	OnDelete      func(Team) error
}

func (m *MockTeamStorage) Insert(ctx context.Context, t Team) error {
	return m.OnInsert(t)
}

func (m *MockTeamStorage) Update(ctx context.Context, t Team) error {
	return m.OnUpdate(t)
}

func (m *MockTeamStorage) FindAll(ctx context.Context) ([]Team, error) {
	return m.OnFindAll()
}

func (m *MockTeamStorage) FindByName(ctx context.Context, name string) (*Team, error) {
	return m.OnFindByName(name)
}

func (m *MockTeamStorage) FindByNames(ctx context.Context, names []string) ([]Team, error) {
	return m.OnFindByNames(names)
}

func (m *MockTeamStorage) Delete(ctx context.Context, t Team) error {
	return m.OnDelete(t)
}

type MockTeamService struct {
	OnCreate      func(string, []string, *User) error
	OnUpdate      func(string, []string) error
	OnList        func() ([]Team, error)
	OnFindByName  func(string) (*Team, error)
	OnFindByNames func([]string) ([]Team, error)
	OnRemove      func(string) error
}

func (m *MockTeamService) Create(ctx context.Context, teamName string, tags []string, user *User) error {
	if m.OnCreate == nil {
		return nil
	}
	return m.OnCreate(teamName, tags, user)
}

func (m *MockTeamService) Update(ctx context.Context, teamName string, tags []string) error {
	if m.OnUpdate == nil {
		return nil
	}
	return m.OnUpdate(teamName, tags)
}

func (m *MockTeamService) List(ctx context.Context) ([]Team, error) {
	if m.OnList == nil {
		return nil, nil
	}
	return m.OnList()
}

func (m *MockTeamService) FindByName(ctx context.Context, teamName string) (*Team, error) {
	if m.OnFindByName == nil {
		return nil, nil
	}
	return m.OnFindByName(teamName)
}

func (m *MockTeamService) FindByNames(ctx context.Context, teamNames []string) ([]Team, error) {
	if m.OnFindByNames == nil {
		return nil, nil
	}
	return m.OnFindByNames(teamNames)
}

func (m *MockTeamService) Remove(ctx context.Context, teamName string) error {
	if m.OnRemove == nil {
		return nil
	}
	return m.OnRemove(teamName)
}
