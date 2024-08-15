// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rebuild_test

import (
	"context"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/servicemanager"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/types/quota"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	user        *auth.User
	team        *authTypes.Team
	mockService servicemock.MockService
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "router_rebuild_tests")
	config.Set("routers:fake:type", "fake")
	config.Set("routers:fake:default", true)
	config.Set("docker:router", "fake")
	config.Set("auth:hash-cost", bcrypt.MinCost)
	provision.DefaultProvisioner = "fake"

	storagev2.Reset()
}

func (s *S) TearDownSuite(c *check.C) {
	storagev2.ClearAllCollections(nil)
}

func (s *S) SetUpTest(c *check.C) {
	rebuild.Initialize(func(appName string) (rebuild.RebuildApp, error) {
		a, err := app.GetByName(context.TODO(), appName)
		if err == appTypes.ErrAppNotFound {
			return nil, nil
		}
		return a, err
	})
	routertest.FakeRouter.Reset()
	provisiontest.ProvisionerInstance.Reset()
	err := storagev2.ClearAllCollections(nil)
	c.Assert(err, check.IsNil)
	s.user = &auth.User{Email: "myadmin@arrakis.com", Password: "123456", Quota: quota.UnlimitedQuota}
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	_, err = nativeScheme.Create(context.TODO(), s.user)
	c.Assert(err, check.IsNil)
	s.team = &authTypes.Team{Name: "admin"}
	err = pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "p1",
		Default:     true,
		Provisioner: "fake",
	})
	c.Assert(err, check.IsNil)
	servicemock.SetMockService(&s.mockService)
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{*s.team}, nil
	}
	s.mockService.Team.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return s.team, nil
	}
	plan := appTypes.Plan{
		Name:    "default",
		Default: true,
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{plan}, nil
	}
	s.mockService.Plan.OnDefaultPlan = func() (*appTypes.Plan, error) {
		return &plan, nil
	}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		if name == plan.Name {
			return &plan, nil
		}
		return nil, appTypes.ErrPlanNotFound
	}
	servicemanager.AppVersion, err = version.AppVersionService()
	c.Assert(err, check.IsNil)
}
