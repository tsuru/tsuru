// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicemanager

import (
	"github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/types/auth"
)

var (
	Cache    app.CacheService
	Plan     app.PlanService
	Platform app.PlatformService
	Team     auth.TeamService
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
