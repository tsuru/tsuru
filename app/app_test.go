// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/globalsign/mgo/bson"
	pkgErrors "github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	tsuruEnvs "github.com/tsuru/tsuru/envs"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/safe"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	tsuruTest "github.com/tsuru/tsuru/test"
	"github.com/tsuru/tsuru/tsurutest"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	"github.com/tsuru/tsuru/types/cache"
	"github.com/tsuru/tsuru/types/quota"
	routerTypes "github.com/tsuru/tsuru/types/router"
	volumeTypes "github.com/tsuru/tsuru/types/volume"
	check "gopkg.in/check.v1"
)

func (s *S) TestGetAppByName(c *check.C) {
	newApp := App{Name: "my-app", Platform: "Django", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &newApp, s.user)
	c.Assert(err, check.IsNil)
	newApp.Env = map[string]bindTypes.EnvVar{}
	err = s.conn.Apps().Update(bson.M{"name": newApp.Name}, &newApp)
	c.Assert(err, check.IsNil)
	myApp, err := GetByName(context.TODO(), "my-app")
	c.Assert(err, check.IsNil)
	c.Assert(myApp.Name, check.Equals, newApp.Name)
}

func (s *S) TestGetAppByNameNotFound(c *check.C) {
	app, err := GetByName(context.TODO(), "wat")
	c.Assert(err, check.Equals, appTypes.ErrAppNotFound)
	c.Assert(app, check.IsNil)
}

func (s *S) TestDelete(c *check.C) {
	a := App{
		Name:      "ritual",
		Platform:  "ruby",
		Owner:     s.user.Email,
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	app, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	err = servicemanager.LogService.Add(a.Name, "msg", "src", "unit")
	c.Assert(err, check.IsNil)
	version, err := servicemanager.AppVersion.NewAppVersion(context.TODO(), appTypes.NewVersionArgs{
		App: app,
	})
	c.Assert(err, check.IsNil)
	err = version.CommitBaseImage()
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDelete,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	err = Delete(context.TODO(), app, evt, "")
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasBackend(app.Name), check.Equals, false)
	_, err = GetByName(context.TODO(), app.Name)
	c.Assert(err, check.Equals, appTypes.ErrAppNotFound)
	c.Assert(s.provisioner.Provisioned(&a), check.Equals, false)
	err = servicemanager.UserQuota.Inc(context.TODO(), s.user, 1)
	c.Assert(err, check.IsNil)
	appVersion, err := servicemanager.AppVersion.AppVersions(context.TODO(), app)
	c.Assert(err, check.IsNil)
	c.Assert(appVersion.Count, check.Not(check.Equals), 0)
	c.Assert(appVersion.Versions, check.DeepEquals, map[int]appTypes.AppVersionInfo{})
}

func (s *S) TestDeleteVersion(c *check.C) {
	a := App{
		Name:      "ritual",
		Platform:  "ruby",
		Owner:     s.user.Email,
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	app, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	err = servicemanager.LogService.Add(a.Name, "msg", "src", "unit")
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, app)
	version2 := newSuccessfulAppVersion(c, app)
	appVersions, err := servicemanager.AppVersion.AppVersions(context.TODO(), app)
	c.Assert(err, check.IsNil)
	c.Assert(appVersions.Count, check.Equals, 2)
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppUpdate,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	err = a.DeleteVersion(context.TODO(), evt, strconv.Itoa(version2.Version()))
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.Provisioned(&a), check.Equals, false)
	err = servicemanager.UserQuota.Inc(context.TODO(), s.user, 1)
	c.Assert(err, check.IsNil)
}

func (s *S) TestDeleteAppWithNoneRouters(c *check.C) {
	a := App{
		Name:      "myapp",
		Platform:  "go",
		Owner:     s.user.Email,
		TeamOwner: s.team.Name,
		Router:    "none",
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDelete,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	err = Delete(context.TODO(), &a, evt, "")
	c.Assert(err, check.IsNil)
}

func (s *S) TestDeleteWithBoundVolumes(c *check.C) {
	a := App{
		Name:      "ritual",
		Platform:  "ruby",
		Owner:     s.user.Email,
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	v1 := volumeTypes.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volumeTypes.VolumePlan{Name: "nfs"}}
	err = servicemanager.Volume.Create(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v1,
		AppName:    a.Name,
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	c.Assert(err, check.IsNil)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v1,
		AppName:    a.Name,
		MountPoint: "/mnt2",
		ReadOnly:   false,
	})
	c.Assert(err, check.IsNil)
	app, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDelete,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	err = Delete(context.TODO(), app, evt, "")
	c.Assert(err, check.IsNil)
	dbV, err := servicemanager.Volume.Get(context.TODO(), v1.Name)
	c.Assert(err, check.IsNil)
	binds, err := servicemanager.Volume.Binds(context.TODO(), dbV)
	c.Assert(err, check.IsNil)
	c.Assert(binds, check.IsNil)
}

func (s *S) TestCreateApp(c *check.C) {
	a := App{
		Name:      "appname",
		Platform:  "python",
		TeamOwner: s.team.Name,
		Tags:      []string{"", " test a  ", "  ", "test b ", " test a "},
	}
	var teamQuotaIncCalled bool
	s.mockService.TeamQuota.OnInc = func(item quota.QuotaItem, q int) error {
		teamQuotaIncCalled = true
		c.Assert(item.GetName(), check.Equals, s.team.Name)
		return nil
	}
	var userQuotaIncCalled bool
	s.mockService.UserQuota.OnInc = func(item quota.QuotaItem, q int) error {
		userQuotaIncCalled = true
		c.Assert(item.GetName(), check.Equals, s.user.Email)
		return nil
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": 1}})
	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	c.Assert(teamQuotaIncCalled, check.Equals, true)
	c.Assert(userQuotaIncCalled, check.Equals, true)
	retrievedApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(retrievedApp.Name, check.Equals, a.Name)
	c.Assert(retrievedApp.Platform, check.Equals, a.Platform)
	c.Assert(retrievedApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(retrievedApp.Owner, check.Equals, s.user.Email)
	c.Assert(retrievedApp.Tags, check.DeepEquals, []string{"test a", "test b"})
	env := retrievedApp.Envs()
	c.Assert(env["TSURU_APPNAME"].Value, check.Equals, a.Name)
	c.Assert(env["TSURU_APPNAME"].Public, check.Equals, false)
}

func (s *S) TestCreateAppAlreadyExists(c *check.C) {
	a := App{
		Name:      "appname",
		Platform:  "python",
		TeamOwner: s.team.Name,
		Tags:      []string{"", " test a  ", "  ", "test b ", " test a "},
	}
	s.mockService.UserQuota.OnInc = func(item quota.QuotaItem, q int) error {
		c.Assert(item.GetName(), check.Equals, s.user.Email)
		return nil
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": 1}})
	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	ra := App{Name: "appname", Platform: "python", TeamOwner: s.team.Name, Pool: "invalid"}
	err = CreateApp(context.TODO(), &ra, s.user)
	c.Assert(err, check.DeepEquals, &appTypes.AppCreationError{App: ra.Name, Err: ErrAppAlreadyExists})
}

func (s *S) TestCreateAppDefaultPlan(c *check.C) {
	a := App{
		Name:      "appname",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": 1}})
	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	retrievedApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(retrievedApp.Plan, check.DeepEquals, s.defaultPlan)
}

func (s *S) TestCreateAppDefaultRouterForPool(c *check.C) {
	pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr: "pool1",
		Field:    pool.ConstraintTypeRouter,
		Values:   []string{"fake-tls", "fake"},
	})
	a := App{
		Name:      "appname",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": 1}})
	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	retrievedApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(retrievedApp.GetRouters(), check.DeepEquals, []appTypes.AppRouter{{Name: "fake-tls", Opts: map[string]string{}}})
}

func (s *S) TestCreateAppDefaultPlanForPool(c *check.C) {
	s.plan = appTypes.Plan{Name: "large", Memory: 4194304}
	pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr: "pool1",
		Field:    pool.ConstraintTypePlan,
		Values:   []string{"large", "huge"},
	})
	a := App{
		Name:      "appname",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": 1}})
	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	retrievedApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(retrievedApp.Plan.Name, check.Equals, "large")
}

func (s *S) TestCreateAppDefaultPlanWildCardForPool(c *check.C) {
	s.plan = appTypes.Plan{Name: "large", Memory: 4194304}
	pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr: "pool1",
		Field:    pool.ConstraintTypePlan,
		Values:   []string{"l*", "huge"},
	})
	a := App{
		Name:      "appname",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": 1}})
	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	retrievedApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(retrievedApp.Plan.Name, check.Equals, "large")
}

func (s *S) TestCreateAppDefaultPlanWildCardNotMatchForPoolReturnError(c *check.C) {
	s.plan = appTypes.Plan{Name: "large", Memory: 4194304}
	pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr: "pool1",
		Field:    pool.ConstraintTypePlan,
		Values:   []string{"blah*"},
	})
	a := App{
		Name:      "appname",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": 1}})
	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err.Error(), check.Equals, "no plan found for pool")
}

func (s *S) TestCreateAppDefaultPlanWildCardDefaultPlan(c *check.C) {
	s.plan = appTypes.Plan{Name: "large", Memory: 4194304}
	pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr: "pool1",
		Field:    pool.ConstraintTypePlan,
		Values:   []string{"*"},
	})
	a := App{
		Name:      "appname",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": 1}})
	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	retrievedApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(retrievedApp.Plan.Name, check.Equals, "default-plan")
}

func (s *S) TestCreateAppWithExplicitPlan(c *check.C) {
	myPlan := appTypes.Plan{
		Name:   "myplan",
		Memory: 4194304,
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{s.defaultPlan, myPlan}, nil
	}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		c.Assert(name, check.Equals, myPlan.Name)
		return &myPlan, nil
	}
	a := App{
		Name:      "appname",
		Platform:  "python",
		Plan:      appTypes.Plan{Name: "myplan"},
		TeamOwner: s.team.Name,
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": 1}})
	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	retrievedApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(retrievedApp.Plan, check.DeepEquals, myPlan)
}

func (s *S) TestCreateAppWithExplicitPlanConstraint(c *check.C) {
	myPlan := appTypes.Plan{
		Name:   "myplan",
		Memory: 4194304,
	}
	err := pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr:  "pool1",
		Field:     pool.ConstraintTypePlan,
		Values:    []string{myPlan.Name},
		Blacklist: true,
	})
	c.Assert(err, check.IsNil)
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{s.defaultPlan, s.plan, myPlan}, nil
	}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		if name == s.defaultPlan.Name {
			return &s.defaultPlan, nil
		}
		if s.plan.Name == name {
			return &s.plan, nil
		}
		if name == "myplan" {
			return &myPlan, nil
		}
		return nil, appTypes.ErrPlanNotFound
	}
	a := App{
		Name:      "appname",
		Platform:  "python",
		Plan:      appTypes.Plan{Name: "myplan"},
		TeamOwner: s.team.Name,
	}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.ErrorMatches, `App plan "myplan" is not allowed on pool "pool1"`)
}

func (s *S) TestCreateAppUserQuotaExceeded(c *check.C) {
	app := App{Name: "america", Platform: "python", TeamOwner: s.team.Name}
	s.conn.Users().Update(
		bson.M{"email": s.user.Email},
		bson.M{"$set": bson.M{"quota.limit": 1, "quota.inuse": 1}},
	)
	s.mockService.UserQuota.OnInc = func(item quota.QuotaItem, q int) error {
		c.Assert(item.GetName(), check.Equals, s.user.Email)
		return &quota.QuotaExceededError{Available: 0, Requested: 1}
	}
	err := CreateApp(context.TODO(), &app, s.user)
	e, ok := err.(*appTypes.AppCreationError)
	c.Assert(ok, check.Equals, true)
	qe, ok := e.Err.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(qe.Available, check.Equals, uint(0))
	c.Assert(qe.Requested, check.Equals, uint(1))
}

func (s *S) TestCreteAppTeamQuotaExceeded(c *check.C) {
	a := App{Name: "my-app", Platform: "python", TeamOwner: "my-team"}
	t := authTypes.Team{Name: a.TeamOwner, Quota: quota.Quota{InUse: 10, Limit: 10}}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{s.team, t}, nil
	}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		if name != a.TeamOwner {
			return nil, stderrors.New("team not found")
		}
		return &t, nil
	}
	var teamQuotaIncCalled bool
	s.mockService.TeamQuota.OnInc = func(item quota.QuotaItem, delta int) error {
		teamQuotaIncCalled = true
		c.Assert(item.GetName(), check.Equals, a.TeamOwner)
		c.Assert(delta, check.Equals, 1)
		return &quota.QuotaExceededError{Available: 0, Requested: 1}
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.NotNil)
	c.Assert(teamQuotaIncCalled, check.Equals, true)
	e, ok := err.(*appTypes.AppCreationError)
	c.Assert(ok, check.Equals, true)
	qe, ok := e.Err.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(qe.Available, check.Equals, uint(0))
	c.Assert(qe.Requested, check.Equals, uint(1))
}

func (s *S) TestCreateAppTeamOwner(c *check.C) {
	app := App{Name: "america", Platform: "python", TeamOwner: "tsuruteam"}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	c.Assert(app.TeamOwner, check.Equals, "tsuruteam")
}

func (s *S) TestCreateAppTeamOwnerTeamNotFound(c *check.C) {
	app := App{
		Name:      "someapp",
		Platform:  "python",
		TeamOwner: "not found",
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "team not found")
}

func (s *S) TestCannotCreateAppWithoutTeamOwner(c *check.C) {
	u := auth.User{Email: "perpetual@yes.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	a := App{Name: "beyond"}
	err = CreateApp(context.TODO(), &a, &u)
	c.Check(err, check.DeepEquals, &errors.ValidationError{Message: authTypes.ErrTeamNotFound.Error()})
}

func (s *S) TestCantCreateTwoAppsWithTheSameName(c *check.C) {
	err := CreateApp(context.TODO(), &App{Name: "appname", TeamOwner: s.team.Name}, s.user)
	c.Assert(err, check.IsNil)
	a := App{Name: "appname", Platform: "python", TeamOwner: s.team.Name}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.NotNil)
	e, ok := err.(*appTypes.AppCreationError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.App, check.Equals, "appname")
	c.Assert(e.Err, check.NotNil)
	c.Assert(e.Err.Error(), check.Equals, "there is already an app with this name")
}

func (s *S) TestCantCreateAppWithInvalidName(c *check.C) {
	a := App{
		Name:      "1123app",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.ValidationError)
	c.Assert(ok, check.Equals, true)
	msg := "Invalid app name, your app should have at most 40 " +
		"characters, containing only lower case letters, numbers or dashes, " +
		"starting with a letter."
	c.Assert(e.Message, check.Equals, msg)
}

func (s *S) TestCreateAppProvisionerFailures(c *check.C) {
	s.provisioner.PrepareFailure("Provision", fmt.Errorf("exit status 1"))
	a := App{
		Name:      "theirapp",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.NotNil)
	expected := `tsuru failed to create the app "theirapp": exit status 1`
	c.Assert(err.Error(), check.Equals, expected)
	_, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.NotNil)
}

func (s *S) TestCreateAppUserFromTsuruToken(c *check.C) {
	s.mockService.UserQuota.OnInc = func(item quota.QuotaItem, delta int) error {
		return stderrors.New("cannot be called")
	}
	user := auth.User{
		Email:     "my-token@tsuru-team-token",
		Quota:     quota.UnlimitedQuota,
		FromToken: true,
	}
	a := App{
		Name:      "my-app",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, &user)
	c.Assert(err, check.IsNil)
}

func (s *S) TestAddUnits(c *check.C) {
	app := App{
		Name: "warpaint", Platform: "python",
		Quota:     quota.UnlimitedQuota,
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app)
	err = app.AddUnits(5, "web", "", nil)
	c.Assert(err, check.IsNil)
	units, err := app.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 5)
	err = app.AddUnits(2, "worker", "", nil)
	c.Assert(err, check.IsNil)
	units, err = app.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 7)
	for i, unit := range units {
		c.Assert(unit.AppName, check.Equals, app.Name)
		if i < 5 {
			c.Assert(unit.ProcessName, check.Equals, "web")
		} else {
			c.Assert(unit.ProcessName, check.Equals, "worker")
		}
	}
}

func (s *S) TestAddUnitsInStoppedApp(c *check.C) {
	a := App{
		Name: "sejuani", Platform: "python",
		TeamOwner: s.team.Name,
		Quota:     quota.UnlimitedQuota,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	err = a.AddUnits(1, "web", "", nil)
	c.Assert(err, check.IsNil)
	err = a.Stop(context.TODO(), nil, "web", "")
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", "", nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot add units to an app that has stopped units")
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
}

func (s *S) TestAddUnitsWithWriter(c *check.C) {
	app := App{
		Name: "warpaint", Platform: "python",
		Quota:     quota.UnlimitedQuota,
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	newSuccessfulAppVersion(c, &app)
	err = app.AddUnits(2, "web", "", &buf)
	c.Assert(err, check.IsNil)
	units, err := app.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 2)
	for _, unit := range units {
		c.Assert(unit.AppName, check.Equals, app.Name)
	}
	c.Assert(buf.String(), check.Matches, "(?s)added 2 units.*")
}

func (s *S) TestAddUnitsQuota(c *check.C) {
	app := App{
		Name: "warpaint", Platform: "python",
		TeamOwner: s.team.Name, Quota: quota.Quota{Limit: 7, InUse: 0},
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	var inUseNow int
	s.mockService.AppQuota.OnInc = func(item quota.QuotaItem, quantity int) error {
		inUseNow++
		c.Assert(item.GetName(), check.Equals, app.Name)
		c.Assert(quantity, check.Equals, 1)
		return nil
	}
	newSuccessfulAppVersion(c, &app)
	otherApp := App{Name: "warpaint"}
	for i := 1; i <= 7; i++ {
		err = otherApp.AddUnits(1, "web", "", nil)
		c.Assert(err, check.IsNil)
		c.Assert(inUseNow, check.Equals, i)
		units := s.provisioner.GetUnits(&app)
		c.Assert(units, check.HasLen, i)
	}
}

func (s *S) TestAddUnitsQuotaExceeded(c *check.C) {
	app := App{
		Name: "warpaint", Platform: "ruby",
		TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake"}},
		Quota: quota.Quota{Limit: 7, InUse: 7},
	}
	s.mockService.AppQuota.OnInc = func(item quota.QuotaItem, quantity int) error {
		c.Assert(item.GetName(), check.Equals, app.Name)
		c.Assert(quantity, check.Equals, 1)
		return &quota.QuotaExceededError{Available: 0, Requested: 1}
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app)
	err = app.AddUnits(1, "web", "", nil)
	e, ok := pkgErrors.Cause(err).(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Available, check.Equals, uint(0))
	c.Assert(e.Requested, check.Equals, uint(1))
	units := s.provisioner.GetUnits(&app)
	c.Assert(units, check.HasLen, 0)
}

func (s *S) TestAddUnitsMultiple(c *check.C) {
	app := App{
		Name: "warpaint", Platform: "ruby",
		TeamOwner: s.team.Name,
		Quota:     quota.Quota{Limit: 11, InUse: 0},
	}
	s.mockService.AppQuota.OnInc = func(item quota.QuotaItem, quantity int) error {
		c.Assert(item.GetName(), check.Equals, app.Name)
		c.Assert(quantity, check.Equals, 10)
		return nil
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app)
	err = app.AddUnits(10, "web", "", nil)
	c.Assert(err, check.IsNil)
	units := s.provisioner.GetUnits(&app)
	c.Assert(units, check.HasLen, 10)
}

func (s *S) TestAddZeroUnits(c *check.C) {
	app := App{Name: "warpaint", Platform: "ruby"}
	err := app.AddUnits(0, "web", "", nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot add zero units.")
}

func (s *S) TestAddUnitsFailureInProvisioner(c *check.C) {
	app := App{
		Name:      "scars",
		Platform:  "golang",
		Quota:     quota.UnlimitedQuota,
		TeamOwner: s.team.Name,
		Routers:   []appTypes.AppRouter{{Name: "fake"}},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app)
	err = app.AddUnits(2, "web", "", nil)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "(?s).*App is not provisioned.*")
}

func (s *S) TestAddUnitsIsAtomic(c *check.C) {
	app := App{
		Name: "warpaint", Platform: "golang",
		Quota: quota.UnlimitedQuota,
	}
	err := app.AddUnits(2, "web", "", nil)
	c.Assert(err, check.NotNil)
	_, err = GetByName(context.TODO(), app.Name)
	c.Assert(err, check.Equals, appTypes.ErrAppNotFound)
}

func (s *S) TestRemoveUnitsWithQuota(c *check.C) {
	a := App{
		Name:      "ble",
		TeamOwner: s.team.Name,
	}
	s.mockService.AppQuota.OnSetLimit = func(item quota.QuotaItem, quantity int) error {
		c.Assert(item.GetName(), check.Equals, a.Name)
		c.Assert(quantity, check.Equals, 6)
		return nil
	}
	s.mockService.AppQuota.OnInc = func(item quota.QuotaItem, quantity int) error {
		c.Assert(item.GetName(), check.Equals, a.Name)
		c.Assert(quantity, check.Equals, 4)
		return nil
	}
	s.mockService.AppQuota.OnGet = func(item quota.QuotaItem) (*quota.Quota, error) {
		c.Assert(item.GetName(), check.Equals, a.Name)
		return &quota.Quota{Limit: 6, InUse: 2}, nil
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetQuotaLimit(6)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 6, "web", newSuccessfulAppVersion(c, &a), nil)
	err = a.RemoveUnits(context.TODO(), 4, "web", "", nil)
	c.Assert(err, check.IsNil)
	quota, err := servicemanager.AppQuota.Get(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	err = tsurutest.WaitCondition(2e9, func() bool {
		return quota.InUse == 2
	})
	c.Assert(err, check.IsNil)
}

func (s *S) TestRemoveUnits(c *check.C) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := service.Create(srvc)
	c.Assert(err, check.IsNil)
	app := App{
		Name:      "chemistry",
		Platform:  "python",
		Quota:     quota.UnlimitedQuota,
		TeamOwner: s.team.Name,
	}
	instance := service.ServiceInstance{
		Name:        "my-inst",
		ServiceName: srvc.Name,
		Teams:       []string{s.team.Name},
		Apps:        []string{app.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app)
	err = app.AddUnits(2, "web", "", nil)
	c.Assert(err, check.IsNil)
	err = app.AddUnits(2, "worker", "", nil)
	c.Assert(err, check.IsNil)
	err = app.AddUnits(2, "web", "", nil)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(nil)
	err = app.RemoveUnits(context.TODO(), 2, "worker", "", buf)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Matches, "(?s)removing 2 units.*")
	err = tsurutest.WaitCondition(2e9, func() bool {
		units, inErr := app.Units()
		c.Assert(inErr, check.IsNil)
		return len(units) == 4
	})
	c.Assert(err, check.IsNil)
	ts.Close()
	units, err := app.Units()
	c.Assert(err, check.IsNil)
	for _, unit := range units {
		c.Assert(unit.ProcessName, check.Equals, "web")
	}
}

func (s *S) TestRemoveUnitsInvalidValues(c *check.C) {
	var tests = []struct {
		n        uint
		expected string
	}{
		{0, "cannot remove 0 units"},
		{4, "too many units to remove"},
	}
	app := App{
		Name:      "chemistryii",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &app, 3, "web", newSuccessfulAppVersion(c, &app), nil)
	for _, test := range tests {
		err := app.RemoveUnits(context.TODO(), test.n, "web", "", nil)
		c.Check(err, check.ErrorMatches, "(?s).*"+test.expected+".*")
	}
}

func (s *S) TestGrantAccessFailsIfTheTeamAlreadyHasAccessToTheApp(c *check.C) {
	a := App{Name: "app-name", Platform: "django", Teams: []string{s.team.Name}}
	err := a.Grant(&s.team)
	c.Assert(err, check.Equals, ErrAlreadyHaveAccess)
}

func (s *S) TestRevokeAccessDoesntLeaveOrphanApps(c *check.C) {
	app := App{Name: "app-name", Platform: "django", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	err = app.Revoke(&s.team)
	c.Assert(err, check.Equals, ErrCannotOrphanApp)
}

func (s *S) TestRevokeAccessFailsIfTheTeamsDoesNotHaveAccessToTheApp(c *check.C) {
	a := App{Name: "app-name", Platform: "django", Teams: []string{}}
	err := a.Revoke(&s.team)
	c.Assert(err, check.Equals, ErrNoAccess)
}

func (s *S) TestSetEnvNewAppsTheMapIfItIsNil(c *check.C) {
	a := App{Name: "how-many-more-times"}
	c.Assert(a.Env, check.IsNil)
	env := bindTypes.EnvVar{Name: "PATH", Value: "/"}
	a.setEnv(env)
	c.Assert(a.Env, check.NotNil)
}

func (s *S) TestSetPublicEnvironmentVariableToApp(c *check.C) {
	a := App{Name: "app-name", Platform: "django"}
	a.setEnv(bindTypes.EnvVar{Name: "PATH", Value: "/", Public: true})
	env := a.Env["PATH"]
	c.Assert(env.Name, check.Equals, "PATH")
	c.Assert(env.Value, check.Equals, "/")
	c.Assert(env.Public, check.Equals, true)
}

func (s *S) TestSetPrivateEnvironmentVariableToApp(c *check.C) {
	a := App{Name: "app-name", Platform: "django"}
	a.setEnv(bindTypes.EnvVar{Name: "PATH", Value: "/", Public: false})
	env := a.Env["PATH"]
	c.Assert(env.Name, check.Equals, "PATH")
	c.Assert(env.Value, check.Equals, "/")
	c.Assert(env.Public, check.Equals, false)
}

func (s *S) TestSetMultiplePublicEnvironmentVariableToApp(c *check.C) {
	a := App{Name: "app-name", Platform: "django"}
	a.setEnv(bindTypes.EnvVar{Name: "PATH", Value: "/", Public: true})
	a.setEnv(bindTypes.EnvVar{Name: "DATABASE", Value: "mongodb", Public: true})
	env := a.Env["PATH"]
	c.Assert(env.Name, check.Equals, "PATH")
	c.Assert(env.Value, check.Equals, "/")
	c.Assert(env.Public, check.Equals, true)
	env = a.Env["DATABASE"]
	c.Assert(env.Name, check.Equals, "DATABASE")
	c.Assert(env.Value, check.Equals, "mongodb")
	c.Assert(env.Public, check.Equals, true)
}

func (s *S) TestSetMultiplePrivateEnvironmentVariableToApp(c *check.C) {
	a := App{Name: "app-name", Platform: "django"}
	a.setEnv(bindTypes.EnvVar{Name: "PATH", Value: "/", Public: false})
	a.setEnv(bindTypes.EnvVar{Name: "DATABASE", Value: "mongodb", Public: false})
	env := a.Env["PATH"]
	c.Assert(env.Name, check.Equals, "PATH")
	c.Assert(env.Value, check.Equals, "/")
	c.Assert(env.Public, check.Equals, false)
	env = a.Env["DATABASE"]
	c.Assert(env.Name, check.Equals, "DATABASE")
	c.Assert(env.Value, check.Equals, "mongodb")
	c.Assert(env.Public, check.Equals, false)
}

func (s *S) TestSetEnvKeepServiceVariables(c *check.C) {
	a := App{
		Name: "myapp",
		ServiceEnvs: []bindTypes.ServiceEnvVar{
			{
				EnvVar: bindTypes.EnvVar{
					Name:   "DATABASE_HOST",
					Value:  "localhost",
					Public: false,
				},
				InstanceName: "instance",
				ServiceName:  "service",
			},
		},
		TeamOwner: s.team.Name,
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	err = a.AddUnits(1, "web", "", nil)
	c.Assert(err, check.IsNil)
	envs := []bindTypes.EnvVar{
		{
			Name:   "DATABASE_HOST",
			Value:  "remotehost",
			Public: false,
		},
		{
			Name:   "DATABASE_PASSWORD",
			Value:  "123",
			Public: true,
		},
	}
	var buf bytes.Buffer
	err = a.SetEnvs(bind.SetEnvArgs{
		Envs:          envs,
		ShouldRestart: true,
		Writer:        &buf,
	})
	c.Assert(err, check.ErrorMatches, "Environment variable \"DATABASE_HOST\" is already in use by service bind \"service/instance\"")
	newApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	expected := map[string]bindTypes.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "localhost",
			Public: false,
		},
	}
	newAppEnvs := newApp.Envs()
	delete(newAppEnvs, tsuruEnvs.TsuruServicesEnvVar)
	delete(newAppEnvs, "TSURU_APPNAME")
	delete(newAppEnvs, "TSURU_APPDIR")

	c.Assert(newAppEnvs, check.DeepEquals, expected)
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 0)
	c.Assert(buf.String(), check.Equals, "---- Setting 2 new environment variables ----\n---- environment variables have conflicts with service binds: Environment variable \"DATABASE_HOST\" is already in use by service bind \"service/instance\" ----\n")
}

func (s *S) TestSetEnvWithNoRestartFlag(c *check.C) {
	a := App{
		Name: "myapp",
		Env: map[string]bindTypes.EnvVar{
			"DATABASE_HOST": {
				Name:   "DATABASE_HOST",
				Value:  "localhost",
				Public: false,
			},
		},
		TeamOwner: s.team.Name,
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	envs := []bindTypes.EnvVar{
		{
			Name:   "DATABASE_HOST",
			Value:  "remotehost",
			Public: true,
		},
		{
			Name:   "DATABASE_PASSWORD",
			Value:  "123",
			Public: true,
		},
	}
	err = a.SetEnvs(bind.SetEnvArgs{
		Envs:          envs,
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	newApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	expected := map[string]bindTypes.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "remotehost",
			Public: true,
		},
		"DATABASE_PASSWORD": {
			Name:   "DATABASE_PASSWORD",
			Value:  "123",
			Public: true,
		},
	}
	c.Assert(newApp.Env, check.DeepEquals, expected)
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 0)
}

func (s *S) TestSetEnvsWhenAppHaveNoUnits(c *check.C) {
	a := App{
		Name: "myapp",
		Env: map[string]bindTypes.EnvVar{
			"DATABASE_HOST": {
				Name:   "DATABASE_HOST",
				Value:  "localhost",
				Public: false,
			},
		},
		TeamOwner: s.team.Name,
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	envs := []bindTypes.EnvVar{
		{
			Name:   "DATABASE_HOST",
			Value:  "remotehost",
			Public: true,
		},
		{
			Name:   "DATABASE_PASSWORD",
			Value:  "123",
			Public: true,
		},
	}
	err = a.SetEnvs(bind.SetEnvArgs{
		Envs:          envs,
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	newApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	expected := map[string]bindTypes.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "remotehost",
			Public: true,
		},
		"DATABASE_PASSWORD": {
			Name:   "DATABASE_PASSWORD",
			Value:  "123",
			Public: true,
		},
	}
	c.Assert(newApp.Env, check.DeepEquals, expected)
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 0)
}

func (s *S) TestSetEnvsValidation(c *check.C) {
	a := App{
		Name:      "myapp",
		TeamOwner: s.team.Name,
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	var tests = []struct {
		envName string
		isValid bool
	}{
		{"VALID_ENV", true},
		{"ENV123", true},
		{"lowcase", true},
		{"ENV-WITH-DASHES", true},
		{"-NO_LEADING_DASH", false},
		{"_NO_LEADING_UNDERSCORE", false},
		{"ENV.WITH.DOTS", false},
		{"ENV VAR WITH SPACES", false},
		{"0NO_LEADING_NUMBER", false},
	}
	for _, test := range tests {
		envs := []bindTypes.EnvVar{
			{
				Name:  test.envName,
				Value: "any value",
			},
		}
		err = a.SetEnvs(bind.SetEnvArgs{Envs: envs})
		if test.isValid {
			c.Check(err, check.IsNil)
		} else {
			c.Check(err, check.ErrorMatches, fmt.Sprintf("Invalid environment variable name: '%s'", test.envName))
		}
	}
}

func (s *S) TestUnsetEnvKeepServiceVariables(c *check.C) {
	a := App{
		Name: "myapp",
		Env: map[string]bindTypes.EnvVar{
			"DATABASE_PASSWORD": {
				Name:   "DATABASE_PASSWORD",
				Value:  "123",
				Public: false,
			},
		},
		ServiceEnvs: []bindTypes.ServiceEnvVar{
			{
				EnvVar: bindTypes.EnvVar{
					Name:   "DATABASE_HOST",
					Value:  "localhost",
					Public: false,
				},
				ServiceName:  "s1",
				InstanceName: "si1",
			},
		},
		Quota:     quota.Quota{Limit: 10},
		TeamOwner: s.team.Name,
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	err = a.AddUnits(1, "web", "", nil)
	c.Assert(err, check.IsNil)
	err = a.UnsetEnvs(bind.UnsetEnvArgs{
		VariableNames: []string{"DATABASE_HOST", "DATABASE_PASSWORD"},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	newApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	expected := map[string]bindTypes.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "localhost",
			Public: false,
		},
	}
	newAppEnvs := newApp.Envs()
	delete(newAppEnvs, tsuruEnvs.TsuruServicesEnvVar)
	c.Assert(newAppEnvs, check.DeepEquals, expected)
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 1)
}

func (s *S) TestUnsetEnvWithNoRestartFlag(c *check.C) {
	a := App{
		Name: "myapp",
		Env: map[string]bindTypes.EnvVar{
			"DATABASE_HOST": {
				Name:   "DATABASE_HOST",
				Value:  "localhost",
				Public: false,
			},
			"DATABASE_PASSWORD": {
				Name:   "DATABASE_PASSWORD",
				Value:  "123",
				Public: true,
			},
		},
		Quota:     quota.Quota{Limit: 10},
		TeamOwner: s.team.Name,
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	err = a.AddUnits(1, "web", "", nil)
	c.Assert(err, check.IsNil)
	err = a.UnsetEnvs(bind.UnsetEnvArgs{
		VariableNames: []string{"DATABASE_HOST", "DATABASE_PASSWORD"},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	newApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(newApp.Env, check.DeepEquals, map[string]bindTypes.EnvVar{})
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 0)
}
func (s *S) TestUnsetEnvNoUnits(c *check.C) {
	a := App{
		Name: "myapp",
		Env: map[string]bindTypes.EnvVar{
			"DATABASE_HOST": {
				Name:   "DATABASE_HOST",
				Value:  "localhost",
				Public: false,
			},
			"DATABASE_PASSWORD": {
				Name:   "DATABASE_PASSWORD",
				Value:  "123",
				Public: true,
			},
		},
		TeamOwner: s.team.Name,
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = a.UnsetEnvs(bind.UnsetEnvArgs{
		VariableNames: []string{"DATABASE_HOST", "DATABASE_PASSWORD"},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	newApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(newApp.Env, check.DeepEquals, map[string]bindTypes.EnvVar{})
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 0)
}

func (s *S) TestInstanceEnvironmentReturnEnvironmentVariablesForTheServer(c *check.C) {
	envs := []bindTypes.ServiceEnvVar{
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, ServiceName: "srv1", InstanceName: "mysql"},
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root"}, ServiceName: "srv1", InstanceName: "mysql"},
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "postgresaddr"}, ServiceName: "srv2", InstanceName: "postgres"},
		{EnvVar: bindTypes.EnvVar{Name: "HOST", Value: "10.0.2.1"}, ServiceName: "srv3", InstanceName: "redis"},
	}
	expected := map[string]bindTypes.EnvVar{
		"DATABASE_HOST": {Name: "DATABASE_HOST", Value: "localhost"},
		"DATABASE_USER": {Name: "DATABASE_USER", Value: "root"},
	}
	a := App{Name: "hi-there", ServiceEnvs: envs}
	c.Assert(a.InstanceEnvs("srv1", "mysql"), check.DeepEquals, expected)
}

func (s *S) TestInstanceEnvironmentDoesNotPanicIfTheEnvMapIsNil(c *check.C) {
	a := App{Name: "hi-there"}
	c.Assert(a.InstanceEnvs("srv1", "mysql"), check.DeepEquals, map[string]bindTypes.EnvVar{})
}

func (s *S) TestAddCName(c *check.C) {
	app := &App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	app, err = GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{"ktulu.mycompany.com"})
	err = app.AddCName("ktulu2.mycompany.com")
	c.Assert(err, check.IsNil)
	app, err = GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{"ktulu.mycompany.com", "ktulu2.mycompany.com"})
}

func (s *S) TestAddCNameCantBeDuplicatedWithSameRouter(c *check.C) {
	app := &App{Name: "ktulu", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake"}, {Name: "fake-tls"}}}
	err := CreateApp(context.TODO(), app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = app.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cname ktulu.mycompany.com already exists for this app")
	app2 := &App{Name: "ktulu2", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}}
	err = CreateApp(context.TODO(), app2, s.user)
	c.Assert(err, check.IsNil)
	err = app2.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cname ktulu.mycompany.com already exists for app ktulu using same router")
}

func (s *S) TestAddCNameErrorForDifferentTeamOwners(c *check.C) {
	app := &App{Name: "ktulu", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake"}}}
	app2 := &App{Name: "ktulu2", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}}
	err := CreateApp(context.TODO(), app, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), app2, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	app2.TeamOwner = "some-other-team"
	err = app2.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cname ktulu.mycompany.com already exists for another app ktulu and belongs to a different team owner")
}

func (s *S) TestAddCNameDifferentAppsNoRouter(c *check.C) {
	app := &App{Name: "ktulu", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{}}
	app2 := &App{Name: "ktulu2", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}}
	err := CreateApp(context.TODO(), app, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), app2, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = app2.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
}

func (s *S) TestAddCNameWithWildCard(c *check.C) {
	app := &App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddCName("*.mycompany.com")
	c.Assert(err, check.IsNil)
	app, err = GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{"*.mycompany.com"})
}

func (s *S) TestAddCNameErrsOnEmptyCName(c *check.C) {
	app := &App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddCName("")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Invalid cname")
}

func (s *S) TestAddCNameErrsOnInvalid(c *check.C) {
	app := &App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddCName("_ktulu.mycompany.com")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Invalid cname")
}

func (s *S) TestAddCNamePartialUpdate(c *check.C) {
	a := &App{Name: "master", Platform: "puppet", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	other := App{Name: a.Name, Routers: []appTypes.AppRouter{{Name: "fake"}}}
	err = other.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(a.Platform, check.Equals, "puppet")
	c.Assert(a.Name, check.Equals, "master")
	c.Assert(a.CName, check.DeepEquals, []string{"ktulu.mycompany.com"})
}

func (s *S) TestAddCNameUnknownApp(c *check.C) {
	a := App{Name: "ktulu", TeamOwner: s.team.Name}
	err := a.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.NotNil)
}

func (s *S) TestAddCNameValidatesTheCName(c *check.C) {
	var data = []struct {
		input string
		valid bool
	}{
		{"ktulu.mycompany.com", true},
		{"ktulu-super.mycompany.com", true},
		{"ktulu_super.mycompany.com", true},
		{"KTULU.MYCOMPANY.COM", true},
		{"ktulu", true},
		{"KTULU", true},
		{"http://ktulu.mycompany.com", false},
		{"http:ktulu.mycompany.com", false},
		{"/ktulu.mycompany.com", false},
		{".ktulu.mycompany.com", false},
		{"0800.com", true},
		{"-0800.com", false},
		{"", false},
	}
	a := App{Name: "live-to-die", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	for _, t := range data {
		err := a.AddCName(t.input)
		if !t.valid {
			c.Check(err.Error(), check.Equals, "Invalid cname")
		} else {
			c.Check(err, check.IsNil)
		}
	}
}

func (s *S) TestAddCNameCallsRouterSetCName(c *check.C) {
	a := App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddCName("ktulu.mycompany.com", "ktulu2.mycompany.com")
	c.Assert(err, check.IsNil)
	hasCName := routertest.FakeRouter.HasCNameFor(a.Name, "ktulu.mycompany.com")
	c.Assert(hasCName, check.Equals, true)
	hasCName = routertest.FakeRouter.HasCNameFor(a.Name, "ktulu2.mycompany.com")
	c.Assert(hasCName, check.Equals, true)
}

func (s *S) TestAddCnameRollbackWithDuplicatedCName(c *check.C) {
	a := App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = a.AddCName("ktulu2.mycompany.com", "ktulu3.mycompany.com", "ktulu.mycompany.com")
	c.Assert(err, check.NotNil)
	c.Assert(a.CName, check.DeepEquals, []string{"ktulu.mycompany.com"})
	hasCName := routertest.FakeRouter.HasCNameFor(a.Name, "ktulu2.mycompany.com")
	c.Assert(hasCName, check.Equals, false)
	hasCName = routertest.FakeRouter.HasCNameFor(a.Name, "ktulu3.mycompany.com")
	c.Assert(hasCName, check.Equals, false)
}

func (s *S) TestAddCnameRollbackWithInvalidCName(c *check.C) {
	a := App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddCName("ktulu2.mycompany.com")
	c.Assert(err, check.IsNil)
	invalidCName := "-------"
	err = a.AddCName("ktulu3.mycompany.com", invalidCName)
	c.Assert(err, check.NotNil)
	c.Assert(a.CName, check.DeepEquals, []string{"ktulu2.mycompany.com"})
	hasCName := routertest.FakeRouter.HasCNameFor(a.Name, "ktulu3.mycompany.com")
	c.Assert(hasCName, check.Equals, false)
}

func (s *S) TestAddCnameRollbackWithRouterFailure(c *check.C) {
	a1 := App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a1, s.user)
	c.Assert(err, check.IsNil)

	a2 := App{Name: "ktulu3", TeamOwner: s.team.Name}
	err = CreateApp(context.TODO(), &a2, s.user)
	c.Assert(err, check.IsNil)

	err = a1.AddCName("ktulu2.mycompany.com")
	c.Assert(err, check.IsNil)

	err = a2.AddCName("ktulu3.mycompany.com")
	c.Assert(err, check.IsNil)

	err = a1.AddCName("ktulu3.mycompany.com")
	c.Assert(err, check.ErrorMatches, "cname ktulu3.mycompany.com already exists for app ktulu3 using same router")
	c.Assert(a1.CName, check.DeepEquals, []string{"ktulu2.mycompany.com"})
	hasCName := routertest.FakeRouter.HasCNameFor(a1.Name, "ktulu3.mycompany.com")
	c.Assert(hasCName, check.Equals, false)
}

func (s *S) TestAddCnameRollbackWithDatabaseFailure(c *check.C) {
	a := App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddCName("ktulu2.mycompany.com")
	c.Assert(err, check.IsNil)
	s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = a.AddCName("ktulu3.mycompany.com")
	c.Assert(err, check.NotNil)
	hasCName := routertest.FakeRouter.HasCNameFor(a.Name, "ktulu3.mycompany.com")
	c.Assert(hasCName, check.Equals, false)
}

func (s *S) TestRemoveCNameRollback(c *check.C) {
	a := App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddCName("ktulu2.mycompany.com", "ktulu3.mycompany.com", "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = a.RemoveCName("ktulu2.mycompany.com", "test.com")
	c.Assert(err, check.NotNil)
	c.Assert(a.CName, check.DeepEquals, []string{"ktulu2.mycompany.com", "ktulu3.mycompany.com", "ktulu.mycompany.com"})
	hasCName := routertest.FakeRouter.HasCNameFor(a.Name, "ktulu2.mycompany.com")
	c.Assert(hasCName, check.Equals, true)

	routertest.FakeRouter.FailuresByHost["ktulu3.mycompany.com"] = true
	err = a.RemoveCName("ktulu2.mycompany.com")
	c.Assert(err, check.ErrorMatches, "Forced failure")
	c.Assert(a.CName, check.DeepEquals, []string{"ktulu2.mycompany.com", "ktulu3.mycompany.com", "ktulu.mycompany.com"})
}

func (s *S) TestRemoveCNameRemovesFromDatabase(c *check.C) {
	a := &App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = a.RemoveCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(a.CName, check.DeepEquals, []string{})
}

func (s *S) TestRemoveCNameWhichNoExists(c *check.C) {
	a := &App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = a.RemoveCName("ktulu.mycompany.com")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cname ktulu.mycompany.com not exists in app")
}

func (s *S) TestRemoveMoreThanOneCName(c *check.C) {
	a := &App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = a.AddCName("ktulu2.mycompany.com")
	c.Assert(err, check.IsNil)
	err = a.RemoveCName("ktulu.mycompany.com", "ktulu2.mycompany.com")
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(a.CName, check.DeepEquals, []string{})
}

func (s *S) TestRemoveCNameRemovesFromRouter(c *check.C) {
	a := App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = a.RemoveCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	hasCName := routertest.FakeRouter.HasCNameFor(a.Name, "ktulu.mycompany.com")
	c.Assert(hasCName, check.Equals, false)
}

func (s *S) TestAddInstanceFirst(c *check.C) {
	a := &App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "srv1"},
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_PORT", Value: "3306"}, InstanceName: "myinstance", ServiceName: "srv1"},
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root"}, InstanceName: "myinstance", ServiceName: "srv1"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	serviceEnv := allEnvs[tsuruEnvs.TsuruServicesEnvVar]
	c.Assert(serviceEnv.Name, check.Equals, tsuruEnvs.TsuruServicesEnvVar)
	c.Assert(serviceEnv.Public, check.Equals, false)
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(serviceEnv.Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{
		"srv1": []interface{}{
			map[string]interface{}{"instance_name": "myinstance", "envs": map[string]interface{}{
				"DATABASE_HOST": "localhost",
				"DATABASE_PORT": "3306",
				"DATABASE_USER": "root",
			}},
		},
	})
	delete(allEnvs, tsuruEnvs.TsuruServicesEnvVar)
	delete(allEnvs, "TSURU_APPDIR")
	delete(allEnvs, "TSURU_APPNAME")
	c.Assert(allEnvs, check.DeepEquals, map[string]bindTypes.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "localhost",
			Public: false,
		},
		"DATABASE_PORT": {
			Name:   "DATABASE_PORT",
			Value:  "3306",
			Public: false,
		},
		"DATABASE_USER": {
			Name:   "DATABASE_USER",
			Value:  "root",
			Public: false,
		},
	})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 0)
}

func (s *S) TestAddInstanceDuplicated(c *check.C) {
	a := &App{Name: "sith", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "ZMQ_PEER", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "srv1"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	// inserts duplicated
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "ZMQ_PEER", Value: "8.8.8.8"}, InstanceName: "myinstance", ServiceName: "srv1"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	serviceEnv := allEnvs[tsuruEnvs.TsuruServicesEnvVar]
	c.Assert(serviceEnv.Name, check.Equals, tsuruEnvs.TsuruServicesEnvVar)
	c.Assert(serviceEnv.Public, check.Equals, false)
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(serviceEnv.Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{
		"srv1": []interface{}{
			map[string]interface{}{"instance_name": "myinstance", "envs": map[string]interface{}{
				"ZMQ_PEER": "8.8.8.8",
			}},
		},
	})
	c.Assert(allEnvs["ZMQ_PEER"], check.DeepEquals, bindTypes.EnvVar{
		Name:   "ZMQ_PEER",
		Value:  "8.8.8.8",
		Public: false,
	})
}

func (s *S) TestAddInstanceWithUnits(c *check.C) {
	a := &App{Name: "dark", Quota: quota.Quota{Limit: 10}, TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, a)
	err = a.AddUnits(1, "web", "", nil)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "myservice"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	serviceEnv := allEnvs[tsuruEnvs.TsuruServicesEnvVar]
	c.Assert(serviceEnv.Name, check.Equals, tsuruEnvs.TsuruServicesEnvVar)
	c.Assert(serviceEnv.Public, check.Equals, false)
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(serviceEnv.Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{
		"myservice": []interface{}{
			map[string]interface{}{"instance_name": "myinstance", "envs": map[string]interface{}{
				"DATABASE_HOST": "localhost",
			}},
		},
	})
	c.Assert(allEnvs["DATABASE_HOST"], check.DeepEquals, bindTypes.EnvVar{
		Name:   "DATABASE_HOST",
		Value:  "localhost",
		Public: false,
	})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 1)
}

func (s *S) TestAddInstanceWithUnitsNoRestart(c *check.C) {
	a := &App{Name: "dark", Quota: quota.Quota{Limit: 10}, TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, a)
	err = a.AddUnits(1, "web", "", nil)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "myservice"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	serviceEnv := allEnvs[tsuruEnvs.TsuruServicesEnvVar]
	c.Assert(serviceEnv.Name, check.Equals, tsuruEnvs.TsuruServicesEnvVar)
	c.Assert(serviceEnv.Public, check.Equals, false)
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(serviceEnv.Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{
		"myservice": []interface{}{
			map[string]interface{}{"instance_name": "myinstance", "envs": map[string]interface{}{
				"DATABASE_HOST": "localhost",
			}},
		},
	})
	c.Assert(allEnvs["DATABASE_HOST"], check.DeepEquals, bindTypes.EnvVar{
		Name:   "DATABASE_HOST",
		Value:  "localhost",
		Public: false,
	})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 0)
}

func (s *S) TestAddInstanceMultipleServices(c *check.C) {
	a := &App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "host1"}, InstanceName: "instance1", ServiceName: "mysql"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "host2"}, InstanceName: "instance2", ServiceName: "mysql"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "host3"}, InstanceName: "instance3", ServiceName: "mongodb"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	serviceEnv := allEnvs[tsuruEnvs.TsuruServicesEnvVar]
	c.Assert(serviceEnv.Name, check.Equals, tsuruEnvs.TsuruServicesEnvVar)
	c.Assert(serviceEnv.Public, check.Equals, false)
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(serviceEnv.Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{
		"mysql": []interface{}{
			map[string]interface{}{"instance_name": "instance1", "envs": map[string]interface{}{
				"DATABASE_HOST": "host1",
			}},
			map[string]interface{}{"instance_name": "instance2", "envs": map[string]interface{}{
				"DATABASE_HOST": "host2",
			}},
		},
		"mongodb": []interface{}{
			map[string]interface{}{"instance_name": "instance3", "envs": map[string]interface{}{
				"DATABASE_HOST": "host3",
			}},
		},
	})
	c.Assert(allEnvs["DATABASE_HOST"], check.DeepEquals, bindTypes.EnvVar{
		Name:   "DATABASE_HOST",
		Value:  "host3",
		Public: false,
	})
}

func (s *S) TestAddInstanceAndRemoveInstanceMultipleServices(c *check.C) {
	a := &App{Name: "fuchsia", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "host1"}, InstanceName: "instance1", ServiceName: "mysql"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "host2"}, InstanceName: "instance2", ServiceName: "mysql"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	c.Assert(allEnvs["DATABASE_HOST"], check.DeepEquals, bindTypes.EnvVar{
		Name:   "DATABASE_HOST",
		Value:  "host2",
		Public: false,
	})
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(allEnvs[tsuruEnvs.TsuruServicesEnvVar].Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{
		"mysql": []interface{}{
			map[string]interface{}{"instance_name": "instance1", "envs": map[string]interface{}{
				"DATABASE_HOST": "host1",
			}},
			map[string]interface{}{"instance_name": "instance2", "envs": map[string]interface{}{
				"DATABASE_HOST": "host2",
			}},
		},
	})
	err = a.RemoveInstance(bind.RemoveInstanceArgs{
		ServiceName:   "mysql",
		InstanceName:  "instance2",
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	allEnvs = a.Envs()
	c.Assert(allEnvs["DATABASE_HOST"], check.DeepEquals, bindTypes.EnvVar{
		Name:   "DATABASE_HOST",
		Value:  "host1",
		Public: false,
	})
	err = json.Unmarshal([]byte(allEnvs[tsuruEnvs.TsuruServicesEnvVar].Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{
		"mysql": []interface{}{
			map[string]interface{}{"instance_name": "instance1", "envs": map[string]interface{}{
				"DATABASE_HOST": "host1",
			}},
		},
	})
}

func (s *S) TestRemoveInstance(c *check.C) {
	a := &App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, InstanceName: "mydb", ServiceName: "mysql"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = a.RemoveInstance(bind.RemoveInstanceArgs{
		ServiceName:   "mysql",
		InstanceName:  "mydb",
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	c.Assert(allEnvs["DATABASE_HOST"], check.DeepEquals, bindTypes.EnvVar{})
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(allEnvs[tsuruEnvs.TsuruServicesEnvVar].Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 0)
}

func (s *S) TestRemoveInstanceShifts(c *check.C) {
	a := &App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	toAdd := []bindTypes.ServiceEnvVar{
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, InstanceName: "mydb", ServiceName: "mysql"},
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_NAME", Value: "yourdb"}, InstanceName: "yourdb", ServiceName: "mysql"},
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_NAME", Value: "hisdb"}, InstanceName: "hisdb", ServiceName: "mysql"},
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_NAME", Value: "herdb"}, InstanceName: "herdb", ServiceName: "mysql"},
		{EnvVar: bindTypes.EnvVar{Name: "DATABASE_NAME", Value: "ourdb"}, InstanceName: "ourdb", ServiceName: "mysql"},
	}
	for _, env := range toAdd {
		err = a.AddInstance(bind.AddInstanceArgs{
			Envs:          []bindTypes.ServiceEnvVar{env},
			ShouldRestart: false,
		})
		c.Assert(err, check.IsNil)
	}
	err = a.RemoveInstance(bind.RemoveInstanceArgs{
		ServiceName:   "mysql",
		InstanceName:  "hisdb",
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(allEnvs[tsuruEnvs.TsuruServicesEnvVar].Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{
		"mysql": []interface{}{
			map[string]interface{}{"instance_name": "mydb", "envs": map[string]interface{}{
				"DATABASE_NAME": "mydb",
			}},
			map[string]interface{}{"instance_name": "yourdb", "envs": map[string]interface{}{
				"DATABASE_NAME": "yourdb",
			}},
			map[string]interface{}{"instance_name": "herdb", "envs": map[string]interface{}{
				"DATABASE_NAME": "herdb",
			}},
			map[string]interface{}{"instance_name": "ourdb", "envs": map[string]interface{}{
				"DATABASE_NAME": "ourdb",
			}},
		},
	})
	c.Assert(allEnvs["DATABASE_NAME"], check.DeepEquals, bindTypes.EnvVar{
		Name:  "DATABASE_NAME",
		Value: "ourdb",
	})
}

func (s *S) TestRemoveInstanceNotFound(c *check.C) {
	a := &App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs:          []bindTypes.ServiceEnvVar{{EnvVar: bindTypes.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, InstanceName: "mydb", ServiceName: "mysql"}},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = a.RemoveInstance(bind.RemoveInstanceArgs{
		ServiceName:   "mysql",
		InstanceName:  "yourdb",
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(allEnvs[tsuruEnvs.TsuruServicesEnvVar].Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{
		"mysql": []interface{}{
			map[string]interface{}{"instance_name": "mydb", "envs": map[string]interface{}{
				"DATABASE_NAME": "mydb",
			}},
		},
	})
	c.Assert(allEnvs["DATABASE_NAME"], check.DeepEquals, bindTypes.EnvVar{
		Name:  "DATABASE_NAME",
		Value: "mydb",
	})
}

func (s *S) TestRemoveInstanceServiceNotFound(c *check.C) {
	a := &App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs:          []bindTypes.ServiceEnvVar{{EnvVar: bindTypes.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, InstanceName: "mydb", ServiceName: "mysql"}},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = a.RemoveInstance(bind.RemoveInstanceArgs{
		ServiceName:   "mongodb",
		InstanceName:  "mydb",
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(allEnvs[tsuruEnvs.TsuruServicesEnvVar].Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{
		"mysql": []interface{}{
			map[string]interface{}{"instance_name": "mydb", "envs": map[string]interface{}{
				"DATABASE_NAME": "mydb",
			}},
		},
	})
	c.Assert(allEnvs["DATABASE_NAME"], check.DeepEquals, bindTypes.EnvVar{
		Name:  "DATABASE_NAME",
		Value: "mydb",
	})
}

func (s *S) TestRemoveInstanceWithUnits(c *check.C) {
	a := &App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, a)
	err = a.AddUnits(1, "web", "", nil)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs:          []bindTypes.ServiceEnvVar{{EnvVar: bindTypes.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, InstanceName: "mydb", ServiceName: "mysql"}},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = a.RemoveInstance(bind.RemoveInstanceArgs{
		ServiceName:   "mysql",
		InstanceName:  "mydb",
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(allEnvs[tsuruEnvs.TsuruServicesEnvVar].Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{})
	c.Assert(allEnvs["DATABASE_NAME"], check.DeepEquals, bindTypes.EnvVar{})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 1)
}

func (s *S) TestRemoveInstanceWithUnitsNoRestart(c *check.C) {
	a := &App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, a)
	err = a.AddUnits(1, "web", "", nil)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs:          []bindTypes.ServiceEnvVar{{EnvVar: bindTypes.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, InstanceName: "mydb", ServiceName: "mysql"}},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = a.RemoveInstance(bind.RemoveInstanceArgs{
		ServiceName:   "mysql",
		InstanceName:  "mydb",
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(allEnvs[tsuruEnvs.TsuruServicesEnvVar].Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{})
	c.Assert(allEnvs["DATABASE_NAME"], check.DeepEquals, bindTypes.EnvVar{})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 0)
}

func (s *S) TestIsValid(c *check.C) {
	teamName := "noaccessteam"
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		if name == teamName || name == s.team.Name {
			return &authTypes.Team{Name: teamName}, nil
		}
		return nil, authTypes.ErrTeamNotFound
	}
	plan1 := appTypes.Plan{Name: "plan1", Memory: 1}
	plan2 := appTypes.Plan{Name: "plan2", Memory: 1}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{s.defaultPlan, plan1, plan2}, nil
	}
	err := pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr:  "pool1",
		Field:     pool.ConstraintTypeTeam,
		Values:    []string{teamName},
		Blacklist: true,
	})
	c.Assert(err, check.IsNil)
	err = pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr:  "pool1",
		Field:     pool.ConstraintTypePlan,
		Values:    []string{"plan1"},
		Blacklist: true,
	})
	c.Assert(err, check.IsNil)
	errMsg := "Invalid app name, your app should have at most 40 characters, containing only lower case letters, numbers or dashes, starting with a letter."
	var data = []struct {
		name      string
		teamOwner string
		pool      string
		router    string
		plan      string
		expected  string
	}{
		{"myappmyappmyappmyappmyappmyappmyappmyappmy", s.team.Name, "pool1", "fake", "default-plan", errMsg},
		{"myappmyappmyappmyappmyappmyappmyappmyappm", s.team.Name, "pool1", "fake", "default-plan", errMsg},
		{"myappmyappmyappmyappmyappmyappmyappmyapp", s.team.Name, "pool1", "fake", "default-plan", ""},
		{"myApp", s.team.Name, "pool1", "fake", "default-plan", errMsg},
		{"my app", s.team.Name, "pool1", "fake", "default-plan", errMsg},
		{"123myapp", s.team.Name, "pool1", "fake", "default-plan", errMsg},
		{"myapp", s.team.Name, "pool1", "fake", "default-plan", ""},
		{"_theirapp", s.team.Name, "pool1", "fake", "default-plan", errMsg},
		{"my-app", s.team.Name, "pool1", "fake", "default-plan", ""},
		{"-myapp", s.team.Name, "pool1", "fake", "default-plan", errMsg},
		{"my_app", s.team.Name, "pool1", "fake", "default-plan", errMsg},
		{"b", s.team.Name, "pool1", "fake", "default-plan", ""},
		{"myapp", "invalidteam", "pool1", "fake", "default-plan", "team not found"},
		{"myapp", s.team.Name, "pool1", "faketls", "default-plan", "router \"faketls\" is not available for pool \"pool1\". Available routers are: \"fake, fake-tls\""},
		{"myapp", "noaccessteam", "pool1", "fake", "default-plan", "App team owner \"noaccessteam\" has no access to pool \"pool1\""},
		{"myApp", s.team.Name, "pool1", "fake", "default-plan", errMsg},
		{"myapp", s.team.Name, "pool1", "fake", "plan1", "App plan \"plan1\" is not allowed on pool \"pool1\""},
		{"myapp", s.team.Name, "pool1", "fake", "plan2", ""},
	}
	for _, d := range data {
		a := App{Name: d.name, Plan: appTypes.Plan{Name: d.plan}, TeamOwner: d.teamOwner, Pool: d.pool, Routers: []appTypes.AppRouter{{Name: d.router}}}
		if valid := a.validateNew(context.TODO()); valid != nil && valid.Error() != d.expected {
			c.Errorf("Is %q a valid app? Expected: %v. Got: %v.", d.name, d.expected, valid)
		}
	}
}

func (s *S) TestRestart(c *check.C) {
	s.provisioner.PrepareOutput([]byte("not yaml")) // loadConf
	a := App{
		Name:      "someapp",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Routers:   []appTypes.AppRouter{{Name: "fake"}},
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	var b bytes.Buffer
	err = a.Restart(context.TODO(), "", "", &b)
	c.Assert(err, check.IsNil)
	c.Assert(b.String(), check.Matches, `(?s).*---- Restarting the app "someapp" ----.*`)
	restarts := s.provisioner.Restarts(&a, "")
	c.Assert(restarts, check.Equals, 1)
}

func (s *S) TestStop(c *check.C) {
	a := App{Name: "app", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	newSuccessfulAppVersion(c, &a)
	err = a.AddUnits(2, "web", "", nil)
	c.Assert(err, check.IsNil)
	err = a.Stop(context.TODO(), &buf, "", "")
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Find(bson.M{"name": a.GetName()}).One(&a)
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 2)
	for _, u := range units {
		c.Assert(u.Status, check.Equals, provision.StatusStopped)
	}
}

func (s *S) TestStopPastUnits(c *check.C) {
	a := App{Name: "app", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	version := newSuccessfulAppVersion(c, &a)
	err = a.AddUnits(2, "web", strconv.Itoa(version.Version()), nil)
	c.Assert(err, check.IsNil)
	err = a.Stop(context.TODO(), &buf, "", "")
	c.Assert(err, check.IsNil)
	var updatedVersion appTypes.AppVersion
	updatedVersion, err = a.getVersion(context.TODO(), strconv.Itoa(version.Version()))
	c.Assert(err, check.IsNil)
	pastUnits := updatedVersion.VersionInfo().PastUnits
	c.Assert(pastUnits, check.HasLen, 1)
	c.Assert(pastUnits, check.DeepEquals, map[string]int{"web": 2})
}

func (s *S) TestLastLogs(c *check.C) {
	app := App{
		Name:      "app3",
		Platform:  "vougan",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	for i := 0; i < 15; i++ {
		servicemanager.LogService.Add(app.Name, strconv.Itoa(i), "tsuru", "rdaneel")
		time.Sleep(1e6) // let the time flow
	}
	servicemanager.LogService.Add(app.Name, "app3 log from circus", "circus", "rdaneel")
	logs, err := app.LastLogs(context.TODO(), servicemanager.LogService, appTypes.ListLogArgs{
		Limit:  10,
		Source: "tsuru",
	})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 10)
	for i := 5; i < 15; i++ {
		c.Check(logs[i-5].Message, check.Equals, strconv.Itoa(i))
		c.Check(logs[i-5].Source, check.Equals, "tsuru")
	}
}

func (s *S) TestLastLogsInvertFilters(c *check.C) {
	app := App{
		Name:      "app3",
		Platform:  "vougan",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	for i := 0; i < 15; i++ {
		servicemanager.LogService.Add(app.Name, strconv.Itoa(i), "tsuru", "rdaneel")
		time.Sleep(1e6) // let the time flow
	}
	servicemanager.LogService.Add(app.Name, "app3 log from circus", "circus", "rdaneel")
	logs, err := app.LastLogs(context.TODO(), servicemanager.LogService, appTypes.ListLogArgs{
		Limit:        10,
		Source:       "tsuru",
		InvertSource: true,
	})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 1)
	c.Check(logs[0].Message, check.Equals, "app3 log from circus")
	c.Check(logs[0].Source, check.Equals, "circus")
}

type logDisabledFakeProvisioner struct {
	provisiontest.FakeProvisioner
}

func (p *logDisabledFakeProvisioner) LogsEnabled(app provision.App) (bool, string, error) {
	return false, "my doc msg", nil
}

func (s *S) TestLastLogsDisabled(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "log-disabled"
	provision.Register("log-disabled", func() (provision.Provisioner, error) {
		return &logDisabledFakeProvisioner{}, nil
	})
	defer provision.Unregister("log-disabled")
	app := App{
		Name:     "app3",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	_, err = app.LastLogs(context.TODO(), servicemanager.LogService, appTypes.ListLogArgs{
		Limit: 10,
	})
	c.Assert(err, check.ErrorMatches, "my doc msg")
}

func (s *S) TestGetTeams(c *check.C) {
	app := App{Name: "app", Teams: []string{s.team.Name}}
	teams := app.GetTeams()
	c.Assert(teams, check.HasLen, 1)
	c.Assert(teams[0].Name, check.Equals, s.team.Name)
}

func (s *S) TestAppMarshalJSON(c *check.C) {
	s.plan = appTypes.Plan{Name: "myplan", Memory: 64}
	team := authTypes.Team{Name: "myteam"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{team, {Name: s.team.Name}}, nil
	}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, team.Name)
		return &team, nil
	}
	opts := pool.AddPoolOptions{Name: "test", Default: false}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	config.Set("volume-plans:test-plan:fake:plugin", "nfs")

	volume := &volumeTypes.Volume{
		Name:      "test-volume",
		Pool:      "test",
		TeamOwner: "myteam",
		Plan: volumeTypes.VolumePlan{
			Name: "test-plan",
		},
	}
	err = servicemanager.Volume.Create(context.TODO(), volume)
	c.Assert(err, check.IsNil)

	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     volume,
		AppName:    "name",
		MountPoint: "/mnt",
		ReadOnly:   true,
	})
	c.Assert(err, check.IsNil)

	app := App{
		Name:        "name",
		Platform:    "Framework",
		Teams:       []string{"team1"},
		CName:       []string{"name.mycompany.com"},
		Owner:       s.user.Email,
		Deploys:     7,
		Pool:        "test",
		Description: "description",
		Plan:        s.plan,
		TeamOwner:   "myteam",
		Routers:     []appTypes.AppRouter{{Name: "fake", Opts: map[string]string{"opt1": "val1"}}},
		Tags:        []string{"tag a", "tag b"},
		InternalAddresses: []provision.AppInternalAddress{
			{
				Domain:   "name-web.cluster.local",
				Protocol: "TCP",
				Port:     4000,
				Process:  "web",
			},
		},
	}
	err = CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)

	newSuccessfulAppVersion(c, &app)
	_, err = s.provisioner.AddUnitsToNode(&app, 1, "web", nil, "addr1", nil)
	c.Assert(err, check.IsNil)

	err = routertest.FakeRouter.EnsureBackend(context.TODO(), &app, router.EnsureBackendOpts{})
	c.Assert(err, check.IsNil)

	units, err := app.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)

	unitAddress := map[string]interface{}{}
	rawAddress, err := json.Marshal(units[0].Address)
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(rawAddress, &unitAddress)
	c.Assert(err, check.IsNil)

	expected := map[string]interface{}{
		"name":     "name",
		"platform": "Framework",
		"teams":    []interface{}{"myteam"},
		"units": []interface{}{
			map[string]interface{}{
				"Address":      unitAddress,
				"Addresses":    nil,
				"AppName":      "name",
				"CreatedAt":    nil,
				"HostAddr":     "addr1",
				"HostPort":     "1",
				"ID":           "name-0",
				"IP":           "addr1",
				"InternalIP":   "",
				"Name":         "",
				"ProcessName":  "web",
				"Ready":        nil,
				"Restarts":     nil,
				"Routable":     false,
				"Status":       "started",
				"StatusReason": "",
				"Type":         "Framework", "Version": 0},
		},
		"unitsMetrics": []interface{}{
			map[string]interface{}{
				"ID":     "name-0",
				"CPU":    "10m",
				"Memory": "100Mi",
			},
		},
		"ip": "name.fakerouter.com",
		"internalAddresses": []interface{}{
			map[string]interface{}{
				"Domain":   "name-web.cluster.local",
				"Protocol": "TCP",
				"Port":     float64(4000),
				"Process":  "web",
				"Version":  "",
			}},
		"provisioner": "fake",
		"cname":       []interface{}{"name.mycompany.com"},
		"owner":       s.user.Email,
		"deploys":     float64(7),
		"pool":        "test",
		"description": "description",
		"teamowner":   "myteam",
		"lock":        s.zeroLock,
		"plan": map[string]interface{}{
			"name":     "myplan",
			"memory":   float64(64),
			"cpumilli": float64(0),
			"router":   "fake",
			"override": map[string]interface{}{
				"cpumilli": nil,
				"memory":   nil,
				"cpuBurst": nil,
			},
		},
		"router":     "fake",
		"routeropts": map[string]interface{}{"opt1": "val1"},
		"routers": []interface{}{
			map[string]interface{}{
				"name":      "fake",
				"address":   "name.fakerouter.com",
				"addresses": []interface{}{"name.fakerouter.com"},
				"type":      "fake",
				"opts":      map[string]interface{}{"opt1": "val1"},
				"status":    "ready",
			},
		},
		"tags": []interface{}{"tag a", "tag b"},
		"metadata": map[string]interface{}{
			"annotations": nil,
			"labels":      nil,
		},
		"volumeBinds": []interface{}{
			map[string]interface{}{
				"ID": map[string]interface{}{
					"App":        "name",
					"MountPoint": "/mnt",
					"Volume":     "test-volume",
				},
				"ReadOnly": true,
			},
		},
		"quota": map[string]interface{}{
			"inuse": float64(0),
			"limit": float64(-1),
		},
		"serviceInstanceBinds": []interface{}{},
	}
	data, err := app.MarshalJSON()
	c.Assert(err, check.IsNil)
	result := make(map[string]interface{})
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, tsuruTest.JSONEquals, expected)
}

func (s *S) TestAppMarshalJSONWithAutoscaleProv(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "autoscaleProv"
	provision.Register("autoscaleProv", func() (provision.Provisioner, error) {
		return &provisiontest.AutoScaleProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("autoscaleProv")

	opts := pool.AddPoolOptions{Name: "test", Default: false, Provisioner: "autoscaleProv"}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	app := App{
		Name:        "name",
		Platform:    "Framework",
		Teams:       []string{"team1"},
		CName:       []string{"name.mycompany.com"},
		Owner:       "appOwner",
		Deploys:     7,
		Pool:        "test",
		Description: "description",
		Plan:        appTypes.Plan{Name: "myplan", Memory: 64},
		TeamOwner:   "myteam",
		Routers:     []appTypes.AppRouter{{Name: "fake", Opts: map[string]string{"opt1": "val1"}}},
		Tags:        []string{"tag a", "tag b"},
		InternalAddresses: []provision.AppInternalAddress{
			{
				Domain:   "name-web.cluster.local",
				Protocol: "TCP",
				Port:     4000,
				Process:  "web",
			},
		},
	}
	err = app.AutoScale(provision.AutoScaleSpec{Process: "p1"})
	c.Assert(err, check.IsNil)
	err = routertest.FakeRouter.EnsureBackend(context.TODO(), &app, router.EnsureBackendOpts{})
	c.Assert(err, check.IsNil)
	expected := map[string]interface{}{
		"name":     "name",
		"platform": "Framework",
		"teams":    []interface{}{"team1"},
		"units":    []interface{}{},
		"ip":       "name.fakerouter.com",
		"internalAddresses": []interface{}{
			map[string]interface{}{
				"Domain":   "name-web.cluster.local",
				"Protocol": "TCP",
				"Port":     float64(4000),
				"Version":  "",
				"Process":  "web",
			}},
		"provisioner": "fake",
		"cname":       []interface{}{"name.mycompany.com"},
		"owner":       "appOwner",
		"deploys":     float64(7),
		"pool":        "test",
		"description": "description",
		"teamowner":   "myteam",
		"lock":        s.zeroLock,
		"plan": map[string]interface{}{
			"name":     "myplan",
			"memory":   float64(64),
			"cpumilli": float64(0),
			"router":   "fake",
			"override": map[string]interface{}{
				"cpumilli": nil,
				"memory":   nil,
				"cpuBurst": nil,
			},
		},
		"metadata": map[string]interface{}{
			"annotations": nil,
			"labels":      nil,
		},
		"router":     "fake",
		"routeropts": map[string]interface{}{"opt1": "val1"},
		"routers": []interface{}{
			map[string]interface{}{
				"name":    "fake",
				"address": "name.fakerouter.com",
				"addresses": []interface{}{
					"name.fakerouter.com",
				},
				"type":   "fake",
				"opts":   map[string]interface{}{"opt1": "val1"},
				"status": "ready",
			},
		},
		"tags": []interface{}{"tag a", "tag b"},
		"autoscale": []interface{}{
			map[string]interface{}{"process": "p1", "minUnits": float64(0), "maxUnits": float64(0), "version": float64(0)},
		},
		"autoscaleRecommendation": []interface{}{
			map[string]interface{}{
				"process": "p1",
				"recommendations": []interface{}{
					map[string]interface{}{"type": "target", "cpu": "100m", "memory": "100MiB"},
				},
			},
		},
		"quota": map[string]interface{}{
			"inuse": float64(0),
			"limit": float64(-1),
		},
		"serviceInstanceBinds": []interface{}{},
	}
	data, err := app.MarshalJSON()
	c.Assert(err, check.IsNil)
	result := make(map[string]interface{})
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
}

func (s *S) TestAppMarshalJSONUnitsError(c *check.C) {
	provisiontest.ProvisionerInstance.PrepareFailure("Units", fmt.Errorf("my err"))
	app := App{
		Name:    "name",
		Routers: []appTypes.AppRouter{{Name: "fake", Opts: map[string]string{}}},
		InternalAddresses: []provision.AppInternalAddress{
			{
				Domain:   "name-web.cluster.local",
				Protocol: "TCP",
				Port:     4000,
				Process:  "web",
			},
		},
	}
	err := routertest.FakeRouter.EnsureBackend(context.TODO(), &app, router.EnsureBackendOpts{})
	c.Assert(err, check.IsNil)
	expected := map[string]interface{}{
		"name":        "name",
		"platform":    "",
		"teams":       nil,
		"units":       []interface{}{},
		"ip":          "name.fakerouter.com",
		"cname":       nil,
		"owner":       "",
		"deploys":     float64(0),
		"pool":        "",
		"description": "",
		"teamowner":   "",
		"lock":        s.zeroLock,
		"plan": map[string]interface{}{
			"name":     "",
			"memory":   float64(0),
			"cpumilli": float64(0),
			"router":   "fake",
			"override": map[string]interface{}{
				"cpumilli": nil,
				"memory":   nil,
				"cpuBurst": nil,
			},
		},
		"metadata": map[string]interface{}{
			"annotations": nil,
			"labels":      nil,
		},
		"router":     "fake",
		"routeropts": map[string]interface{}{},
		"routers": []interface{}{
			map[string]interface{}{
				"name":    "fake",
				"address": "name.fakerouter.com",
				"addresses": []interface{}{
					"name.fakerouter.com",
				},
				"type":   "fake",
				"opts":   map[string]interface{}{},
				"status": "ready",
			},
		},
		"tags":        nil,
		"provisioner": "fake",
		"quota": map[string]interface{}{
			"inuse": float64(0),
			"limit": float64(-1),
		},
		"internalAddresses": []interface{}{
			map[string]interface{}{
				"Domain":   "name-web.cluster.local",
				"Protocol": "TCP",
				"Port":     float64(4000),
				"Process":  "web",
				"Version":  "",
			}},
		"serviceInstanceBinds": []interface{}{},
	}
	data, err := app.MarshalJSON()
	c.Assert(err, check.IsNil)
	result := make(map[string]interface{})
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result["error"], check.Matches, `(?s)unable to list app units: my err.*`)
	delete(result, "error")
	c.Assert(result, check.DeepEquals, expected)
}

func (s *S) TestAppMarshalJSONPlatformLocked(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test", Default: false}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	app := App{
		Name:            "name",
		Platform:        "Framework",
		PlatformVersion: "v1",
		Teams:           []string{"team1"},
		CName:           []string{"name.mycompany.com"},
		Owner:           "appOwner",
		Deploys:         7,
		Pool:            "test",
		Description:     "description",
		Plan:            appTypes.Plan{Name: "myplan", Memory: 64},
		TeamOwner:       "myteam",
		Routers:         []appTypes.AppRouter{{Name: "fake", Opts: map[string]string{"opt1": "val1"}}},
		Tags:            []string{"tag a", "tag b"},
		Metadata:        appTypes.Metadata{Labels: []appTypes.MetadataItem{{Name: "label", Value: "value"}}},
		InternalAddresses: []provision.AppInternalAddress{
			{
				Domain:   "name-web.cluster.local",
				Protocol: "TCP",
				Port:     4000,
				Process:  "web",
			},
		},
	}
	err = routertest.FakeRouter.EnsureBackend(context.TODO(), &app, router.EnsureBackendOpts{})
	c.Assert(err, check.IsNil)
	expected := map[string]interface{}{
		"name":        "name",
		"platform":    "Framework:v1",
		"teams":       []interface{}{"team1"},
		"units":       []interface{}{},
		"ip":          "name.fakerouter.com",
		"cname":       []interface{}{"name.mycompany.com"},
		"owner":       "appOwner",
		"deploys":     float64(7),
		"pool":        "test",
		"description": "description",
		"teamowner":   "myteam",
		"lock":        s.zeroLock,
		"plan": map[string]interface{}{
			"name":     "myplan",
			"memory":   float64(64),
			"cpumilli": float64(0),
			"router":   "fake",
			"override": map[string]interface{}{
				"cpumilli": nil,
				"memory":   nil,
				"cpuBurst": nil,
			},
		},
		"metadata": map[string]interface{}{
			"annotations": nil,
			"labels":      []interface{}{map[string]interface{}{"name": "label", "value": "value"}},
		},
		"router":     "fake",
		"routeropts": map[string]interface{}{"opt1": "val1"},
		"routers": []interface{}{
			map[string]interface{}{
				"name":    "fake",
				"address": "name.fakerouter.com",
				"addresses": []interface{}{
					"name.fakerouter.com",
				},
				"type":   "fake",
				"opts":   map[string]interface{}{"opt1": "val1"},
				"status": "ready",
			},
		},
		"tags":        []interface{}{"tag a", "tag b"},
		"provisioner": "fake",
		"quota": map[string]interface{}{
			"inuse": float64(0),
			"limit": float64(-1),
		},
		"serviceInstanceBinds": []interface{}{},
		"internalAddresses": []interface{}{
			map[string]interface{}{
				"Domain":   "name-web.cluster.local",
				"Protocol": "TCP",
				"Port":     float64(4000),
				"Process":  "web",
				"Version":  "",
			}},
	}
	data, err := app.MarshalJSON()
	c.Assert(err, check.IsNil)
	result := make(map[string]interface{})
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
}

func (s *S) TestAppMarshalJSONWithCustomQuota(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "my-pool", Default: false})
	c.Assert(err, check.IsNil)
	app := App{
		Name:            "my-awesome-app",
		Platform:        "awesome-platform",
		PlatformVersion: "v1",
		Teams:           []string{"team-one"},
		CName:           []string{"my-awesome-app.mycompany.com"},
		Owner:           "admin@example.com",
		Deploys:         1,
		Pool:            "my-pool",
		Description:     "Awesome description about my-awesome-app",
		Plan:            appTypes.Plan{Name: "small", CPUMilli: 1000, Memory: 128},
		TeamOwner:       "team-one",
		Routers:         []appTypes.AppRouter{{Name: "fake", Opts: map[string]string{"opt1": "val1"}}},
		InternalAddresses: []provision.AppInternalAddress{
			{
				Domain:   "name-web.cluster.local",
				Protocol: "TCP",
				Port:     4000,
				Process:  "web",
			},
		},
	}
	err = routertest.FakeRouter.EnsureBackend(context.TODO(), &app, router.EnsureBackendOpts{})
	c.Assert(err, check.IsNil)
	s.mockService.AppQuota.OnGet = func(_ quota.QuotaItem) (*quota.Quota, error) {
		return &quota.Quota{InUse: 100, Limit: 777}, nil
	}
	data, err := app.MarshalJSON()
	c.Assert(err, check.IsNil)
	result := make(map[string]interface{})
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, map[string]interface{}{
		"name":        "my-awesome-app",
		"platform":    "awesome-platform:v1",
		"teams":       []interface{}{"team-one"},
		"units":       []interface{}{},
		"ip":          "my-awesome-app.fakerouter.com",
		"cname":       []interface{}{"my-awesome-app.mycompany.com"},
		"owner":       "admin@example.com",
		"deploys":     float64(1),
		"pool":        "my-pool",
		"description": "Awesome description about my-awesome-app",
		"teamowner":   "team-one",
		"lock":        s.zeroLock,
		"plan": map[string]interface{}{
			"name":     "small",
			"cpumilli": float64(1000),
			"memory":   float64(128),
			"router":   "fake",
			"override": map[string]interface{}{
				"cpumilli": nil,
				"memory":   nil,
				"cpuBurst": nil,
			},
		},
		"metadata": map[string]interface{}{
			"annotations": nil,
			"labels":      nil,
		},
		"router":     "fake",
		"routeropts": map[string]interface{}{"opt1": "val1"},
		"routers": []interface{}{
			map[string]interface{}{
				"name":    "fake",
				"address": "my-awesome-app.fakerouter.com",
				"addresses": []interface{}{
					"my-awesome-app.fakerouter.com",
				},
				"type":   "fake",
				"opts":   map[string]interface{}{"opt1": "val1"},
				"status": "ready",
			},
		},
		"tags":        nil,
		"provisioner": "fake",
		"quota": map[string]interface{}{
			"inuse": float64(100),
			"limit": float64(777),
		},
		"serviceInstanceBinds": []interface{}{},
		"internalAddresses": []interface{}{
			map[string]interface{}{
				"Domain":   "name-web.cluster.local",
				"Protocol": "TCP",
				"Port":     float64(4000),
				"Process":  "web",
				"Version":  "",
			}},
	})

}

func (s *S) TestAppMarshalJSONServiceInstanceBinds(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "my-pool", Default: false})
	c.Assert(err, check.IsNil)
	app := App{
		Name:            "my-awesome-app",
		Platform:        "awesome-platform",
		PlatformVersion: "v1",
		Teams:           []string{"team-one"},
		CName:           []string{"my-awesome-app.mycompany.com"},
		Owner:           "admin@example.com",
		Deploys:         1,
		Pool:            "my-pool",
		Description:     "Awesome description about my-awesome-app",
		Plan:            appTypes.Plan{Name: "small", CPUMilli: 1000, Memory: 128},
		TeamOwner:       "team-one",
		Routers:         []appTypes.AppRouter{{Name: "fake", Opts: map[string]string{"opt1": "val1"}}},
	}
	s.mockService.Team.OnFindByNames = func(_ []string) ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: "team-one"}}, nil
	}
	err = routertest.FakeRouter.EnsureBackend(context.TODO(), &app, router.EnsureBackendOpts{})
	c.Assert(err, check.IsNil)
	service1 := service.Service{
		Name:       "service-1",
		Teams:      []string{"team-one"},
		OwnerTeams: []string{"team-one"},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err = service.Create(service1)
	c.Assert(err, check.IsNil)
	instance1 := service.ServiceInstance{
		ServiceName: service1.Name,
		Name:        service1.Name + "-1",
		Teams:       []string{"team-one"},
		Apps:        []string{app.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance1)
	c.Assert(err, check.IsNil)
	instance2 := service.ServiceInstance{
		ServiceName: service1.Name,
		Name:        service1.Name + "-2",
		Teams:       []string{"team-one"},
		Apps:        []string{app.Name},
		PlanName:    "some-example",
	}
	err = s.conn.ServiceInstances().Insert(instance2)
	c.Assert(err, check.IsNil)
	service2 := service.Service{
		Name:       "service-2",
		Teams:      []string{"team-one"},
		OwnerTeams: []string{"team-one"},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err = service.Create(service2)
	c.Assert(err, check.IsNil)
	instance3 := service.ServiceInstance{
		ServiceName: service2.Name,
		Name:        service2.Name + "-1",
		Teams:       []string{"team-one"},
		Apps:        []string{app.Name},
		PlanName:    "another-plan",
	}
	err = s.conn.ServiceInstances().Insert(instance3)
	c.Assert(err, check.IsNil)
	data, err := app.MarshalJSON()
	c.Assert(err, check.IsNil)
	result := make(map[string]interface{})
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, map[string]interface{}{
		"name":        "my-awesome-app",
		"platform":    "awesome-platform:v1",
		"teams":       []interface{}{"team-one"},
		"units":       []interface{}{},
		"ip":          "my-awesome-app.fakerouter.com",
		"cname":       []interface{}{"my-awesome-app.mycompany.com"},
		"owner":       "admin@example.com",
		"deploys":     float64(1),
		"pool":        "my-pool",
		"description": "Awesome description about my-awesome-app",
		"teamowner":   "team-one",
		"lock":        s.zeroLock,
		"plan": map[string]interface{}{
			"name":     "small",
			"cpumilli": float64(1000),
			"memory":   float64(128),
			"router":   "fake",
			"override": map[string]interface{}{
				"cpumilli": nil,
				"memory":   nil,
				"cpuBurst": nil,
			},
		},
		"metadata": map[string]interface{}{
			"annotations": nil,
			"labels":      nil,
		},
		"router":     "fake",
		"routeropts": map[string]interface{}{"opt1": "val1"},
		"routers": []interface{}{
			map[string]interface{}{
				"name":    "fake",
				"address": "my-awesome-app.fakerouter.com",
				"addresses": []interface{}{
					"my-awesome-app.fakerouter.com",
				},
				"type":   "fake",
				"opts":   map[string]interface{}{"opt1": "val1"},
				"status": "ready",
			},
		},
		"tags":        nil,
		"provisioner": "fake",
		"quota": map[string]interface{}{
			"inuse": float64(0),
			"limit": float64(-1),
		},
		"serviceInstanceBinds": []interface{}{
			map[string]interface{}{"service": "service-1", "instance": "service-1-1", "plan": ""},
			map[string]interface{}{"service": "service-1", "instance": "service-1-2", "plan": "some-example"},
			map[string]interface{}{"service": "service-2", "instance": "service-2-1", "plan": "another-plan"},
		},
		"internalAddresses": []interface{}{
			map[string]interface{}{
				"Domain":   "my-awesome-app-web.fake-cluster.local",
				"Port":     float64(80),
				"Process":  "web",
				"Protocol": "TCP",
				"Version":  "",
			},
			map[string]interface{}{
				"Domain":   "my-awesome-app-logs.fake-cluster.local",
				"Port":     float64(12201),
				"Process":  "logs",
				"Protocol": "UDP",
				"Version":  "",
			},
			map[string]interface{}{
				"Domain":   "my-awesome-app-logs-v2.fake-cluster.local",
				"Port":     float64(12201),
				"Process":  "logs",
				"Protocol": "UDP",
				"Version":  "2",
			},
			map[string]interface{}{
				"Domain":   "my-awesome-app-web-v2.fake-cluster.local",
				"Port":     float64(80),
				"Process":  "web",
				"Protocol": "TCP",
				"Version":  "2",
			},
		},
	})
}

func (s *S) TestRun(c *check.C) {
	s.provisioner.PrepareOutput([]byte("a lot of files"))
	s.provisioner.PrepareOutput([]byte("a lot of files"))
	app := App{Name: "myapp", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &app, 2, "web", newSuccessfulAppVersion(c, &app), nil)
	var buf bytes.Buffer
	args := provision.RunArgs{Once: false, Isolated: false}
	err = app.Run("ls -lh", &buf, args)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "a lot of filesa lot of files")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls -lh"
	units, err := app.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 2)
	allExecs := s.provisioner.AllExecs()
	c.Assert(allExecs, check.HasLen, 2)
	c.Assert(allExecs[units[0].GetID()], check.HasLen, 1)
	c.Assert(allExecs[units[0].GetID()][0].Cmds, check.DeepEquals, []string{"/bin/sh", "-c", expected})
	c.Assert(allExecs[units[1].GetID()], check.HasLen, 1)
	c.Assert(allExecs[units[1].GetID()][0].Cmds, check.DeepEquals, []string{"/bin/sh", "-c", expected})
	var logs []appTypes.Applog
	timeout := time.After(5 * time.Second)
	for {
		logs, err = app.LastLogs(context.TODO(), servicemanager.LogService, appTypes.ListLogArgs{
			Limit: 10,
		})
		c.Assert(err, check.IsNil)
		if len(logs) > 2 {
			break
		}
		select {
		case <-timeout:
			c.Fatal("timeout waiting for logs")
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 3)
	c.Assert(logs[1].Message, check.Equals, "a lot of files")
	c.Assert(logs[1].Source, check.Equals, "app-run")
	c.Assert(logs[2].Message, check.Equals, "a lot of files")
	c.Assert(logs[2].Source, check.Equals, "app-run")
}

func (s *S) TestRunOnce(c *check.C) {
	s.provisioner.PrepareOutput([]byte("a lot of files"))
	app := App{
		Name:      "myapp",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &app, 2, "web", newSuccessfulAppVersion(c, &app), nil)
	var buf bytes.Buffer
	args := provision.RunArgs{Once: true, Isolated: false}
	err = app.Run("ls -lh", &buf, args)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "a lot of files")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls -lh"
	units, err := app.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 2)
	allExecs := s.provisioner.AllExecs()
	c.Assert(allExecs, check.HasLen, 1)
	c.Assert(allExecs[units[0].GetID()], check.HasLen, 1)
	c.Assert(allExecs[units[0].GetID()][0].Cmds, check.DeepEquals, []string{"/bin/sh", "-c", expected})
}

func (s *S) TestRunIsolated(c *check.C) {
	s.provisioner.PrepareOutput([]byte("a lot of files"))
	app := App{
		Name:      "myapp",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &app, 1, "web", newSuccessfulAppVersion(c, &app), nil)
	var buf bytes.Buffer
	args := provision.RunArgs{Once: false, Isolated: true}
	err = app.Run("ls -lh", &buf, args)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "a lot of files")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls -lh"
	allExecs := s.provisioner.AllExecs()
	c.Assert(allExecs, check.HasLen, 1)
	c.Assert(allExecs["isolated"], check.HasLen, 1)
	c.Assert(allExecs["isolated"][0].Cmds, check.DeepEquals, []string{"/bin/sh", "-c", expected})
}

func (s *S) TestRunWithoutUnits(c *check.C) {
	s.provisioner.PrepareOutput([]byte("a lot of files"))
	app := App{
		Name:      "myapp",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	args := provision.RunArgs{Once: false, Isolated: false}
	err = app.Run("ls -lh", &buf, args)
	c.Assert(err, check.ErrorMatches, `App must be available to run non-isolated commands`)
}

func (s *S) TestRunWithoutUnitsIsolated(c *check.C) {
	s.provisioner.PrepareOutput([]byte("a lot of files"))
	app := App{
		Name:      "myapp",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	args := provision.RunArgs{Once: false, Isolated: true}
	err = app.Run("ls -lh", &buf, args)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "a lot of files")
}

func (s *S) TestEnvs(c *check.C) {
	app := App{
		Name: "time",
		Env: map[string]bindTypes.EnvVar{
			"http_proxy": {
				Name:   "http_proxy",
				Value:  "http://theirproxy.com:3128/",
				Public: true,
			},
		},
	}
	expected := map[string]bindTypes.EnvVar{
		"http_proxy": {
			Name:   "http_proxy",
			Value:  "http://theirproxy.com:3128/",
			Public: true,
		},
		"TSURU_SERVICES": {
			Name:  "TSURU_SERVICES",
			Value: "{}",
		},
	}
	env := app.Envs()
	c.Assert(env, check.DeepEquals, expected)
}

func (s *S) TestEnvsInterpolate(c *check.C) {
	app := App{
		Name: "time",
		ServiceEnvs: []bindTypes.ServiceEnvVar{
			{
				EnvVar:       bindTypes.EnvVar{Name: "DB_HOST", Value: "host1"},
				ServiceName:  "srv1",
				InstanceName: "inst1",
			},
		},
		Env: map[string]bindTypes.EnvVar{
			"a":  {Name: "a", Value: "1"},
			"aa": {Name: "aa", Alias: "c"},
			"b":  {Name: "b", Alias: "a"},
			"c":  {Name: "c", Alias: "b"},

			// Mutual recursion
			"e": {Name: "e", Alias: "f"},
			"f": {Name: "f", Alias: "e"},

			// Self recursion
			"g": {Name: "g", Alias: "g"},

			// Service envs
			"h": {Name: "h", Alias: "DB_HOST"},

			// Not found
			"i": {Name: "i", Alias: "notfound"},
		},
	}
	expected := map[string]bindTypes.EnvVar{
		"a":              {Name: "a", Value: "1"},
		"aa":             {Name: "aa", Value: "1", Alias: "c"},
		"b":              {Name: "b", Value: "1", Alias: "a"},
		"c":              {Name: "c", Value: "1", Alias: "b"},
		"e":              {Name: "e", Value: "", Alias: "f"},
		"f":              {Name: "f", Value: "", Alias: "e"},
		"g":              {Name: "g", Value: "", Alias: "g"},
		"h":              {Name: "h", Value: "host1", Alias: "DB_HOST"},
		"i":              {Name: "i", Value: "", Alias: "notfound"},
		"DB_HOST":        {Name: "DB_HOST", Value: "host1"},
		"TSURU_SERVICES": {Name: "TSURU_SERVICES", Value: "{\"srv1\":[{\"instance_name\":\"inst1\",\"envs\":{\"DB_HOST\":\"host1\"}}]}"},
	}
	env := app.Envs()
	c.Assert(env, check.DeepEquals, expected)
}

func (s *S) TestEnvsWithServiceEnvConflict(c *check.C) {
	app := App{
		Name: "time",
		Env: map[string]bindTypes.EnvVar{
			"http_proxy": {
				Name:   "http_proxy",
				Value:  "http://theirproxy.com:3128/",
				Public: true,
			},
			"DB_HOST": {
				Name:  "DB_HOST",
				Value: "manual_host",
			},
		},
		ServiceEnvs: []bindTypes.ServiceEnvVar{
			{
				EnvVar: bindTypes.EnvVar{
					Name:  "DB_HOST",
					Value: "host1",
				},
				ServiceName:  "srv1",
				InstanceName: "inst1",
			},
			{
				EnvVar: bindTypes.EnvVar{
					Name:  "DB_HOST",
					Value: "host2",
				},
				ServiceName:  "srv1",
				InstanceName: "inst2",
			},
		},
	}
	expected := map[string]bindTypes.EnvVar{
		"http_proxy": {
			Name:   "http_proxy",
			Value:  "http://theirproxy.com:3128/",
			Public: true,
		},
		"DB_HOST": {
			Name:  "DB_HOST",
			Value: "host2",
		},
	}
	env := app.Envs()
	serviceEnvsRaw := env[tsuruEnvs.TsuruServicesEnvVar]
	delete(env, tsuruEnvs.TsuruServicesEnvVar)
	c.Assert(env, check.DeepEquals, expected)
	var serviceEnvVal map[string]interface{}
	err := json.Unmarshal([]byte(serviceEnvsRaw.Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{
		"srv1": []interface{}{
			map[string]interface{}{"instance_name": "inst1", "envs": map[string]interface{}{
				"DB_HOST": "host1",
			}},
			map[string]interface{}{"instance_name": "inst2", "envs": map[string]interface{}{
				"DB_HOST": "host2",
			}},
		},
	})
}

func (s *S) TestListReturnsAppsForAGivenUser(c *check.C) {
	a := App{
		Name:      "testapp",
		TeamOwner: s.team.Name,
	}
	a2 := App{
		Name:      "othertestapp",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), &a2, s.user)
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 2)
}

func (s *S) TestListReturnsAppsForAGivenUserFilteringByName(c *check.C) {
	a := App{
		Name:      "app1",
		TeamOwner: s.team.Name,
	}
	a2 := App{
		Name:      "app2",
		TeamOwner: s.team.Name,
	}
	a3 := App{
		Name:      "foo",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), &a2, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), &a3, s.user)
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), &Filter{NameMatches: "app\\d{1}"})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 2)
}

func (s *S) TestListReturnsAppsForAGivenUserFilteringByPlatform(c *check.C) {
	a := App{
		Name:      "testapp",
		Platform:  "ruby",
		TeamOwner: s.team.Name,
	}
	a2 := App{
		Name:      "othertestapp",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), &a2, s.user)
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), &Filter{Platform: "ruby"})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
}

func (s *S) TestListReturnsAppsForAGivenUserFilteringByPlatformVersion(c *check.C) {
	apps := []App{
		{Name: "testapp", Platform: "ruby", TeamOwner: s.team.Name},
		{Name: "testapplatest", Platform: "ruby", TeamOwner: s.team.Name, PlatformVersion: "latest"},
		{Name: "othertestapp", Platform: "ruby", PlatformVersion: "v1", TeamOwner: s.team.Name},
		{Name: "testappwithoutversion", Platform: "ruby", TeamOwner: s.team.Name},
		{Name: "testappwithoutversionfield", Platform: "ruby", TeamOwner: s.team.Name},
	}
	for _, a := range apps {
		err := s.conn.Apps().Insert(a)
		c.Assert(err, check.IsNil)
	}
	err := s.conn.Apps().Update(bson.M{"name": "testappwithoutversionfield"}, bson.M{"$unset": bson.M{"platformversion": ""}})
	c.Assert(err, check.IsNil)
	tt := []struct {
		platform string
		apps     []string
	}{
		{platform: "ruby", apps: []string{"testapp", "testapplatest", "othertestapp", "testappwithoutversion", "testappwithoutversionfield"}},
		{platform: "ruby:latest", apps: []string{"testapp", "testapplatest", "testappwithoutversion", "testappwithoutversionfield"}},
		{platform: "ruby:v1", apps: []string{"othertestapp"}},
	}
	for _, t := range tt {
		apps, err := List(context.TODO(), &Filter{Platform: t.platform})
		c.Assert(err, check.IsNil)
		var appNames []string
		for _, a := range apps {
			appNames = append(appNames, a.Name)
		}
		c.Assert(appNames, check.DeepEquals, t.apps, check.Commentf("Invalid apps for platform %v", t.platform))
	}
}

func (s *S) TestListReturnsAppsForAGivenUserFilteringByTeamOwner(c *check.C) {
	a := App{
		Name:      "testapp",
		Teams:     []string{s.team.Name},
		TeamOwner: "foo",
	}
	a2 := App{
		Name:      "othertestapp",
		Teams:     []string{"commonteam", s.team.Name},
		TeamOwner: "bar",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), &Filter{TeamOwner: "foo"})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
}

func (s *S) TestListReturnsAppsForAGivenUserFilteringByOwner(c *check.C) {
	a := App{
		Name:  "testapp",
		Teams: []string{s.team.Name},
		Owner: "foo",
	}
	a2 := App{
		Name:  "othertestapp",
		Teams: []string{"commonteam", s.team.Name},
		Owner: "bar",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), &Filter{UserOwner: "foo"})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
}

func (s *S) TestListReturnsAppsForAGivenUserFilteringByLockState(c *check.C) {
	a := App{
		Name:      "testapp",
		Owner:     "foo",
		TeamOwner: s.team.Name,
	}
	a2 := App{
		Name:  "othertestapp",
		Owner: "bar",
		Lock: appTypes.AppLock{
			Locked:      true,
			Reason:      "something",
			Owner:       s.user.Email,
			AcquireDate: time.Now(),
		},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), &a2, s.user)
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), &Filter{Locked: true})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].GetName(), check.Equals, "othertestapp")
}

func (s *S) TestListAll(c *check.C) {
	a := App{
		Name:  "testapp",
		Teams: []string{s.team.Name},
	}
	a2 := App{
		Name:  "othertestapp",
		Teams: []string{"commonteam", s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 2)
}

func (s *S) TestListUsesCachedRouterAddrs(c *check.C) {
	a := App{
		Name:      "app1",
		TeamOwner: s.team.Name,
	}
	a2 := App{
		Name:      "app2",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), &a2, s.user)
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 2)
	sort.Slice(apps, func(i, j int) bool {
		return apps[i].Name < apps[j].Name
	})
	c.Assert(apps, tsuruTest.JSONEquals, []App{
		{
			Name:      "app1",
			CName:     []string{},
			Teams:     []string{"tsuruteam"},
			TeamOwner: "tsuruteam",
			Owner:     "whydidifall@thewho.com",
			Env: map[string]bindTypes.EnvVar{
				"TSURU_APPNAME": {Name: "TSURU_APPNAME", Value: "app1"},
				"TSURU_APPDIR":  {Name: "TSURU_APPDIR", Value: "/home/application/current"},
			},
			ServiceEnvs: []bindTypes.ServiceEnvVar{},
			Plan: appTypes.Plan{
				Name:    "default-plan",
				Memory:  1024,
				Default: true,
			},
			Pool:       "pool1",
			RouterOpts: map[string]string{},
			Metadata: appTypes.Metadata{
				Labels:      []appTypes.MetadataItem{},
				Annotations: []appTypes.MetadataItem{},
			},
			Tags: []string{},
			Routers: []appTypes.AppRouter{
				{Name: "fake", Opts: map[string]string{}},
			},
			Quota: quota.UnlimitedQuota,
		},
		{
			Name:      "app2",
			CName:     []string{},
			Teams:     []string{"tsuruteam"},
			TeamOwner: "tsuruteam",
			Owner:     "whydidifall@thewho.com",
			Env: map[string]bindTypes.EnvVar{
				"TSURU_APPNAME": {Name: "TSURU_APPNAME", Value: "app2"},
				"TSURU_APPDIR":  {Name: "TSURU_APPDIR", Value: "/home/application/current"},
			},
			ServiceEnvs: []bindTypes.ServiceEnvVar{},
			Plan: appTypes.Plan{
				Name:    "default-plan",
				Memory:  1024,
				Default: true,
			},
			Pool: "pool1",
			Metadata: appTypes.Metadata{
				Labels:      []appTypes.MetadataItem{},
				Annotations: []appTypes.MetadataItem{},
			},
			RouterOpts: map[string]string{},
			Tags:       []string{},
			Routers: []appTypes.AppRouter{
				{Name: "fake", Opts: map[string]string{}},
			},
			Quota: quota.UnlimitedQuota,
		},
	})
	s.mockService.Cache.OnList = func(keys ...string) ([]cache.CacheEntry, error) {
		return []cache.CacheEntry{
			{Key: "app-router-addr\x00app1\x00fake", Value: "app1.fakerouter.com"},
			{Key: "app-router-addr\x00app2\x00fake", Value: "app2.fakerouter.com"},
		}, nil
	}
	timeout := time.After(5 * time.Second)
	for {
		apps, err = List(context.TODO(), nil)
		c.Assert(err, check.IsNil)
		c.Assert(apps, check.HasLen, 2)
		if apps[0].Routers[0].Address != "" && apps[1].Routers[0].Address != "" {
			break
		}
		select {
		case <-time.After(200 * time.Millisecond):
		case <-timeout:
			c.Fatal("timeout waiting for routers addr to show up")
		}
	}
	c.Assert(apps[0].Routers[0].Address, check.Equals, "app1.fakerouter.com")
	c.Assert(apps[1].Routers[0].Address, check.Equals, "app2.fakerouter.com")
}

func (s *S) TestListUsesCachedRouterAddrsWithLegacyRouter(c *check.C) {
	a := App{
		Name:      "app1",
		TeamOwner: s.team.Name,
		Teams:     []string{s.team.Name},
		Router:    "fake",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	err = routertest.FakeRouter.EnsureBackend(context.TODO(), &a, router.EnsureBackendOpts{})
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps, tsuruTest.JSONEquals, []App{
		{
			Name:        "app1",
			CName:       []string{},
			Teams:       []string{"tsuruteam"},
			TeamOwner:   "tsuruteam",
			Env:         map[string]bindTypes.EnvVar{},
			ServiceEnvs: []bindTypes.ServiceEnvVar{},
			Router:      "fake",
			RouterOpts:  map[string]string{},
			Tags:        []string{},
			Metadata: appTypes.Metadata{
				Labels:      []appTypes.MetadataItem{},
				Annotations: []appTypes.MetadataItem{},
			},
			Routers: []appTypes.AppRouter{
				{Name: "fake", Opts: map[string]string{}},
			},
		},
	})
	s.mockService.Cache.OnList = func(keys ...string) ([]cache.CacheEntry, error) {
		return []cache.CacheEntry{
			{Key: "app-router-addr\x00app1\x00fake", Value: "app1.fakerouter.com"},
		}, nil
	}
	timeout := time.After(5 * time.Second)
	for {
		apps, err = List(context.TODO(), nil)
		c.Assert(err, check.IsNil)
		c.Assert(apps, check.HasLen, 1)
		if apps[0].Routers[0].Address != "" {
			break
		}
		select {
		case <-time.After(200 * time.Millisecond):
		case <-timeout:
			c.Fatal("timeout waiting for routers addr to show up")
		}
	}
	c.Assert(apps[0].Routers[0].Address, check.Equals, "app1.fakerouter.com")
}

func (s *S) TestListFilteringByNameMatch(c *check.C) {
	a := App{
		Name:      "app1",
		TeamOwner: s.team.Name,
	}
	a2 := App{
		Name:      "app2",
		TeamOwner: s.team.Name,
	}
	a3 := App{
		Name:      "foo",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), &a2, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), &a3, s.user)
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), &Filter{NameMatches: `app\d{1}`})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 2)
}

func (s *S) TestListFilteringByNameExact(c *check.C) {
	a := App{
		Name:      "app1",
		TeamOwner: s.team.Name,
	}
	a2 := App{
		Name:      "app1-dev",
		TeamOwner: s.team.Name,
	}
	a3 := App{
		Name:      "foo",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), &a2, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), &a3, s.user)
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), &Filter{Name: "app1", NameMatches: `app\d{1}`})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].Name, check.Equals, "app1")
}

func (s *S) TestListFilteringByPlatform(c *check.C) {
	a := App{
		Name:      "testapp",
		Platform:  "ruby",
		TeamOwner: s.team.Name,
	}
	a2 := App{
		Name:      "othertestapp",
		TeamOwner: s.team.Name,
		Platform:  "python",
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), &a2, s.user)
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), &Filter{Platform: "ruby"})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
}

func (s *S) TestListFilteringByOwner(c *check.C) {
	a := App{
		Name:  "testapp",
		Owner: "foo",
	}
	a2 := App{
		Name:  "othertestapp",
		Owner: "bar",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), &Filter{UserOwner: "foo"})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
}

func (s *S) TestListFilteringByTeamOwner(c *check.C) {
	a := App{
		Name:      "testapp",
		Teams:     []string{s.team.Name},
		TeamOwner: "foo",
	}
	a2 := App{
		Name:      "othertestapp",
		Teams:     []string{s.team.Name},
		TeamOwner: "bar",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), &Filter{TeamOwner: "foo"})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
}

func (s *S) TestListFilteringByPool(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test2", Default: false}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	a := App{
		Name:  "testapp",
		Owner: "foo",
		Pool:  opts.Name,
	}
	a2 := App{
		Name:  "othertestapp",
		Owner: "bar",
		Pool:  s.Pool,
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), &Filter{Pool: s.Pool})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].GetName(), check.Equals, a2.Name)
	c.Assert(apps[0].GetPool(), check.Equals, a2.Pool)
}

func (s *S) TestListFilteringByPools(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test2", Default: false}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "test3", Default: false}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	a := App{
		Name:  "testapp",
		Owner: "foo",
		Pool:  s.Pool,
	}
	a2 := App{
		Name:  "testapp2",
		Owner: "bar",
		Pool:  "test2",
	}
	a3 := App{
		Name:  "testapp3",
		Owner: "bar",
		Pool:  "test3",
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Insert(a3)
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), &Filter{Pools: []string{s.Pool, "test2"}})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 2)
	appNames := []string{apps[0].GetName(), apps[1].GetName()}
	sort.Strings(appNames)
	c.Assert(appNames, check.DeepEquals, []string{"testapp", "testapp2"})
}

func (s *S) TestListFilteringByStatuses(c *check.C) {
	var apps []*App
	appNames := []string{"ta1", "ta2", "ta3"}
	for _, name := range appNames {
		a := App{
			Name:      name,
			Teams:     []string{s.team.Name},
			Quota:     quota.Quota{Limit: 10},
			TeamOwner: s.team.Name,
			Routers:   []appTypes.AppRouter{{Name: "fake"}},
		}
		err := CreateApp(context.TODO(), &a, s.user)
		c.Assert(err, check.IsNil)
		newSuccessfulAppVersion(c, &a)
		err = a.AddUnits(1, "", "", nil)
		c.Assert(err, check.IsNil)
		apps = append(apps, &a)
	}
	var buf bytes.Buffer
	err := apps[1].Stop(context.TODO(), &buf, "", "")
	c.Assert(err, check.IsNil)
	resultApps, err := List(context.TODO(), &Filter{Statuses: []string{"stopped"}})
	c.Assert(err, check.IsNil)
	c.Assert(resultApps, check.HasLen, 1)
	c.Assert([]string{resultApps[0].Name}, check.DeepEquals, []string{"ta2"})
}

func (s *S) TestListFilteringByTag(c *check.C) {
	app1 := App{Name: "app1", TeamOwner: s.team.Name, Tags: []string{"tag 1"}}
	err := CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := App{Name: "app2", TeamOwner: s.team.Name, Tags: []string{"tag 1", "tag 2", "tag 3"}}
	err = CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	app3 := App{Name: "app3", TeamOwner: s.team.Name, Tags: []string{"tag 4"}}
	err = CreateApp(context.TODO(), &app3, s.user)
	c.Assert(err, check.IsNil)
	resultApps, err := List(context.TODO(), &Filter{Tags: []string{" tag 1  ", "tag 1"}})
	c.Assert(err, check.IsNil)
	c.Assert(resultApps, check.HasLen, 2)
	c.Assert(resultApps[0].Name, check.Equals, app1.Name)
	c.Assert(resultApps[1].Name, check.Equals, app2.Name)
	resultApps, err = List(context.TODO(), &Filter{Tags: []string{"tag 3 ", "tag 1", ""}})
	c.Assert(err, check.IsNil)
	c.Assert(resultApps, check.HasLen, 1)
	c.Assert(resultApps[0].Name, check.Equals, app2.Name)
	resultApps, err = List(context.TODO(), &Filter{Tags: []string{"tag 1 ", "   ", " tag 4"}})
	c.Assert(err, check.IsNil)
	c.Assert(resultApps, check.HasLen, 0)
}

func (s *S) TestListReturnsEmptyAppArrayWhenUserHasNoAccessToAnyApp(c *check.C) {
	apps, err := List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.DeepEquals, []App{})
}

func (s *S) TestListReturnsAllAppsWhenUsedWithNoFilters(c *check.C) {
	a := App{Name: "testApp", Teams: []string{"notAdmin", "noSuperUser"}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(len(apps), Greater, 0)
	c.Assert(apps[0].GetName(), check.Equals, "testApp")
	c.Assert(apps[0].GetTeamsName(), check.DeepEquals, []string{"notAdmin", "noSuperUser"})
}

func (s *S) TestListFilteringExtraWithOr(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test2", Default: false}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	a := App{
		Name:  "testapp1",
		Owner: "foo",
		Pool:  opts.Name,
	}
	a2 := App{
		Name:  "testapp2",
		Teams: []string{s.team.Name},
		Owner: "bar",
		Pool:  s.Pool,
	}
	a3 := App{
		Name:  "testapp3",
		Teams: []string{"otherteam"},
		Owner: "bar",
		Pool:  opts.Name,
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Insert(a2)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Insert(a3)
	c.Assert(err, check.IsNil)
	f := &Filter{}
	f.ExtraIn("pool", s.Pool)
	f.ExtraIn("teams", "otherteam")
	apps, err := List(context.TODO(), f)
	c.Assert(err, check.IsNil)
	var appNames []string
	for _, a := range apps {
		appNames = append(appNames, a.GetName())
	}
	sort.Strings(appNames)
	c.Assert(appNames, check.DeepEquals, []string{a2.Name, a3.Name})
}

func (s *S) TestGetName(c *check.C) {
	a := App{Name: "something"}
	c.Assert(a.GetName(), check.Equals, a.Name)
}

func (s *S) TestGetQuota(c *check.C) {
	s.mockService.AppQuota.OnGet = func(item quota.QuotaItem) (*quota.Quota, error) {
		c.Assert(item.GetName(), check.Equals, "app1")
		return &quota.Quota{InUse: 1, Limit: 2}, nil
	}
	a := App{Name: "app1"}
	q, err := a.GetQuota()
	c.Assert(err, check.IsNil)
	c.Assert(q, check.DeepEquals, &quota.Quota{InUse: 1, Limit: 2})
}

func (s *S) TestGetCname(c *check.C) {
	a := App{CName: []string{"cname1", "cname2"}}
	c.Assert(a.GetCname(), check.DeepEquals, a.CName)
}

func (s *S) TestGetPlatform(c *check.C) {
	a := App{Platform: "django"}
	c.Assert(a.GetPlatform(), check.Equals, a.Platform)
}

func (s *S) TestGetDeploys(c *check.C) {
	a := App{Deploys: 3}
	c.Assert(a.GetDeploys(), check.Equals, a.Deploys)
}

func (s *S) TestGetMetadata(c *check.C) {
	a := App{}
	c.Assert(a.GetMetadata(""), check.DeepEquals, appTypes.Metadata{})

	a = App{
		Metadata: appTypes.Metadata{
			Labels: []appTypes.MetadataItem{
				{
					Name:  "abc",
					Value: "123",
				},
			},
			Annotations: []appTypes.MetadataItem{
				{
					Name:  "tsuru",
					Value: "io",
				},
			},
		},
	}
	c.Assert(a.GetMetadata(""), check.DeepEquals, appTypes.Metadata{
		Labels: []appTypes.MetadataItem{
			{
				Name:  "abc",
				Value: "123",
			},
		},
		Annotations: []appTypes.MetadataItem{
			{
				Name:  "tsuru",
				Value: "io",
			},
		},
	})

	a = App{
		Processes: []appTypes.Process{
			{
				Name: "web",
				Metadata: appTypes.Metadata{
					Labels: []appTypes.MetadataItem{
						{
							Name:  "abc",
							Value: "321",
						},
					},
					Annotations: []appTypes.MetadataItem{
						{
							Name:  "tsuru",
							Value: "PAAS",
						},
					},
				},
			},
		},
		Metadata: appTypes.Metadata{
			Labels: []appTypes.MetadataItem{
				{
					Name:  "abc",
					Value: "123",
				},
			},
			Annotations: []appTypes.MetadataItem{
				{
					Name:  "tsuru",
					Value: "io",
				},
			},
		},
	}

	c.Assert(a.GetMetadata("web"), check.DeepEquals, appTypes.Metadata{
		Labels: []appTypes.MetadataItem{
			{
				Name:  "abc",
				Value: "321",
			},
		},
		Annotations: []appTypes.MetadataItem{
			{
				Name:  "tsuru",
				Value: "PAAS",
			},
		},
	})
}

func (s *S) TestAppUnits(c *check.C) {
	a := App{Name: "anycolor", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 1, "web", newSuccessfulAppVersion(c, &a), nil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
}

func (s *S) TestAppUnitsWithAutoscaler(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "autoscaleProv"
	provisioner := &provisiontest.AutoScaleProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}
	provision.Register("autoscaleProv", func() (provision.Provisioner, error) {
		return provisioner, nil
	})
	defer provision.Unregister("autoscaleProv")

	a := App{Name: "anycolor", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	err = provisioner.SetAutoScale(context.TODO(), &a, provision.AutoScaleSpec{
		Process:    "web",
		Version:    1,
		MinUnits:   1,
		MaxUnits:   5,
		AverageCPU: "70",
	})
	c.Assert(err, check.IsNil)

	err = a.AddUnits(1, "web", "1", io.Discard)
	c.Assert(err.Error(), check.Equals, "cannot add units to an app with autoscaler configured, please update autoscale settings")
}

func (s *S) TestAppAvailable(c *check.C) {
	a := App{
		Name:      "anycolor",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	version := newSuccessfulAppVersion(c, &a)
	s.provisioner.AddUnits(context.TODO(), &a, 1, "web", version, nil)
	c.Assert(a.available(), check.Equals, true)
	s.provisioner.Stop(context.TODO(), &a, "", version, nil)
	c.Assert(a.available(), check.Equals, false)
}

func (s *S) TestStart(c *check.C) {
	s.provisioner.PrepareOutput([]byte("not yaml")) // loadConf
	a := App{
		Name:      "someapp",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Routers:   []appTypes.AppRouter{{Name: "fake"}},
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	var b bytes.Buffer
	newSuccessfulAppVersion(c, &a)
	err = a.Start(context.TODO(), &b, "", "")
	c.Assert(err, check.IsNil)
	starts := s.provisioner.Starts(&a, "")
	c.Assert(starts, check.Equals, 1)
}

func (s *S) TestAppSetUpdatePlatform(c *check.C) {
	a := App{
		Name:      "someapp",
		Platform:  "django",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	a.SetUpdatePlatform(true)
	app, err := GetByName(context.TODO(), "someapp")
	c.Assert(err, check.IsNil)
	c.Assert(app.UpdatePlatform, check.Equals, true)
}

func (s *S) TestCreateAppValidateTeamOwner(c *check.C) {
	team := authTypes.Team{Name: "test"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{team, {Name: s.team.Name}}, nil
	}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, team.Name)
		return &team, nil
	}
	a := App{Name: "test", Platform: "python", TeamOwner: team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
}

func (s *S) TestAppValidateProcesses(c *check.C) {
	a := App{
		Name: "test",
		Processes: []appTypes.Process{
			{Name: "web"},
		},
	}
	err := a.validateProcesses()
	c.Assert(err, check.IsNil)

	a = App{
		Name: "test",
		Processes: []appTypes.Process{
			{Name: "web"},
			{Name: "web"},
		},
	}
	err = a.validateProcesses()
	c.Assert(err.Error(), check.Equals, "process \"web\" is duplicated")

	a = App{
		Name: "test",
		Processes: []appTypes.Process{
			{},
		},
	}
	err = a.validateProcesses()
	c.Assert(err.Error(), check.Equals, "empty process name is not allowed")
}

func (s *S) TestAppUpdateProcessesWhenAppend(c *check.C) {
	a := App{
		Name: "test",
		Processes: []appTypes.Process{
			{Name: "web"},
		},
	}
	a.updateProcesses([]appTypes.Process{
		{
			Name: "web",
			Metadata: appTypes.Metadata{
				Labels: []appTypes.MetadataItem{
					{
						Name:  "ABC",
						Value: "123",
					},
				},
			},
		},
	})

	c.Assert(a.Processes, check.DeepEquals, []appTypes.Process{
		{
			Name: "web",
			Metadata: appTypes.Metadata{
				Labels: []appTypes.MetadataItem{
					{
						Name:  "ABC",
						Value: "123",
					},
				},
			},
		},
	})
}

func (s *S) TestAppUpdateProcessesWhenAppendEmpty(c *check.C) {
	a := App{
		Name:      "test",
		Processes: []appTypes.Process{},
	}
	a.updateProcesses([]appTypes.Process{
		{
			Name: "web",
			Metadata: appTypes.Metadata{
				Labels: []appTypes.MetadataItem{
					{
						Name:  "ABC",
						Value: "123",
					},
				},
			},
		},
	})

	c.Assert(a.Processes, check.DeepEquals, []appTypes.Process{
		{
			Name: "web",
			Metadata: appTypes.Metadata{
				Labels: []appTypes.MetadataItem{
					{
						Name:  "ABC",
						Value: "123",
					},
				},
			},
		},
	})
}

func (s *S) TestAppUpdateProcessesWhenOverride(c *check.C) {
	a := App{
		Name: "test",
		Processes: []appTypes.Process{
			{
				Name: "web",
				Metadata: appTypes.Metadata{
					Labels: []appTypes.MetadataItem{
						{
							Name:  "ABC",
							Value: "123",
						},
					},
				},
			},
		},
	}
	a.updateProcesses([]appTypes.Process{
		{
			Name: "web",
			Metadata: appTypes.Metadata{
				Labels: []appTypes.MetadataItem{
					{
						Name:  "ABC",
						Value: "321",
					},
				},
			},
		},
	})
	c.Assert(a.Processes, check.DeepEquals, []appTypes.Process{
		{
			Name: "web",
			Metadata: appTypes.Metadata{
				Labels: []appTypes.MetadataItem{
					{
						Name:  "ABC",
						Value: "321",
					},
				},
			},
		},
	})
}

func (s *S) TestAppUpdateProcessesWhenDelete(c *check.C) {
	a := App{
		Name: "test",
		Processes: []appTypes.Process{
			{
				Name: "web",
				Metadata: appTypes.Metadata{
					Labels: []appTypes.MetadataItem{
						{
							Name:  "ABC",
							Value: "123",
						},
					},
				},
			},
		},
	}
	a.updateProcesses([]appTypes.Process{
		{
			Name: "web",
			Metadata: appTypes.Metadata{
				Labels: []appTypes.MetadataItem{
					{
						Name:   "ABC",
						Delete: true,
					},
				},
			},
		},
	})
	c.Assert(a.Processes, check.DeepEquals, []appTypes.Process{})
}

func (s *S) TestAppUpdateProcessesWhenPlan(c *check.C) {
	oldPlanService := servicemanager.Plan
	servicemanager.Plan = &appTypes.MockPlanService{
		Plans: []appTypes.Plan{{Name: "c1m1"}, {Name: "c2m2"}},
	}
	defer func() {
		servicemanager.Plan = oldPlanService
	}()

	a := App{
		Name: "test",
		Processes: []appTypes.Process{
			{
				Name: "web",
			},
			{
				Name: "priority-worker",
				Plan: "c4m4",
			},
			{
				Name: "worker-metadata",
				Plan: "c4m4",
				Metadata: appTypes.Metadata{
					Labels: []appTypes.MetadataItem{
						{Name: "abc", Value: "123"},
					},
				},
			},
		},
	}
	changed, err := a.updateProcesses([]appTypes.Process{
		{
			Name: "web",
			Plan: "c1m1",
		},
		{
			Name: "worker",
			Plan: "c2m2",
		},
		{
			Name: "stale-worker",
			Plan: "$default",
		},
		{
			Name: "priority-worker",
			Plan: "$default",
		},
		{
			Name: "worker-metadata",
			Plan: "$default",
		},
	})
	c.Assert(err, check.IsNil)
	c.Assert(changed, check.Equals, true)

	c.Assert(a.Processes, check.DeepEquals, []appTypes.Process{
		{
			Name: "web",
			Plan: "c1m1",
		},
		{
			Name: "worker",
			Plan: "c2m2",
		},
		{
			Name: "worker-metadata",
			Metadata: appTypes.Metadata{
				Labels: []appTypes.MetadataItem{
					{Name: "abc", Value: "123"},
				},
			},
		},
	})
}

func (s *S) TestValidateAppService(c *check.C) {
	app := App{Name: "fyrone-flats", Platform: "python", TeamOwner: s.team.Name, Pool: s.Pool}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	serv := service.Service{
		Name:       "healthcheck",
		Password:   "nidalee",
		Endpoint:   map[string]string{"production": "somehost.com"},
		OwnerTeams: []string{s.team.Name},
	}
	err = service.Create(serv)
	c.Assert(err, check.IsNil)
	err = app.ValidateService(serv.Name)
	c.Assert(err, check.IsNil)
	err = app.ValidateService("invalidService")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "service \"invalidService\" is not available for pool \"pool1\". Available services are: \"my, mysql, healthcheck\"")
}

func (s *S) TestValidateBlacklistedAppService(c *check.C) {
	app := App{Name: "urgot", Platform: "python", TeamOwner: s.team.Name, Pool: s.Pool}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	serv := service.Service{
		Name:       "healthcheck",
		Password:   "nidalee",
		Endpoint:   map[string]string{"production": "somehost.com"},
		OwnerTeams: []string{s.team.Name},
	}
	err = service.Create(serv)
	c.Assert(err, check.IsNil)
	err = app.ValidateService(serv.Name)
	c.Assert(err, check.IsNil)

	oldOnServices := s.mockService.Pool.OnServices
	s.mockService.Pool.OnServices = func(pool string) ([]string, error) {
		if pool == "pool1" {
			return []string{}, nil
		}
		return []string{"healthcheck"}, nil
	}
	defer func() { s.mockService.Pool.OnServices = oldOnServices }()

	poolConstraint := pool.PoolConstraint{PoolExpr: s.Pool, Field: pool.ConstraintTypeService, Values: []string{serv.Name}, Blacklist: true}
	err = pool.SetPoolConstraint(&poolConstraint)
	c.Assert(err, check.IsNil)
	err = app.ValidateService(serv.Name)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "service \"healthcheck\" is not available for pool \"pool1\".")
	opts := pool.AddPoolOptions{Name: "poolz"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	app2 := App{Name: "nidalee", Platform: "python", TeamOwner: s.team.Name, Pool: "poolz"}
	err = CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	err = app2.ValidateService(serv.Name)
	c.Assert(err, check.IsNil)
}

func (s *S) TestAppCreateValidateTeamOwnerSetAnTeamWhichNotExists(c *check.C) {
	a := App{Name: "test", Platform: "python", TeamOwner: "not-exists"}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.DeepEquals, &errors.ValidationError{Message: authTypes.ErrTeamNotFound.Error()})
}

func (s *S) TestAppCreateValidateRouterNotAvailableForPool(c *check.C) {
	pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr:  "pool1",
		Field:     pool.ConstraintTypeRouter,
		Values:    []string{"fake-tls"},
		Blacklist: true,
	})
	a := App{Name: "test", Platform: "python", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.DeepEquals, &errors.ValidationError{
		Message: "router \"fake-tls\" is not available for pool \"pool1\". Available routers are: \"fake\"",
	})
}

func (s *S) TestAppSetPoolByTeamOwner(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test"}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("test", []string{"tsuruteam"})
	c.Assert(err, check.IsNil)
	app := App{
		Name:      "test",
		TeamOwner: "tsuruteam",
	}
	err = app.SetPool()
	c.Assert(err, check.IsNil)
	c.Assert(app.Pool, check.Equals, "test")
}

func (s *S) TestAppSetPoolDefault(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test", Public: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	app := App{
		Name:      "test",
		TeamOwner: "tsuruteam",
	}
	err = app.SetPool()
	c.Assert(err, check.IsNil)
	c.Assert(app.Pool, check.Equals, "pool1")
}

func (s *S) TestAppSetPoolByPool(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test"}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("test", []string{"tsuruteam"})
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "pool2"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("pool2", []string{"tsuruteam"})
	c.Assert(err, check.IsNil)
	app := App{
		Name:      "test",
		Pool:      "pool2",
		TeamOwner: "tsuruteam",
	}
	err = app.SetPool()
	c.Assert(err, check.IsNil)
	c.Assert(app.Pool, check.Equals, "pool2")
}

func (s *S) TestAppSetPoolManyPools(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test"}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("test", []string{"test"})
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "pool2"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("pool2", []string{"test"})
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "pool3", Public: true}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	app := App{
		Name:      "test",
		TeamOwner: "test",
	}
	err = app.SetPool()
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "you have access to \"test\",\"pool2\",\"pool1\",\"pool3\" pools. Please choose one in app creation")
}

func (s *S) TestAppSetPoolNoDefault(c *check.C) {
	err := pool.RemovePool("pool1")
	c.Assert(err, check.IsNil)
	app := App{
		Name: "test",
	}
	err = app.SetPool()
	c.Assert(err, check.NotNil)
	c.Assert(app.Pool, check.Equals, "")
}

func (s *S) TestAppSetPoolUserDontHaveAccessToPool(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test"}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("test", []string{"nopool"})
	c.Assert(err, check.IsNil)
	app := App{
		Name:      "test",
		TeamOwner: "tsuruteam",
		Pool:      "test",
	}
	err = app.SetPool()
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `App team owner ".*" has no access to pool ".*"`)
}

func (s *S) TestAppSetPoolToPublicPool(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test", Public: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	app := App{
		Name:      "testapp",
		TeamOwner: "tsuruteam",
		Pool:      "test",
	}
	err = app.SetPool()
	c.Assert(err, check.IsNil)
}

func (s *S) TestAppSetPoolPriorityTeamOwnerOverPublicPools(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test", Public: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "nonpublic"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("nonpublic", []string{"tsuruteam"})
	c.Assert(err, check.IsNil)
	a := App{
		Name:      "testapp",
		TeamOwner: "tsuruteam",
	}
	err = a.SetPool()
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	app, _ := GetByName(context.TODO(), a.Name)
	c.Assert("nonpublic", check.Equals, app.Pool)
}

func (s *S) TestShellToUnit(c *check.C) {
	a := App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.PrepareOutput([]byte("output"))
	s.provisioner.AddUnits(context.TODO(), &a, 1, "web", newSuccessfulAppVersion(c, &a), nil)
	units, err := s.provisioner.Units(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	unit := units[0]
	buf := safe.NewBuffer([]byte("echo teste"))
	conn := &provisiontest.FakeConn{Buf: buf}
	opts := provision.ExecOptions{
		Stdout: conn,
		Stderr: conn,
		Stdin:  conn,
		Width:  200,
		Height: 40,
		Units:  []string{unit.ID},
		Term:   "xterm",
	}
	err = a.Shell(opts)
	c.Assert(err, check.IsNil)
	allExecs := s.provisioner.AllExecs()
	c.Assert(allExecs, check.HasLen, 1)
	c.Assert(allExecs[unit.GetID()], check.HasLen, 1)
	c.Assert(allExecs[unit.GetID()][0].Cmds, check.DeepEquals, []string{"/bin/sh", "-c", "[ -f /home/application/apprc ] && source /home/application/apprc; [ -d /home/application/current ] && cd /home/application/current; [ $(command -v bash) ] && exec bash -l || exec sh -l"})
}

func (s *S) TestShellNoUnits(c *check.C) {
	a := App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.PrepareOutput([]byte("output"))
	buf := safe.NewBuffer([]byte("echo teste"))
	conn := &provisiontest.FakeConn{Buf: buf}
	opts := provision.ExecOptions{
		Stdout: conn,
		Stderr: conn,
		Stdin:  conn,
		Width:  200,
		Height: 40,
		Term:   "xterm",
	}
	err = a.Shell(opts)
	c.Assert(err, check.IsNil)
	allExecs := s.provisioner.AllExecs()
	c.Assert(allExecs, check.HasLen, 1)
	c.Assert(allExecs["isolated"], check.HasLen, 1)
	c.Assert(allExecs["isolated"][0].Cmds, check.DeepEquals, []string{"/bin/sh", "-c", "[ -f /home/application/apprc ] && source /home/application/apprc; [ -d /home/application/current ] && cd /home/application/current; [ $(command -v bash) ] && exec bash -l || exec sh -l"})
}

func (s *S) TestSetCertificateForApp(c *check.C) {
	cname := "app.io"
	cert, err := os.ReadFile("testdata/certificate.crt")
	c.Assert(err, check.IsNil)
	key, err := os.ReadFile("testdata/private.key")
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}, CName: []string{cname}}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate(cname, string(cert), string(key))
	c.Assert(err, check.IsNil)
	c.Assert(routertest.TLSRouter.Certs[cname], check.Equals, string(cert))
	c.Assert(routertest.TLSRouter.Keys[cname], check.Equals, string(key))
}

func (s *S) TestSetCertificateForAppCName(c *check.C) {
	cname := "app.io"
	cert, err := os.ReadFile("testdata/certificate.crt")
	c.Assert(err, check.IsNil)
	key, err := os.ReadFile("testdata/private.key")
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}, CName: []string{cname}}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate(cname, string(cert), string(key))
	c.Assert(err, check.IsNil)
	c.Assert(routertest.TLSRouter.Certs[cname], check.Equals, string(cert))
	c.Assert(routertest.TLSRouter.Keys[cname], check.Equals, string(key))
}

func (s *S) TestSetCertificateNonTLSRouter(c *check.C) {
	cert, err := os.ReadFile("testdata/certificate.crt")
	c.Assert(err, check.IsNil)
	key, err := os.ReadFile("testdata/private.key")
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, CName: []string{"app.io"}}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate("app.io", string(cert), string(key))
	c.Assert(err, check.ErrorMatches, "no router with tls support")
}

func (s *S) TestSetCertificateInvalidCName(c *check.C) {
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate("example.com", "cert", "key")
	c.Assert(err, check.ErrorMatches, "invalid name")
	c.Assert(routertest.TLSRouter.Certs["example.com"], check.Equals, "")
	c.Assert(routertest.TLSRouter.Keys["example.com"], check.Equals, "")
}

func (s *S) TestSetCertificateInvalidCertificateForCName(c *check.C) {
	cname := "example.io"
	cert, err := os.ReadFile("testdata/certificate.crt")
	c.Assert(err, check.IsNil)
	key, err := os.ReadFile("testdata/private.key")
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}, CName: []string{cname}}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate(cname, string(cert), string(key))
	c.Assert(err, check.ErrorMatches, "*certificate is valid for app.io, not example.io*")
	c.Assert(routertest.TLSRouter.Certs[cname], check.Equals, "")
	c.Assert(routertest.TLSRouter.Keys[cname], check.Equals, "")
}

func (s *S) TestRemoveCertificate(c *check.C) {
	cname := "app.io"
	cert, err := os.ReadFile("testdata/certificate.crt")
	c.Assert(err, check.IsNil)
	key, err := os.ReadFile("testdata/private.key")
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}, CName: []string{cname}}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate(cname, string(cert), string(key))
	c.Assert(err, check.IsNil)
	err = a.RemoveCertificate(cname)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.TLSRouter.Certs[cname], check.Equals, "")
	c.Assert(routertest.TLSRouter.Keys[cname], check.Equals, "")
}

func (s *S) TestRemoveCertificateForAppCName(c *check.C) {
	cname := "app.io"
	cert, err := os.ReadFile("testdata/certificate.crt")
	c.Assert(err, check.IsNil)
	key, err := os.ReadFile("testdata/private.key")
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}, CName: []string{cname}}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate(cname, string(cert), string(key))
	c.Assert(err, check.IsNil)
	err = a.RemoveCertificate(cname)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.TLSRouter.Certs[cname], check.Equals, "")
	c.Assert(routertest.TLSRouter.Keys[cname], check.Equals, "")
}

func (s *S) TestGetCertificates(c *check.C) {
	cname := "app.io"
	cert, err := os.ReadFile("testdata/certificate.crt")
	c.Assert(err, check.IsNil)
	key, err := os.ReadFile("testdata/private.key")
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}, CName: []string{cname}}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	newSuccessfulAppVersion(c, &a)
	err = a.AddUnits(1, "", "", nil)
	c.Assert(err, check.IsNil)

	err = a.SetCertificate(cname, string(cert), string(key))
	c.Assert(err, check.IsNil)
	expectedCerts := map[string]string{
		"app.io":                        string(cert),
		"my-test-app.faketlsrouter.com": "",
	}
	certs, err := a.GetCertificates()
	c.Assert(err, check.IsNil)
	c.Assert(certs, check.DeepEquals, map[string]map[string]string{
		"fake-tls": expectedCerts,
	})
}

func (s *S) TestGetCertificatesNonTLSRouter(c *check.C) {
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, CName: []string{"app.io"}}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	certs, err := a.GetCertificates()
	c.Assert(err, check.ErrorMatches, "no router with tls support")
	c.Assert(certs, check.IsNil)
}

func (s *S) TestUpdateAppWithInvalidName(c *check.C) {
	app := App{Name: "app with invalid name", Plan: s.defaultPlan, Platform: "python", TeamOwner: s.team.Name, Pool: s.Pool}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)

	updateData := App{Name: app.Name, Description: "bleble"}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Description, check.Equals, "bleble")
}

func (s *S) TestUpdateDescription(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla"}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "example", Description: "bleble"}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Description, check.Equals, "bleble")
}

func (s *S) TestUpdateAppPlatform(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "example", Platform: "heimerdinger"}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Platform, check.Equals, updateData.Platform)
	c.Assert(dbApp.UpdatePlatform, check.Equals, true)
}

func (s *S) TestUpdateAppPlatformWithVersion(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "example", Platform: "python:v3"}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Platform, check.Equals, "python")
	c.Assert(dbApp.PlatformVersion, check.Equals, "v3")
	c.Assert(dbApp.UpdatePlatform, check.Equals, true)
}

func (s *S) TestUpdateTeamOwner(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla"}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	teamName := "newowner"
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: teamName}, {Name: s.team.Name}}, nil
	}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, teamName)
		return &authTypes.Team{Name: teamName}, nil
	}
	updateData := App{Name: "example", TeamOwner: teamName}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.TeamOwner, check.Equals, teamName)
}

func (s *S) TestUpdateTeamOwnerNotExists(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla"}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "example", TeamOwner: "newowner"}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "team not found")
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.TeamOwner, check.Equals, s.team.Name)
}

func (s *S) TestUpdatePool(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test"}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("test", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "test2"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("test2", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	app := App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
	err = CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app)
	updateData := App{Name: "test", Pool: "test2"}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Pool, check.Equals, "test2")
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 1)
}

func (s *S) TestUpdatePoolOtherProv(c *check.C) {
	p1 := provisiontest.NewFakeProvisioner()
	p2 := provisiontest.NewFakeProvisioner()
	p1.Name = "fake1"
	p2.Name = "fake2"
	provision.Register("fake1", func() (provision.Provisioner, error) {
		return p1, nil
	})
	provision.Register("fake2", func() (provision.Provisioner, error) {
		return p2, nil
	})
	opts := pool.AddPoolOptions{Name: "test", Provisioner: "fake1", Public: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "test2", Provisioner: "fake2", Public: true}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	app := App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
	err = CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app)
	err = app.AddUnits(1, "", "", nil)
	c.Assert(err, check.IsNil)
	prov, err := app.getProvisioner()
	c.Assert(err, check.IsNil)
	c.Assert(prov.GetName(), check.Equals, "fake1")
	c.Assert(p1.GetUnits(&app), check.HasLen, 1)
	c.Assert(p2.GetUnits(&app), check.HasLen, 0)
	c.Assert(p1.Provisioned(&app), check.Equals, true)
	c.Assert(p2.Provisioned(&app), check.Equals, false)
	updateData := App{Name: "test", Pool: "test2"}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Pool, check.Equals, "test2")
	prov, err = dbApp.getProvisioner()
	c.Assert(err, check.IsNil)
	c.Assert(prov.GetName(), check.Equals, "fake2")
	c.Assert(p1.GetUnits(&app), check.HasLen, 0)
	c.Assert(p2.GetUnits(&app), check.HasLen, 1)
	c.Assert(p1.Provisioned(&app), check.Equals, false)
	c.Assert(p2.Provisioned(&app), check.Equals, true)
}

func (s *S) TestUpdatePoolNotExists(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test"}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("test", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	app := App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
	err = CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "test", Pool: "test2"}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.Equals, pool.ErrPoolNotFound)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Pool, check.Equals, "test")
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 0)
}

func (s *S) TestUpdatePoolWithBindedVolumeDifferentProvisioners(c *check.C) {
	p1 := provisiontest.NewFakeProvisioner()
	p2 := provisiontest.NewFakeProvisioner()
	p1.Name = "fake1"
	p2.Name = "fake2"
	provision.Register("fake1", func() (provision.Provisioner, error) {
		return p1, nil
	})
	provision.Register("fake2", func() (provision.Provisioner, error) {
		return p2, nil
	})
	opts := pool.AddPoolOptions{Name: "test", Provisioner: "fake1", Public: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "test2", Provisioner: "fake2", Public: true}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)

	app := App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
	err = CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app)
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	v1 := volumeTypes.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volumeTypes.VolumePlan{Name: "nfs"}}
	err = servicemanager.Volume.Create(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v1,
		AppName:    app.Name,
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	c.Assert(err, check.IsNil)
	err = app.AddUnits(1, "", "", nil)
	c.Assert(err, check.IsNil)
	prov, err := app.getProvisioner()
	c.Assert(err, check.IsNil)
	c.Assert(prov.GetName(), check.Equals, "fake1")
	c.Assert(p1.GetUnits(&app), check.HasLen, 1)
	c.Assert(p2.GetUnits(&app), check.HasLen, 0)
	c.Assert(p1.Provisioned(&app), check.Equals, true)
	c.Assert(p2.Provisioned(&app), check.Equals, false)
	updateData := App{Name: "test", Pool: "test2"}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.ErrorMatches, "can't change the provisioner of an app with binded volumes")
}

func (s *S) TestUpdatePoolWithBindedVolumeSameProvisioner(c *check.C) {
	p1 := provisiontest.NewFakeProvisioner()
	p1.Name = "fake1"
	provision.Register("fake1", func() (provision.Provisioner, error) {
		return p1, nil
	})
	opts := pool.AddPoolOptions{Name: "test", Provisioner: "fake1", Public: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "test2", Provisioner: "fake1", Public: true}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)

	app := App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
	err = CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app)
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	v1 := volumeTypes.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volumeTypes.VolumePlan{Name: "nfs"}}
	err = servicemanager.Volume.Create(context.TODO(), &v1)
	c.Assert(err, check.IsNil)
	err = servicemanager.Volume.BindApp(context.TODO(), &volumeTypes.BindOpts{
		Volume:     &v1,
		AppName:    app.Name,
		MountPoint: "/mnt",
		ReadOnly:   false,
	})
	c.Assert(err, check.IsNil)
	err = app.AddUnits(1, "", "", nil)
	c.Assert(err, check.IsNil)
	prov, err := app.getProvisioner()
	c.Assert(err, check.IsNil)
	c.Assert(prov.GetName(), check.Equals, "fake1")
	c.Assert(p1.GetUnits(&app), check.HasLen, 1)
	c.Assert(p1.Provisioned(&app), check.Equals, true)
	updateData := App{Name: "test", Pool: "test2"}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
}

func (s *S) TestUpdatePlan(c *check.C) {
	s.plan = appTypes.Plan{Name: "something", Memory: 268435456}
	a := App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Memory: 536870912}, TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 3, "web", newSuccessfulAppVersion(c, &a), nil)
	updateData := App{Name: "my-test-app", Plan: appTypes.Plan{Name: "something"}}
	err = a.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Plan, check.DeepEquals, s.plan)
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 0)
}

func (s *S) TestUpdatePlanShouldRestart(c *check.C) {
	s.plan = appTypes.Plan{Name: "something", Memory: 268435456}
	a := App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Memory: 536870912}, TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 3, "web", newSuccessfulAppVersion(c, &a), nil)
	updateData := App{Name: "my-test-app", Plan: appTypes.Plan{Name: "something"}}
	err = a.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer), ShouldRestart: true})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Plan, check.DeepEquals, s.plan)
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 1)
}

func (s *S) TestUpdatePlanWithConstraint(c *check.C) {
	s.plan = appTypes.Plan{Name: "something", Memory: 268435456}
	err := pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr:  "pool1",
		Field:     pool.ConstraintTypePlan,
		Values:    []string{s.plan.Name},
		Blacklist: true,
	})
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Memory: 536870912}, TeamOwner: s.team.Name}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "my-test-app", Plan: appTypes.Plan{Name: "something"}}
	err = a.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.ErrorMatches, `App plan "something" is not allowed on pool "pool1"`)
}

func (s *S) TestUpdatePlanWithCPUBurstExceeds(c *check.C) {
	s.plan = appTypes.Plan{
		Name:     "something",
		Memory:   268435456,
		CPUBurst: appTypes.CPUBurst{MaxAllowed: 1.8},
		Override: appTypes.PlanOverride{CPUBurst: func(f float64) *float64 { return &f }(2)},
	}
	err := pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr:  "pool1",
		Field:     pool.ConstraintTypePlan,
		Values:    []string{s.plan.Name},
		Blacklist: true,
	})
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Memory: 536870912}, TeamOwner: s.team.Name}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "my-test-app", Plan: appTypes.Plan{Name: "something"}}
	err = a.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.ErrorMatches, `CPU burst exceeds the maximum allowed by plan \"something\"`)
}

func (s *S) TestUpdatePlanNoRouteChangeShouldRestart(c *check.C) {
	s.plan = appTypes.Plan{Name: "something", Memory: 268435456}
	a := App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Memory: 536870912}, TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 3, "web", newSuccessfulAppVersion(c, &a), nil)
	updateData := App{Name: "my-test-app", Plan: appTypes.Plan{Name: "something"}}
	err = a.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer), ShouldRestart: true})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Plan, check.DeepEquals, s.plan)
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 1)
}

func (s *S) TestUpdatePlanNotFound(c *check.C) {
	var app App
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		return nil, appTypes.ErrPlanNotFound
	}
	updateData := App{Name: "my-test-app", Plan: appTypes.Plan{Name: "some-unknown-plan"}}
	err := app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.Equals, appTypes.ErrPlanNotFound)
}

func (s *S) TestUpdatePlanRestartFailure(c *check.C) {
	plan := appTypes.Plan{Name: "something", Memory: 268435456}
	oldPlan := appTypes.Plan{Name: "old", Memory: 536870912}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		if name == plan.Name {
			return &plan, nil
		}
		if name == oldPlan.Name {
			return &oldPlan, nil
		}
		c.Errorf("plan name not expected, got: %s", name)
		return nil, nil
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{s.defaultPlan, plan, oldPlan}, nil
	}
	a := App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Name: "old"}, TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 3, "web", newSuccessfulAppVersion(c, &a), nil)
	s.provisioner.PrepareFailure("Restart", fmt.Errorf("cannot restart app, I'm sorry"))
	updateData := App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Name: "something"}}
	err = a.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer), ShouldRestart: true})
	c.Assert(err, check.NotNil)
	dbApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Plan.Name, check.Equals, "old")
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 0)
}

func (s *S) TestUpdateIgnoresEmptyAndDuplicatedTags(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla", Tags: []string{"tag1"}}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Tags: []string{"tag2 ", "  tag3  ", "", " tag3", "  "}}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Tags, check.DeepEquals, []string{"tag2", "tag3"})
}

func (s *S) TestUpdatePlatform(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla"}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{UpdatePlatform: true}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.UpdatePlatform, check.Equals, true)
}

func (s *S) TestUpdateWithEmptyTagsRemovesAllTags(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla", Tags: []string{"tag1"}}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Description: "ble", Tags: []string{}}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Tags, check.DeepEquals, []string{})
}

func (s *S) TestUpdateWithoutTagsKeepsOriginalTags(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla", Tags: []string{"tag1", "tag2"}}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Description: "ble", Tags: nil}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Tags, check.DeepEquals, []string{"tag1", "tag2"})
}

func (s *S) TestUpdateDescriptionPoolPlan(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test"}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("test", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "test2"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("test2", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	plan := appTypes.Plan{Name: "something", Memory: 268435456}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{s.defaultPlan, plan}, nil
	}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		if name == s.defaultPlan.Name {
			return &s.defaultPlan, nil
		}
		c.Assert(name, check.Equals, plan.Name)
		return &plan, nil
	}
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Memory: 536870912}, Description: "blablabla", Pool: "test"}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 3, "web", newSuccessfulAppVersion(c, &a), nil)
	updateData := App{Name: "my-test-app", Plan: appTypes.Plan{Name: "something"}, Description: "bleble", Pool: "test2"}
	err = a.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Plan, check.DeepEquals, plan)
	c.Assert(dbApp.Description, check.Equals, "bleble")
	c.Assert(dbApp.Pool, check.Equals, "test2")
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 1)
}

func (s *S) TestUpdateMetadataWhenEmpty(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla"}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)

	updateData := App{Metadata: appTypes.Metadata{
		Annotations: []appTypes.MetadataItem{{Name: "a", Value: "b"}},
		Labels:      []appTypes.MetadataItem{{Name: "c", Value: "d"}},
	}}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)

	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Metadata.Annotations, check.DeepEquals, []appTypes.MetadataItem{{Name: "a", Value: "b"}})
	c.Assert(dbApp.Metadata.Labels, check.DeepEquals, []appTypes.MetadataItem{{Name: "c", Value: "d"}})
}

func (s *S) TestUpdateMetadataWhenAlreadySet(c *check.C) {
	app := App{
		Name:        "example",
		Platform:    "python",
		TeamOwner:   s.team.Name,
		Description: "blabla",
		Metadata: appTypes.Metadata{
			Annotations: []appTypes.MetadataItem{{Name: "a", Value: "old"}},
			Labels:      []appTypes.MetadataItem{{Name: "c", Value: "d"}},
		},
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)

	updateData := App{Metadata: appTypes.Metadata{Annotations: []appTypes.MetadataItem{{Name: "a", Value: "new"}}}}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)

	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Metadata.Annotations, check.DeepEquals, []appTypes.MetadataItem{{Name: "a", Value: "new"}})
	c.Assert(dbApp.Metadata.Labels, check.DeepEquals, []appTypes.MetadataItem{{Name: "c", Value: "d"}})
}

func (s *S) TestUpdateMetadataCanRemoveAnnotation(c *check.C) {
	app := App{
		Name:        "example",
		Platform:    "python",
		TeamOwner:   s.team.Name,
		Description: "blabla",
		Metadata: appTypes.Metadata{
			Annotations: []appTypes.MetadataItem{{Name: "a", Value: "old"}},
			Labels:      []appTypes.MetadataItem{{Name: "c", Value: "d"}},
		},
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)

	updateData := App{Metadata: appTypes.Metadata{Annotations: []appTypes.MetadataItem{{Name: "a", Delete: true}}}}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)

	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Metadata.Annotations, check.DeepEquals, []appTypes.MetadataItem{})
	c.Assert(dbApp.Metadata.Labels, check.DeepEquals, []appTypes.MetadataItem{{Name: "c", Value: "d"}})
}

func (s *S) TestUpdateMetadataAnnotationValidation(c *check.C) {
	app := App{
		Name:        "example",
		Platform:    "python",
		TeamOwner:   s.team.Name,
		Description: "blabla",
		Metadata: appTypes.Metadata{
			Annotations: []appTypes.MetadataItem{{Name: "a", Value: "old"}},
			Labels:      []appTypes.MetadataItem{{Name: "c", Value: "d"}},
		},
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)

	updateData := App{Metadata: appTypes.Metadata{
		Annotations: []appTypes.MetadataItem{{Name: "_invalidName", Value: "asdf"}},
		Labels:      []appTypes.MetadataItem{{Name: "tsuru.io/app-name", Value: "asdf"}},
	}}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "multiple errors reported (2):\n"+
		"error #0: metadata.annotations: Invalid value: \"_invalidName\": name part must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyName',  or 'my.name',  or '123-abc', regex used for validation is '([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]')\n"+
		"error #1: prefix tsuru.io/ is private\n")
}

func (s *S) TestRenameTeam(c *check.C) {
	apps := []App{
		{Name: "test1", TeamOwner: "t1", Routers: []appTypes.AppRouter{{Name: "fake"}}, Teams: []string{"t2", "t3", "t1"}},
		{Name: "test2", TeamOwner: "t2", Routers: []appTypes.AppRouter{{Name: "fake"}}, Teams: []string{"t3", "t1"}},
	}
	for _, a := range apps {
		err := s.conn.Apps().Insert(a)
		c.Assert(err, check.IsNil)
	}
	err := RenameTeam(context.TODO(), "t2", "t9000")
	c.Assert(err, check.IsNil)
	var dbApps []App
	err = s.conn.Apps().Find(nil).Sort("name").All(&dbApps)
	c.Assert(err, check.IsNil)
	c.Assert(dbApps, check.HasLen, 2)
	c.Assert(dbApps[0].TeamOwner, check.Equals, "t1")
	c.Assert(dbApps[1].TeamOwner, check.Equals, "t9000")
	c.Assert(dbApps[0].Teams, check.DeepEquals, []string{"t9000", "t3", "t1"})
	c.Assert(dbApps[1].Teams, check.DeepEquals, []string{"t3", "t1"})
}

func (s *S) TestRenameTeamLockedApp(c *check.C) {
	apps := []App{
		{Name: "test1", TeamOwner: "t1", Routers: []appTypes.AppRouter{{Name: "fake"}}, Teams: []string{"t2", "t3", "t1"}},
		{Name: "test2", TeamOwner: "t2", Routers: []appTypes.AppRouter{{Name: "fake"}}, Teams: []string{"t3", "t1"}},
	}
	for _, a := range apps {
		err := s.conn.Apps().Insert(a)
		c.Assert(err, check.IsNil)
	}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: "test2"},
		Kind:     permission.PermAppUpdate,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	defer evt.Done(nil)
	err = RenameTeam(context.TODO(), "t2", "t9000")
	c.Assert(err, check.ErrorMatches, `unable to create event: event locked: app\(test2\).*`)
	var dbApps []App
	err = s.conn.Apps().Find(nil).Sort("name").All(&dbApps)
	c.Assert(err, check.IsNil)
	c.Assert(dbApps, check.HasLen, 2)
	c.Assert(dbApps[0].TeamOwner, check.Equals, "t1")
	c.Assert(dbApps[1].TeamOwner, check.Equals, "t2")
	c.Assert(dbApps[0].Teams, check.DeepEquals, []string{"t2", "t3", "t1"})
	c.Assert(dbApps[1].Teams, check.DeepEquals, []string{"t3", "t1"})
}

func (s *S) TestRenameTeamUnchangedLockedApp(c *check.C) {
	apps := []App{
		{Name: "test1", TeamOwner: "t1", Routers: []appTypes.AppRouter{{Name: "fake"}}, Teams: []string{"t2", "t3", "t1"}},
		{Name: "test2", TeamOwner: "t2", Routers: []appTypes.AppRouter{{Name: "fake"}}, Teams: []string{"t3", "t1"}},
		{Name: "test3", TeamOwner: "t3", Routers: []appTypes.AppRouter{{Name: "fake"}}, Teams: []string{"t3", "t1"}},
	}
	for _, a := range apps {
		err := s.conn.Apps().Insert(a)
		c.Assert(err, check.IsNil)
	}
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: "test3"},
		Kind:     permission.PermAppUpdate,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	defer evt.Done(nil)
	err = RenameTeam(context.TODO(), "t2", "t9000")
	c.Assert(err, check.IsNil)
	var dbApps []App
	err = s.conn.Apps().Find(nil).Sort("name").All(&dbApps)
	c.Assert(err, check.IsNil)
	c.Assert(dbApps, check.HasLen, 3)
	c.Assert(dbApps[0].TeamOwner, check.Equals, "t1")
	c.Assert(dbApps[1].TeamOwner, check.Equals, "t9000")
	c.Assert(dbApps[0].Teams, check.DeepEquals, []string{"t9000", "t3", "t1"})
	c.Assert(dbApps[1].Teams, check.DeepEquals, []string{"t3", "t1"})
}

func (s *S) TestUpdateRouter(c *check.C) {
	config.Set("routers:fake:type", "fake")
	defer config.Unset("routers:fake:type")
	app := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name, Router: "none"}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddRouter(appTypes.AppRouter{
		Name: "fake",
		Opts: map[string]string{
			"a": "b",
		},
	})
	c.Assert(err, check.IsNil)
	err = app.UpdateRouter(appTypes.AppRouter{Name: "fake", Opts: map[string]string{
		"c": "d",
	}})
	c.Assert(err, check.IsNil)
	routers := app.GetRouters()
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Opts: map[string]string{"c": "d"}},
	})
	c.Assert(routertest.FakeRouter.BackendOpts["myapp"].Opts, check.DeepEquals, map[string]any{
		"c": "d",
	})
}

func (s *S) TestAddRouterFeedback(c *check.C) {
	config.Set("routers:fake:type", "fake")
	defer config.Unset("routers:fake:type")
	app := App{Name: "myapp-with-error", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(context.TODO(), &app, 1, "web", nil, nil)
	c.Assert(err, check.IsNil)
	err = app.AddRouter(appTypes.AppRouter{
		Name: "fake-tls",
		Opts: map[string]string{
			"a": "b",
		},
	})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Ensure backend error")
}

func (s *S) TestAddRouterV2FeedbackSkipRebuildIfNoUnitsDeployed(c *check.C) {
	config.Set("routers:fake:type", "fake")
	defer config.Unset("routers:fake-v2:type")
	app := App{Name: "myapp-with-error", Platform: "go", TeamOwner: s.team.Name, Router: "none"}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddRouter(appTypes.AppRouter{
		Name: "fake",
		Opts: map[string]string{
			"a": "b",
		},
	})
	c.Assert(err, check.IsNil)
}

func (s *S) TestUpdateRouterNotFound(c *check.C) {
	app := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddRouter(appTypes.AppRouter{
		Name: "fake-tls",
		Opts: map[string]string{
			"a": "b",
		},
	})
	c.Assert(err, check.IsNil)
	err = app.UpdateRouter(appTypes.AppRouter{Name: "fake-opts", Opts: map[string]string{
		"c": "d",
	}})
	c.Assert(err, check.DeepEquals, &router.ErrRouterNotFound{Name: "fake-opts"})
}

func (s *S) TestAppAddRouter(c *check.C) {
	app := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)

	newSuccessfulAppVersion(c, &app)
	err = app.AddUnits(1, "web", "", nil)
	c.Assert(err, check.IsNil)

	err = app.AddRouter(appTypes.AppRouter{
		Name: "fake-tls",
	})

	c.Assert(err, check.IsNil)
	routers, err := app.GetRoutersWithAddr()
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Address: "myapp.fakerouter.com", Addresses: []string{"myapp.fakerouter.com"}, Type: "fake", Status: "ready"},
		{Name: "fake-tls", Address: "myapp.faketlsrouter.com", Addresses: []string{"myapp.faketlsrouter.com"}, Type: "fake-tls", Status: "ready"},
	})
	addrs, err := app.GetAddresses()
	c.Assert(err, check.IsNil)
	c.Assert(addrs, check.DeepEquals, []string{"myapp.fakerouter.com", "myapp.faketlsrouter.com"})
}

func (s *S) TestAppAddRouterWithAlreadyLinkedRouter(c *check.C) {
	app := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	routers, err := app.GetRoutersWithAddr()
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Status: "ready", Type: "fake", Address: "myapp.fakerouter.com", Addresses: []string{"myapp.fakerouter.com"}},
	})
	err = app.AddRouter(appTypes.AppRouter{Name: "fake"})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.DeepEquals, ErrRouterAlreadyLinked)
}

func (s *S) TestAppAddRouterWithAppCNameUsingSameRouterOnAnotherApp(c *check.C) {
	app1 := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake"}}}
	app2 := App{Name: "myapp2", Platform: "go", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}}
	err := CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	err = app1.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	err = app2.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = app1.AddRouter(appTypes.AppRouter{Name: "fake-tls"})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cname ktulu.mycompany.com already exists for app myapp2 using router fake-tls")
	err = app2.AddRouter(appTypes.AppRouter{Name: "fake"})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cname ktulu.mycompany.com already exists for app myapp using router fake")
}

func (s *S) TestAppRemoveRouter(c *check.C) {
	app := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)

	newSuccessfulAppVersion(c, &app)
	err = app.AddUnits(1, "web", "", nil)
	c.Assert(err, check.IsNil)

	err = app.AddRouter(appTypes.AppRouter{
		Name: "fake-tls",
	})
	c.Assert(err, check.IsNil)
	err = app.RemoveRouter("fake")
	c.Assert(err, check.IsNil)
	routers, err := app.GetRoutersWithAddr()
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{
			Name:      "fake-tls",
			Addresses: []string{"myapp.faketlsrouter.com"},
			Address:   "myapp.faketlsrouter.com",
			Type:      "fake-tls",
			Status:    "ready",
		},
	})
	addrs, err := app.GetAddresses()
	c.Assert(err, check.IsNil)
	c.Assert(addrs, check.DeepEquals, []string{"myapp.faketlsrouter.com"})
}

func (s *S) TestGetRoutersWithAddr(c *check.C) {
	app := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)

	newSuccessfulAppVersion(c, &app)
	err = app.AddUnits(1, "web", "", nil)
	c.Assert(err, check.IsNil)

	err = app.AddRouter(appTypes.AppRouter{
		Name: "fake-tls",
	})
	c.Assert(err, check.IsNil)
	s.mockService.Cache.OnCreate = func(entry cache.CacheEntry) error {
		if entry.Value != "myapp.fakerouter.com" && entry.Value != "myapp.faketlsrouter.com" {
			c.Errorf("unexpected cache entry: %v", entry)
		}
		return nil
	}
	routers, err := app.GetRoutersWithAddr()
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Address: "myapp.fakerouter.com", Addresses: []string{"myapp.fakerouter.com"}, Type: "fake", Status: "ready"},
		{Name: "fake-tls", Address: "myapp.faketlsrouter.com", Addresses: []string{"myapp.faketlsrouter.com"}, Type: "fake-tls", Status: "ready"},
	})
}

func (s *S) TestGetRoutersWithAddrError(c *check.C) {
	routertest.FakeRouter.Reset()
	app := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)

	newSuccessfulAppVersion(c, &app)
	err = app.AddUnits(1, "web", "", nil)
	c.Assert(err, check.IsNil)

	err = app.AddRouter(appTypes.AppRouter{
		Name: "fake-tls",
	})
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.FailuresByHost["fake:myapp"] = true

	s.mockService.Cache.OnCreate = func(entry cache.CacheEntry) error {
		if entry.Value != "myapp.faketlsrouter.com" {
			c.Errorf("unexpected cache entry: %v", entry)
		}
		return nil
	}
	routers, err := app.GetRoutersWithAddr()
	c.Assert(strings.Contains(err.Error(), "Forced failure"), check.Equals, true)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Address: "", Type: "fake", Status: "not ready", StatusDetail: "Forced failure"},
		{Name: "fake-tls", Address: "myapp.faketlsrouter.com", Addresses: []string{"myapp.faketlsrouter.com"}, Type: "fake-tls", Status: "ready"},
	})
}

func (s *S) TestGetRoutersWithAddrWithStatus(c *check.C) {
	routertest.FakeRouter.Status.Status = router.BackendStatusNotReady
	routertest.FakeRouter.Status.Detail = "burn"
	defer routertest.FakeRouter.Reset()
	app := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name, Router: "none"}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)

	newSuccessfulAppVersion(c, &app)
	err = app.AddUnits(1, "web", "", nil)
	c.Assert(err, check.IsNil)

	err = app.AddRouter(appTypes.AppRouter{
		Name: "fake",
	})
	c.Assert(err, check.IsNil)
	s.mockService.Cache.OnCreate = func(entry cache.CacheEntry) error {
		if entry.Value != "myapp.fakerouter.com" {
			c.Errorf("unexpected cache entry: %v", entry)
		}
		return nil
	}
	routers, err := app.GetRoutersWithAddr()
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Address: "myapp.fakerouter.com", Addresses: []string{"myapp.fakerouter.com"}, Type: "fake", Status: "not ready", StatusDetail: "burn"},
	})
}

func (s *S) TestGetRoutersIgnoresDuplicatedEntry(c *check.C) {
	app := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddRouter(appTypes.AppRouter{
		Name: "fake-tls",
	})
	c.Assert(err, check.IsNil)
	app.Router = "fake-tls"
	routers := app.GetRouters()
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake"},
		{Name: "fake-tls"},
	})
}

func (s *S) TestUpdateAppUpdatableProvisioner(c *check.C) {
	p1 := provisiontest.NewFakeProvisioner()
	p1.Name = "fake1"
	provision.Register("fake1", func() (provision.Provisioner, error) {
		return p1, nil
	})
	opts := pool.AddPoolOptions{Name: "test", Provisioner: "fake1", Public: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	app := App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
	err = CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app)
	err = app.AddUnits(1, "web", "", nil)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "test", Description: "updated description"}
	err = app.Update(UpdateAppArgs{UpdateData: updateData, Writer: nil})
	c.Assert(err, check.IsNil)
	units, err := app.Units()
	c.Assert(err, check.IsNil)
	updatedApp, err := p1.GetAppFromUnitID(units[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(updatedApp.(*App).Description, check.Equals, "updated description")
}

func (s *S) TestUpdateAppPoolWithInvalidConstraint(c *check.C) {
	p1 := provisiontest.NewFakeProvisioner()
	p1.Name = "fake1"
	provision.Register("fake1", func() (provision.Provisioner, error) {
		return p1, nil
	})
	app := App{Name: "test", TeamOwner: s.team.Name, Pool: s.Pool}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))
	defer server.Close()
	svc := service.Service{
		Name:       "mysql",
		Endpoint:   map[string]string{"production": server.URL},
		Password:   "abcde",
		OwnerTeams: []string{s.team.Name},
	}
	err = service.Create(svc)
	c.Assert(err, check.IsNil)
	si1 := service.ServiceInstance{
		Name:        "mydb",
		ServiceName: svc.Name,
		Apps:        []string{app.Name},
	}
	err = s.conn.ServiceInstances().Insert(si1)
	c.Assert(err, check.IsNil)

	optsPool2 := pool.AddPoolOptions{Name: "pool2", Provisioner: p1.Name, Public: true}
	err = pool.AddPool(context.TODO(), optsPool2)
	c.Assert(err, check.IsNil)

	oldOnServices := s.mockService.Pool.OnServices
	s.mockService.Pool.OnServices = func(pool string) ([]string, error) {
		if pool == "pool2" {
			return []string{}, nil
		}

		return nil, pkgErrors.New("No services found for pool " + pool)
	}
	defer func() {
		s.mockService.Pool.OnServices = oldOnServices
	}()

	err = app.Update(UpdateAppArgs{UpdateData: App{Pool: optsPool2.Name}, Writer: nil})
	c.Assert(err, check.NotNil)
}

func (s *S) TestGetUUID(c *check.C) {
	app := App{Name: "test", TeamOwner: s.team.Name, Pool: s.Pool}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	c.Assert(app.UUID, check.DeepEquals, "")
	uuid, err := app.GetUUID()
	c.Assert(err, check.IsNil)
	c.Assert(uuid, check.Not(check.DeepEquals), "")
	c.Assert(uuid, check.DeepEquals, app.UUID)
	var storedApp App
	err = s.conn.Apps().Find(bson.M{"name": app.Name}).One(&storedApp)
	c.Assert(err, check.IsNil)
	c.Assert(storedApp.UUID, check.Not(check.DeepEquals), "")
	c.Assert(storedApp.UUID, check.DeepEquals, uuid)
}

func (s *S) TestFillInternalAddresses(c *check.C) {
	app := App{Name: "test", TeamOwner: s.team.Name, Pool: s.Pool}
	err := app.fillInternalAddresses()
	c.Assert(err, check.IsNil)
	c.Assert(app.InternalAddresses, check.HasLen, 4)
	c.Assert(app.InternalAddresses[0], check.DeepEquals, provision.AppInternalAddress{
		Domain:   "test-web.fake-cluster.local",
		Protocol: "TCP",
		Process:  "web",
		Port:     80,
	})
	c.Assert(app.InternalAddresses[1], check.DeepEquals, provision.AppInternalAddress{
		Domain:   "test-logs.fake-cluster.local",
		Protocol: "UDP",
		Process:  "logs",
		Port:     12201,
	})
	c.Assert(app.InternalAddresses[2], check.DeepEquals, provision.AppInternalAddress{
		Domain:   "test-logs-v2.fake-cluster.local",
		Protocol: "UDP",
		Process:  "logs",
		Version:  "2",
		Port:     12201,
	})
	c.Assert(app.InternalAddresses[3], check.DeepEquals, provision.AppInternalAddress{
		Domain:   "test-web-v2.fake-cluster.local",
		Protocol: "TCP",
		Process:  "web",
		Version:  "2",
		Port:     80,
	})
}

func (s *S) TestGetHealthcheckData(c *check.C) {
	a := App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	hcData, err := a.GetHealthcheckData()
	c.Assert(err, check.IsNil)
	c.Assert(hcData, check.DeepEquals, routerTypes.HealthcheckData{})
	newSuccessfulAppVersion(c, &a)
	hcData, err = a.GetHealthcheckData()
	c.Assert(err, check.IsNil)
	c.Assert(hcData, check.DeepEquals, routerTypes.HealthcheckData{
		Path: "/",
	})
}

type hcProv struct {
	provisiontest.FakeProvisioner
}

func (p *hcProv) HandlesHC() bool {
	return true
}

func (s *S) TestGetHealthcheckDataHCProvisioner(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "hcprov"
	provision.Register("hcprov", func() (provision.Provisioner, error) {
		return &hcProv{}, nil
	})
	defer provision.Unregister("hcprov")
	a := App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	hcData, err := a.GetHealthcheckData()
	c.Assert(err, check.IsNil)
	c.Assert(hcData, check.DeepEquals, routerTypes.HealthcheckData{
		TCPOnly: true,
	})
}

func (s *S) TestAutoscaleWithAutoscaleProvisioner(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "autoscaleProv"
	provision.Register("autoscaleProv", func() (provision.Provisioner, error) {
		return &provisiontest.AutoScaleProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("autoscaleProv")
	a := App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := a.AutoScale(provision.AutoScaleSpec{Process: "p1"})
	c.Assert(err, check.IsNil)
	err = a.AutoScale(provision.AutoScaleSpec{Process: "p2"})
	c.Assert(err, check.IsNil)
	scales, err := a.AutoScaleInfo()
	c.Assert(err, check.IsNil)
	c.Assert(scales, check.DeepEquals, []provision.AutoScaleSpec{
		{Process: "p1"},
		{Process: "p2"},
	})
	err = a.RemoveAutoScale("p1")
	c.Assert(err, check.IsNil)
	scales, err = a.AutoScaleInfo()
	c.Assert(err, check.IsNil)
	c.Assert(scales, check.DeepEquals, []provision.AutoScaleSpec{
		{Process: "p2"},
	})
}

func (s *S) TestGetInternalBindableAddresses(c *check.C) {
	app := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name, provisioner: s.provisioner}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	addresses, err := app.GetInternalBindableAddresses()
	c.Assert(err, check.IsNil)
	c.Assert(addresses, check.DeepEquals, []string{
		"tcp://myapp-web.fake-cluster.local:80",
		"udp://myapp-logs.fake-cluster.local:12201",
	})
}
