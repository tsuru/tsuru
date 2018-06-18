// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/types/quota"
)

// MockService is a struct to use in tests
type MockService struct {
	Cache     *app.MockCacheService
	Plan      *app.MockPlanService
	Platform  *app.MockPlatformService
	Team      *auth.MockTeamService
	UserQuota *quota.MockQuotaService
	AppQuota  *quota.MockQuotaService
	Cluster   *provision.MockClusterService
}

// SetMockService return a new MockService and set as a servicemanager
func SetMockService(m *MockService) {
	m.Cache = &app.MockCacheService{}
	m.Plan = &app.MockPlanService{}
	m.Platform = &app.MockPlatformService{}
	m.Team = &auth.MockTeamService{}
	m.UserQuota = &quota.MockQuotaService{}
	m.AppQuota = &quota.MockQuotaService{}
	m.Cluster = &provision.MockClusterService{}
	servicemanager.Cache = m.Cache
	servicemanager.Plan = m.Plan
	servicemanager.Platform = m.Platform
	servicemanager.Team = m.Team
	servicemanager.UserQuota = m.UserQuota
	servicemanager.AppQuota = m.AppQuota
	servicemanager.Cluster = m.Cluster
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

func (m *MockService) ResetUserQuota() {
	m.UserQuota.OnInc = nil
	m.UserQuota.OnSet = nil
	m.UserQuota.OnSetLimit = nil
	m.UserQuota.OnGet = nil
}

func (m *MockService) ResetAppQuota() {
	m.AppQuota.OnInc = nil
	m.AppQuota.OnSet = nil
	m.AppQuota.OnSetLimit = nil
	m.AppQuota.OnGet = nil
}

func (m *MockService) ResetCluster() {
	m.Cluster.OnCreate = nil
	m.Cluster.OnUpdate = nil
	m.Cluster.OnList = nil
	m.Cluster.OnFindByName = nil
	m.Cluster.OnFindByProvisioner = nil
	m.Cluster.OnFindByPool = nil
	m.Cluster.OnDelete = nil
}
