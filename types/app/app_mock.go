// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import "context"

type MockApp struct {
	Name, TeamOwner, Platform, PlatformVersion, Pool string
	Deploys                                          uint
	UpdatePlatform                                   bool
}

func (a *MockApp) GetName() string {
	return a.Name
}

func (a *MockApp) GetPool() string {
	return a.Pool
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

var _ AppService = &MockAppService{}

type MockAppService struct {
	Apps []App
}

func (m *MockAppService) GetByName(ctx context.Context, name string) (App, error) {
	for _, app := range m.Apps {
		if app.GetName() == name {
			return app, nil
		}
	}
	return nil, ErrAppNotFound
}
