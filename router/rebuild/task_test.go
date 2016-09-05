// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rebuild_test

import (
	"net/url"
	"time"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/router/routertest"
	"gopkg.in/check.v1"
)

func (s *S) TestRoutesRebuildOrEnqueueNoError(c *check.C) {
	coll := s.conn.Apps()
	a := &app.App{
		Name:     "almah",
		Platform: "static",
	}
	err := coll.Insert(a)
	c.Assert(err, check.IsNil)
	err = provisiontest.ProvisionerInstance.Provision(a)
	c.Assert(err, check.IsNil)
	invalidAddr, err := url.Parse("http://invalid.addr")
	c.Assert(err, check.IsNil)
	err = routertest.FakeRouter.AddRoute(a.GetName(), invalidAddr)
	c.Assert(err, check.IsNil)
	rebuild.RoutesRebuildOrEnqueue(a.GetName())
	c.Assert(routertest.FakeRouter.HasRoute(a.GetName(), invalidAddr.String()), check.Equals, false)
}

func (s *S) TestRoutesRebuildOrEnqueueForceEnqueue(c *check.C) {
	coll := s.conn.Apps()
	a := &app.App{
		Name:     "almah",
		Platform: "static",
	}
	err := coll.Insert(a)
	c.Assert(err, check.IsNil)
	err = provisiontest.ProvisionerInstance.Provision(a)
	c.Assert(err, check.IsNil)
	invalidAddr, err := url.Parse("http://invalid.addr")
	c.Assert(err, check.IsNil)
	err = routertest.FakeRouter.AddRoute(a.GetName(), invalidAddr)
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.FailForIp(invalidAddr.String())
	rebuild.RoutesRebuildOrEnqueue(a.GetName())
	c.Assert(routertest.FakeRouter.HasRoute(a.GetName(), invalidAddr.String()), check.Equals, true)
	routertest.FakeRouter.RemoveFailForIp(invalidAddr.String())
	err = queue.TestingWaitQueueTasks(1, 10*time.Second)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasRoute(a.GetName(), invalidAddr.String()), check.Equals, false)
}

func (s *S) TestRoutesRebuildOrEnqueueLocked(c *check.C) {
	coll := s.conn.Apps()
	a := &app.App{
		Name:     "almah",
		Platform: "static",
	}
	err := coll.Insert(a)
	c.Assert(err, check.IsNil)
	locked, err := app.AcquireApplicationLock(a.Name, "me", "mine")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
	err = provisiontest.ProvisionerInstance.Provision(a)
	c.Assert(err, check.IsNil)
	invalidAddr, err := url.Parse("http://invalid.addr")
	c.Assert(err, check.IsNil)
	err = routertest.FakeRouter.AddRoute(a.GetName(), invalidAddr)
	c.Assert(err, check.IsNil)
	rebuild.LockedRoutesRebuildOrEnqueue(a.GetName())
	c.Assert(routertest.FakeRouter.HasRoute(a.GetName(), invalidAddr.String()), check.Equals, true)
	app.ReleaseApplicationLock(a.Name)
	err = queue.TestingWaitQueueTasks(1, 10*time.Second)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasRoute(a.GetName(), invalidAddr.String()), check.Equals, false)
}
