// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package servicecommon

import (
	"context"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/router/rebuild"
)

func RebuildRoutesPoolApps(pool string) {
	apps, err := app.List(context.TODO(), &app.Filter{Pool: pool})
	if err != nil {
		log.Errorf("[rebuild pool apps] unable to list apps for pool %q: %v", pool, err)
		return
	}
	for _, a := range apps {
		rebuild.LockedRoutesRebuildOrEnqueue(a.Name)
	}
}
