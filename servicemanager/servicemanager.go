// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicemanager

import (
	"github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/types/app/image"
	"github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/types/cache"
	"github.com/tsuru/tsuru/types/event"
	"github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/types/quota"
	"github.com/tsuru/tsuru/types/router"
	"github.com/tsuru/tsuru/types/service"
	"github.com/tsuru/tsuru/types/tracker"
)

var (
	App                       app.AppService
	AppCache                  cache.AppCacheService
	Plan                      app.PlanService
	Platform                  app.PlatformService
	PlatformImage             image.PlatformImageService
	Team                      auth.TeamService
	TeamToken                 auth.TeamTokenService
	Webhook                   event.WebhookService
	AppQuota                  quota.QuotaService
	UserQuota                 quota.QuotaService
	Cluster                   provision.ClusterService
	ServiceBroker             service.ServiceBrokerService
	ServiceBrokerCatalogCache service.ServiceBrokerCatalogCacheService
	AppLog                    app.AppLogService
	InstanceTracker           tracker.InstanceService
	AppVersion                app.AppVersionService
	DynamicRouter             router.DynamicRouterService
)
