// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rebuild_test

import (
	"context"
	"net/http"
	"net/url"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	routerTypes "github.com/tsuru/tsuru/types/router"
	check "gopkg.in/check.v1"
)

func newVersion(c *check.C, a appTypes.AppInterface) appTypes.AppVersion {
	version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
		App: a,
	})
	c.Assert(err, check.IsNil)
	err = version.CommitBaseImage()
	c.Assert(err, check.IsNil)
	err = version.CommitSuccessful()
	c.Assert(err, check.IsNil)
	return version
}

type URLList []*url.URL

func (l URLList) Len() int           { return len(l) }
func (l URLList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l URLList) Less(i, j int) bool { return l[i].String() < l[j].String() }

func (s *S) TestRebuildRoutesBetweenRouters(c *check.C) {
	a := app.App{Name: "my-test-app", TeamOwner: s.team.Name, Router: "none"}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	version := newVersion(c, &a)
	err = provisiontest.ProvisionerInstance.AddUnits(context.TODO(), &a, 1, "web", version, nil)
	c.Assert(err, check.IsNil)
	oldAddrs, err := a.GetAddresses(context.TODO())
	c.Assert(err, check.IsNil)
	a.Router = "fake"
	err = rebuild.RebuildRoutes(context.TODO(), rebuild.RebuildRoutesOpts{
		App: &a,
	})
	c.Assert(err, check.IsNil)
	newAddrs, err := a.GetAddresses(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(newAddrs, check.Not(check.DeepEquals), oldAddrs)
}

func (s *S) TestRebuildRoutesSetsHealthcheck(c *check.C) {
	a := app.App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
		App: &a,
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path":          "/healthcheck",
			"status":        http.StatusFound,
			"use_in_router": true,
		},
	}
	err = version.AddData(appTypes.AddVersionDataArgs{
		CustomData: customData,
	})
	c.Assert(err, check.IsNil)
	err = version.CommitSuccessful()
	c.Assert(err, check.IsNil)
	err = rebuild.RebuildRoutes(context.TODO(), rebuild.RebuildRoutesOpts{
		App: &a,
	})
	c.Assert(err, check.IsNil)
	expected := routerTypes.HealthcheckData{
		Path:   "/healthcheck",
		Status: 302,
	}
	c.Assert(routertest.FakeRouter.GetHealthcheck("my-test-app"), check.DeepEquals, expected)
}
