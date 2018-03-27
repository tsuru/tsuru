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
