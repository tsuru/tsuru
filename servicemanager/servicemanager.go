// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicemanager

import (
	"github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/types/event"
	"github.com/tsuru/tsuru/types/quota"
)

var (
	Cache     app.CacheService
	Plan      app.PlanService
	Platform  app.PlatformService
	Team      auth.TeamService
	TeamToken auth.TeamTokenService
	Webhook   event.WebhookService
	AppQuota  quota.AppQuotaService
	UserQuota quota.UserQuotaService
)
