// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rebuild_test

import (
	"context"
	"net/url"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/router/routertest"
	check "gopkg.in/check.v1"
)

func (s *S) TestRoutesRebuildOrEnqueueNoError(c *check.C) {
	a := &app.App{
		Name:      "almah",
		Platform:  "static",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	invalidAddr, err := url.Parse("http://invalid.addr")
	c.Assert(err, check.IsNil)
	err = routertest.FakeRouter.AddRoutes(a.GetName(), []*url.URL{invalidAddr})
	c.Assert(err, check.IsNil)
	rebuild.RoutesRebuildOrEnqueue(a.GetName())
	c.Assert(routertest.FakeRouter.HasRoute(a.GetName(), invalidAddr.String()), check.Equals, false)
}

func (s *S) TestRoutesRebuildOrEnqueueForceEnqueue(c *check.C) {
	a := &app.App{
		Name:      "almah",
		Platform:  "static",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	invalidAddr, err := url.Parse("http://invalid.addr")
	c.Assert(err, check.IsNil)
	err = routertest.FakeRouter.AddRoutes(a.GetName(), []*url.URL{invalidAddr})
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.FailForIp(invalidAddr.String())
	rebuild.RoutesRebuildOrEnqueue(a.GetName())
	c.Assert(routertest.FakeRouter.HasRoute(a.GetName(), invalidAddr.String()), check.Equals, true)
	routertest.FakeRouter.RemoveFailForIp(invalidAddr.String())
	rebuild.Shutdown(context.Background())
	c.Assert(routertest.FakeRouter.HasRoute(a.GetName(), invalidAddr.String()), check.Equals, false)
}

func (s *S) TestRoutesRebuildOrEnqueueLocked(c *check.C) {
	a := &app.App{
		Name:      "almah",
		Platform:  "static",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	locked, err := app.AcquireApplicationLock(a.Name, "me", "mine")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
	invalidAddr, err := url.Parse("http://invalid.addr")
	c.Assert(err, check.IsNil)
	err = routertest.FakeRouter.AddRoutes(a.GetName(), []*url.URL{invalidAddr})
	c.Assert(err, check.IsNil)
	rebuild.LockedRoutesRebuildOrEnqueue(a.GetName())
	c.Assert(routertest.FakeRouter.HasRoute(a.GetName(), invalidAddr.String()), check.Equals, true)
	app.ReleaseApplicationLock(a.Name)
	rebuild.Shutdown(context.Background())
	c.Assert(routertest.FakeRouter.HasRoute(a.GetName(), invalidAddr.String()), check.Equals, false)
}
