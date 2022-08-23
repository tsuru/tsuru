// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"

	imgTypes "github.com/tsuru/tsuru/types/app/image"
)

type MockApp struct {
	Name, TeamOwner, Platform, PlatformVersion, Pool string
	Deploys                                          uint
	UpdatePlatform                                   bool
	TeamsName                                        []string
	Registry                                         imgTypes.ImageRegistry
}

func (a *MockApp) GetName() string {
	return a.Name
}

func (a *MockApp) GetPool() string {
	return a.Pool
}

func (a *MockApp) GetTeamsName() []string {
	return a.TeamsName
}

func (a *MockApp) GetTeamOwner() string {
	return a.TeamOwner
}

func (a *MockApp) GetPlatform() string {
	return a.Platform
}

func (a *MockApp) GetPlatformVersion() string {
	return a.PlatformVersion
}

func (a *MockApp) GetDeploys() uint {
	return a.Deploys
}

func (a *MockApp) GetUpdatePlatform() bool {
	return a.UpdatePlatform
}

func (a *MockApp) GetRegistry() (imgTypes.ImageRegistry, error) {
	return a.Registry, nil
}

var _ AppService = &MockAppService{}

type MockAppService struct {
	Apps   []App
	OnList func(filter *Filter) ([]App, error)
}

func (m *MockAppService) GetByName(ctx context.Context, name string) (App, error) {
	for _, app := range m.Apps {
		if app.GetName() == name {
			return app, nil
		}
	}
	return nil, ErrAppNotFound
}

func (m *MockAppService) List(ctx context.Context, f *Filter) ([]App, error) {
	if m.OnList == nil {
		return nil, nil
	}

	return m.OnList(f)
}
