// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicemanager

import (
	"github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/types/auth"
)

// MockService is a struct to use in tests
type MockService struct {
	Cache    *app.MockCacheService
	Plan     *app.MockPlanService
	Platform *app.MockPlatformService
	Team     *auth.MockTeamService
}

// SetMockService return a new MockService and set as an servicemanager
func SetMockService(m *MockService) {
	m.Cache = &app.MockCacheService{}
	m.Plan = &app.MockPlanService{}
	m.Platform = &app.MockPlatformService{}
	m.Team = &auth.MockTeamService{}
	Cache = m.Cache
	Plan = m.Plan
	Platform = m.Platform
	Team = m.Team
}

func (m *MockService) ResetCache() {
	m.Cache.OnCreate = nil
	m.Cache.OnFindByName = nil
	m.Cache.OnList = nil
}

func (m *MockService) ResetPlan() {
	m.Plan.OnCreate = nil
	m.Plan.OnFindByName = nil
	m.Plan.OnList = nil
	m.Plan.OnDefaultPlan = nil
	m.Plan.OnRemove = nil
}

func (m *MockService) ResetPlatform() {
	m.Platform.OnCreate = nil
	m.Platform.OnFindByName = nil
	m.Platform.OnList = nil
	m.Platform.OnRemove = nil
	m.Platform.OnUpdate = nil
}

func (m *MockService) ResetTeam() {
	m.Team.OnCreate = nil
	m.Team.OnFindByName = nil
	m.Team.OnList = nil
	m.Team.OnRemove = nil
	m.Team.OnFindByNames = nil
}
