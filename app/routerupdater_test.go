// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"

	"github.com/tsuru/tsuru/types/cache"
	check "gopkg.in/check.v1"
)

func (s *S) TestAppRouterUpdaterUpdateWait(c *check.C) {
	s.mockService.Cache.OnList = func(keys ...string) ([]cache.CacheEntry, error) {
		return []cache.CacheEntry{
			{Key: "app-router-addr\x00app1\x00fake", Value: "app1.fakerouter.com"},
		}, nil
	}
	a := App{
		Name:      "app1",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	updater := GetAppRouterUpdater()
	updater.update(&a)
	err = updater.Shutdown(context.Background())
	c.Assert(err, check.IsNil)
	apps, err := List(nil)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].Routers[0].Address, check.Equals, "app1.fakerouter.com")
}
