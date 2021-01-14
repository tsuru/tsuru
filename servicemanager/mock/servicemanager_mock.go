// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/types/app/image"
	"github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/types/cache"
	"github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/types/quota"
	"github.com/tsuru/tsuru/types/router"
	"github.com/tsuru/tsuru/types/service"
	"github.com/tsuru/tsuru/types/tracker"
	"github.com/tsuru/tsuru/types/volume"
)

// MockService is a struct to use in tests
type MockService struct {
	Cache                     *cache.MockAppCacheService
	Plan                      *app.MockPlanService
	Platform                  *app.MockPlatformService
	PlatformImage             *image.MockPlatformImageService
	Team                      *auth.MockTeamService
	UserQuota                 *quota.MockQuotaService
	AppQuota                  *quota.MockQuotaService
	Cluster                   *provision.MockClusterService
	ServiceBroker             *service.MockServiceBrokerService
	ServiceBrokerCatalogCache *service.MockServiceBrokerCatalogCacheService
	InstanceTracker           *tracker.MockInstanceService
	DynamicRouter             *router.MockDynamicRouterService
	AuthGroup                 *auth.MockGroupService
	Pool                      *provision.MockPoolService
	VolumeService             *volume.MockVolumeService
}

// SetMockService return a new MockService and set as a servicemanager
func SetMockService(m *MockService) {
	m.Cache = &cache.MockAppCacheService{}
	m.Plan = &app.MockPlanService{}
	m.Platform = &app.MockPlatformService{}
	m.PlatformImage = &image.MockPlatformImageService{}
	m.Team = &auth.MockTeamService{}
	m.UserQuota = &quota.MockQuotaService{}
	m.AppQuota = &quota.MockQuotaService{}
	m.Cluster = &provision.MockClusterService{}
	m.ServiceBroker = &service.MockServiceBrokerService{}
	m.ServiceBrokerCatalogCache = &service.MockServiceBrokerCatalogCacheService{}
	m.InstanceTracker = &tracker.MockInstanceService{}
	m.DynamicRouter = &router.MockDynamicRouterService{}
	m.AuthGroup = &auth.MockGroupService{}
	m.Pool = &provision.MockPoolService{}

	m.VolumeService = &volume.MockVolumeService{}
	servicemanager.AppCache = m.Cache
	servicemanager.Plan = m.Plan
	servicemanager.Platform = m.Platform
	servicemanager.PlatformImage = m.PlatformImage
	servicemanager.Team = m.Team
	servicemanager.UserQuota = m.UserQuota
	servicemanager.AppQuota = m.AppQuota
	servicemanager.Cluster = m.Cluster
	servicemanager.ServiceBroker = m.ServiceBroker
	servicemanager.ServiceBrokerCatalogCache = m.ServiceBrokerCatalogCache
	servicemanager.InstanceTracker = m.InstanceTracker
	servicemanager.DynamicRouter = m.DynamicRouter
	servicemanager.AuthGroup = m.AuthGroup
	servicemanager.Pool = m.Pool
	servicemanager.Volume = m.VolumeService
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

func (m *MockService) ResetPlatformImage() {
	m.PlatformImage.OnNewImage = nil
	m.PlatformImage.OnCurrentImage = nil
	m.PlatformImage.OnAppendImage = nil
	m.PlatformImage.OnDeleteImages = nil
	m.PlatformImage.OnListImages = nil
	m.PlatformImage.OnListImagesOrDefault = nil
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
}

func (m *MockService) ResetAppQuota() {
	m.AppQuota.OnInc = nil
	m.AppQuota.OnSet = nil
	m.AppQuota.OnSetLimit = nil
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

func (m *MockService) ResetServiceBroker() {
	m.ServiceBroker.OnCreate = nil
	m.ServiceBroker.OnUpdate = nil
	m.ServiceBroker.OnDelete = nil
	m.ServiceBroker.OnFind = nil
	m.ServiceBroker.OnList = nil
}

func (m *MockService) ResetServiceBrokerCatalogCache() {
	m.ServiceBrokerCatalogCache.OnSave = nil
	m.ServiceBrokerCatalogCache.OnLoad = nil
}

func (m *MockService) ResetPool() {
	m.Pool.OnFindByName = nil
	m.Pool.OnList = nil
}
