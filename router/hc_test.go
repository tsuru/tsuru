// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router_test

import (
	"errors"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/hc"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/routertest"
	"gopkg.in/check.v1"
)

func (s *ExternalSuite) TestBuildHealthCheck(c *check.C) {
	config.Set("routers:fake-hc:type", "fake-hc")
	fn := router.BuildHealthCheck("fake-hc")
	c.Assert(fn(), check.IsNil)
}

func (s *ExternalSuite) TestBuildHealthCheckCustomRouter(c *check.C) {
	config.Set("routers:fakeee:type", "fake-hc")
	fn := router.BuildHealthCheck("fakeee")
	c.Assert(fn(), check.IsNil)
}

func (s *ExternalSuite) TestBuildHealthCheckFailure(c *check.C) {
	config.Set("routers:fake-hc:type", "fake-hc")
	err := errors.New("fatal error")
	routertest.HCRouter.SetErr(err)
	defer routertest.HCRouter.SetErr(nil)
	fn := router.BuildHealthCheck("fake-hc")
	c.Assert(fn(), check.Equals, err)
}

func (s *ExternalSuite) TestBuildHealthCheckUnconfigured(c *check.C) {
	if old, err := config.Get("routers"); err == nil {
		defer config.Set("routers", old)
	}
	config.Unset("routers")
	fn := router.BuildHealthCheck("fake-hc")
	c.Assert(fn(), check.Equals, hc.ErrDisabledComponent)
}

func (s *ExternalSuite) TestBuildHealthCheckNoHealthChecker(c *check.C) {
	config.Set("routers:fakeee:type", "fake")
	fn := router.BuildHealthCheck("fakeee")
	c.Assert(fn(), check.Equals, hc.ErrDisabledComponent)
}
