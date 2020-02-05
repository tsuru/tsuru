// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

type MockApp struct {
	Name, TeamOwner, Platform, PlatformVersion string
	Deploys                                    uint
	UpdatePlatform                             bool
}

func (a *MockApp) GetName() string {
	return a.Name
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
