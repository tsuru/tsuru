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
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	pkgErrors "github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
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
	eventTypes "github.com/tsuru/tsuru/types/event"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/types/quota"
	routerTypes "github.com/tsuru/tsuru/types/router"
	volumeTypes "github.com/tsuru/tsuru/types/volume"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	check "gopkg.in/check.v1"
)

func (s *S) TestGetAppByName(c *check.C) {
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	newApp := appTypes.App{Name: "my-app", Platform: "Django", TeamOwner: s.team.Name}
	err = CreateApp(context.TODO(), &newApp, s.user)
	c.Assert(err, check.IsNil)
	newApp.Env = map[string]bindTypes.EnvVar{}
	_, err = collection.ReplaceOne(context.TODO(), mongoBSON.M{"name": newApp.Name}, &newApp)
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
	a := appTypes.App{
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
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDelete,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
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
	a := appTypes.App{
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
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppUpdate,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	err = DeleteVersion(context.TODO(), &a, evt, strconv.Itoa(version2.Version()))
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.Provisioned(&a), check.Equals, false)
	err = servicemanager.UserQuota.Inc(context.TODO(), s.user, 1)
	c.Assert(err, check.IsNil)
}

func (s *S) TestDeleteAppWithNoneRouters(c *check.C) {
	a := appTypes.App{
		Name:      "myapp",
		Platform:  "go",
		Owner:     s.user.Email,
		TeamOwner: s.team.Name,
		Router:    "none",
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDelete,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	err = Delete(context.TODO(), &a, evt, "")
	c.Assert(err, check.IsNil)
}

func (s *S) TestDeleteWithBoundVolumes(c *check.C) {
	a := appTypes.App{
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
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDelete,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
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
	a := appTypes.App{
		Name:      "appname",
		Platform:  "python",
		TeamOwner: s.team.Name,
		Tags:      []string{"", " test a  ", "  ", "test b ", " test a "},
	}
	var teamQuotaIncCalled bool
	s.mockService.TeamQuota.OnInc = func(t *authTypes.Team, q int) error {
		teamQuotaIncCalled = true
		c.Assert(t.GetName(), check.Equals, s.team.Name)
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

	usersCollection, err := storagev2.UsersCollection()
	c.Assert(err, check.IsNil)

	_, err = usersCollection.UpdateOne(context.TODO(), mongoBSON.M{"email": s.user.Email}, mongoBSON.M{"$set": mongoBSON.M{"quota.limit": 1}})
	c.Assert(err, check.IsNil)

	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err = CreateApp(context.TODO(), &a, s.user)
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
	env := provision.EnvsForApp(retrievedApp)
	c.Assert(env["TSURU_APPNAME"].Value, check.Equals, a.Name)
	c.Assert(env["TSURU_APPNAME"].Public, check.Equals, true)
}

func (s *S) TestCreateAppAlreadyExists(c *check.C) {
	a := appTypes.App{
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

	usersCollection, err := storagev2.UsersCollection()
	c.Assert(err, check.IsNil)
	_, err = usersCollection.UpdateOne(context.TODO(), mongoBSON.M{"email": s.user.Email}, mongoBSON.M{"$set": mongoBSON.M{"quota.limit": 1}})
	c.Assert(err, check.IsNil)

	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	ra := appTypes.App{Name: "appname", Platform: "python", TeamOwner: s.team.Name, Pool: "invalid"}
	err = CreateApp(context.TODO(), &ra, s.user)
	c.Assert(err, check.DeepEquals, &appTypes.AppCreationError{App: ra.Name, Err: ErrAppAlreadyExists})
}

func (s *S) TestCreateAppDefaultPlan(c *check.C) {
	a := appTypes.App{
		Name:      "appname",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)

	usersCollection, err := storagev2.UsersCollection()
	c.Assert(err, check.IsNil)
	_, err = usersCollection.UpdateOne(context.TODO(), mongoBSON.M{"email": s.user.Email}, mongoBSON.M{"$set": mongoBSON.M{"quota.limit": 1}})
	c.Assert(err, check.IsNil)

	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	retrievedApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(retrievedApp.Plan, check.DeepEquals, s.defaultPlan)
}

func (s *S) TestCreateAppDefaultRouterForPool(c *check.C) {
	pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{
		PoolExpr: "pool1",
		Field:    pool.ConstraintTypeRouter,
		Values:   []string{"fake-tls", "fake"},
	})
	a := appTypes.App{
		Name:      "appname",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	usersCollection, err := storagev2.UsersCollection()
	c.Assert(err, check.IsNil)

	_, err = usersCollection.UpdateOne(context.TODO(), mongoBSON.M{"email": s.user.Email}, mongoBSON.M{"$set": mongoBSON.M{"quota.limit": 1}})
	c.Assert(err, check.IsNil)

	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	retrievedApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(GetRouters(retrievedApp), check.DeepEquals, []appTypes.AppRouter{{Name: "fake-tls", Opts: map[string]string{}}})
}

func (s *S) TestCreateAppDefaultPlanForPool(c *check.C) {
	s.plan = appTypes.Plan{Name: "large", Memory: 4194304}
	pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{
		PoolExpr: "pool1",
		Field:    pool.ConstraintTypePlan,
		Values:   []string{"large", "huge"},
	})
	a := appTypes.App{
		Name:      "appname",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)

	usersCollection, err := storagev2.UsersCollection()
	c.Assert(err, check.IsNil)

	_, err = usersCollection.UpdateOne(context.TODO(), mongoBSON.M{"email": s.user.Email}, mongoBSON.M{"$set": mongoBSON.M{"quota.limit": 1}})
	c.Assert(err, check.IsNil)

	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	retrievedApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(retrievedApp.Plan.Name, check.Equals, "large")
}

func (s *S) TestCreateAppDefaultPlanWildCardForPool(c *check.C) {
	s.plan = appTypes.Plan{Name: "large", Memory: 4194304}
	pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{
		PoolExpr: "pool1",
		Field:    pool.ConstraintTypePlan,
		Values:   []string{"l*", "huge"},
	})
	a := appTypes.App{
		Name:      "appname",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	usersCollection, err := storagev2.UsersCollection()
	c.Assert(err, check.IsNil)

	_, err = usersCollection.UpdateOne(context.TODO(), mongoBSON.M{"email": s.user.Email}, mongoBSON.M{"$set": mongoBSON.M{"quota.limit": 1}})
	c.Assert(err, check.IsNil)

	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	retrievedApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(retrievedApp.Plan.Name, check.Equals, "large")
}

func (s *S) TestCreateAppDefaultPlanWildCardNotMatchForPoolReturnError(c *check.C) {
	s.plan = appTypes.Plan{Name: "large", Memory: 4194304}
	pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{
		PoolExpr: "pool1",
		Field:    pool.ConstraintTypePlan,
		Values:   []string{"blah*"},
	})
	a := appTypes.App{
		Name:      "appname",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)

	usersCollection, err := storagev2.UsersCollection()
	c.Assert(err, check.IsNil)
	_, err = usersCollection.UpdateOne(context.TODO(), mongoBSON.M{"email": s.user.Email}, mongoBSON.M{"$set": mongoBSON.M{"quota.limit": 1}})
	c.Assert(err, check.IsNil)

	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err.Error(), check.Equals, "no plan found for pool")
}

func (s *S) TestCreateAppDefaultPlanWildCardDefaultPlan(c *check.C) {
	s.plan = appTypes.Plan{Name: "large", Memory: 4194304}
	pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{
		PoolExpr: "pool1",
		Field:    pool.ConstraintTypePlan,
		Values:   []string{"*"},
	})
	a := appTypes.App{
		Name:      "appname",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	usersCollection, err := storagev2.UsersCollection()
	c.Assert(err, check.IsNil)
	_, err = usersCollection.UpdateOne(context.TODO(), mongoBSON.M{"email": s.user.Email}, mongoBSON.M{"$set": mongoBSON.M{"quota.limit": 1}})
	c.Assert(err, check.IsNil)

	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err = CreateApp(context.TODO(), &a, s.user)
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
	a := appTypes.App{
		Name:      "appname",
		Platform:  "python",
		Plan:      appTypes.Plan{Name: "myplan"},
		TeamOwner: s.team.Name,
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)

	usersCollection, err := storagev2.UsersCollection()
	c.Assert(err, check.IsNil)
	_, err = usersCollection.UpdateOne(context.TODO(), mongoBSON.M{"email": s.user.Email}, mongoBSON.M{"$set": mongoBSON.M{"quota.limit": 1}})
	c.Assert(err, check.IsNil)

	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err = CreateApp(context.TODO(), &a, s.user)
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
	err := pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{
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
	a := appTypes.App{
		Name:      "appname",
		Platform:  "python",
		Plan:      appTypes.Plan{Name: "myplan"},
		TeamOwner: s.team.Name,
	}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.ErrorMatches, `App plan "myplan" is not allowed on pool "pool1"`)
}

func (s *S) TestCreateAppUserQuotaExceeded(c *check.C) {
	app := appTypes.App{Name: "america", Platform: "python", TeamOwner: s.team.Name}

	usersCollection, err := storagev2.UsersCollection()
	c.Assert(err, check.IsNil)
	_, err = usersCollection.UpdateOne(context.TODO(), mongoBSON.M{"email": s.user.Email}, mongoBSON.M{"$set": mongoBSON.M{"quota.limit": 1, "quota.inuse": 1}})
	c.Assert(err, check.IsNil)

	s.mockService.UserQuota.OnInc = func(item quota.QuotaItem, q int) error {
		c.Assert(item.GetName(), check.Equals, s.user.Email)
		return &quota.QuotaExceededError{Available: 0, Requested: 1}
	}
	err = CreateApp(context.TODO(), &app, s.user)
	e, ok := err.(*appTypes.AppCreationError)
	c.Assert(ok, check.Equals, true)
	qe, ok := e.Err.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(qe.Available, check.Equals, uint(0))
	c.Assert(qe.Requested, check.Equals, uint(1))
}

func (s *S) TestCreteAppTeamQuotaExceeded(c *check.C) {
	a := appTypes.App{Name: "my-app", Platform: "python", TeamOwner: "my-team"}
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
	s.mockService.TeamQuota.OnInc = func(t *authTypes.Team, delta int) error {
		teamQuotaIncCalled = true
		c.Assert(t.GetName(), check.Equals, a.TeamOwner)
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
	app := appTypes.App{Name: "america", Platform: "python", TeamOwner: "tsuruteam"}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	c.Assert(app.TeamOwner, check.Equals, "tsuruteam")
}

func (s *S) TestCreateAppTeamOwnerTeamNotFound(c *check.C) {
	app := appTypes.App{
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
	err := u.Create(context.TODO())
	c.Assert(err, check.IsNil)
	a := appTypes.App{Name: "beyond"}
	err = CreateApp(context.TODO(), &a, &u)
	c.Check(err, check.DeepEquals, &errors.ValidationError{Message: authTypes.ErrTeamNotFound.Error()})
}

func (s *S) TestCantCreateTwoAppsWithTheSameName(c *check.C) {
	err := CreateApp(context.TODO(), &appTypes.App{Name: "appname", TeamOwner: s.team.Name}, s.user)
	c.Assert(err, check.IsNil)
	a := appTypes.App{Name: "appname", Platform: "python", TeamOwner: s.team.Name}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.NotNil)
	e, ok := err.(*appTypes.AppCreationError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.App, check.Equals, "appname")
	c.Assert(e.Err, check.NotNil)
	c.Assert(e.Err.Error(), check.Equals, "there is already an app with this name")
}

func (s *S) TestCantCreateAppWithInvalidName(c *check.C) {
	a := appTypes.App{
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
	a := appTypes.App{
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
	a := appTypes.App{
		Name:      "my-app",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, &user)
	c.Assert(err, check.IsNil)
}

func (s *S) TestAddUnits(c *check.C) {
	ctx := context.Background()
	app := appTypes.App{
		Name: "warpaint", Platform: "python",
		Quota:     quota.UnlimitedQuota,
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app)
	err = AddUnits(ctx, &app, 5, "web", "", nil)
	c.Assert(err, check.IsNil)
	units, err := AppUnits(ctx, &app)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 5)
	err = AddUnits(ctx, &app, 2, "worker", "", nil)
	c.Assert(err, check.IsNil)
	units, err = AppUnits(ctx, &app)
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
	ctx := context.Background()
	a := appTypes.App{
		Name: "sejuani", Platform: "python",
		TeamOwner: s.team.Name,
		Quota:     quota.UnlimitedQuota,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	err = AddUnits(ctx, &a, 1, "web", "", nil)
	c.Assert(err, check.IsNil)
	err = Stop(context.TODO(), &a, nil, "web", "")
	c.Assert(err, check.IsNil)
	err = AddUnits(ctx, &a, 1, "web", "", nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot add units to an app that has stopped units")
	units, err := AppUnits(ctx, &a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
}

func (s *S) TestAddUnitsWithWriter(c *check.C) {
	ctx := context.Background()
	app := appTypes.App{
		Name: "warpaint", Platform: "python",
		Quota:     quota.UnlimitedQuota,
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	newSuccessfulAppVersion(c, &app)
	err = AddUnits(ctx, &app, 2, "web", "", &buf)
	c.Assert(err, check.IsNil)
	units, err := AppUnits(ctx, &app)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 2)
	for _, unit := range units {
		c.Assert(unit.AppName, check.Equals, app.Name)
	}
	c.Assert(buf.String(), check.Matches, "(?s)added 2 units.*")
}

func (s *S) TestAddUnitsQuota(c *check.C) {
	ctx := context.Background()
	app := appTypes.App{
		Name: "warpaint", Platform: "python",
		TeamOwner: s.team.Name, Quota: quota.Quota{Limit: 7, InUse: 0},
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	var inUseNow int
	s.mockService.AppQuota.OnInc = func(item *appTypes.App, quantity int) error {
		inUseNow++
		c.Assert(item.Name, check.Equals, app.Name)
		c.Assert(quantity, check.Equals, 1)
		return nil
	}
	newSuccessfulAppVersion(c, &app)
	otherApp := appTypes.App{Name: "warpaint"}
	for i := 1; i <= 7; i++ {
		err = AddUnits(ctx, &otherApp, 1, "web", "", nil)
		c.Assert(err, check.IsNil)
		c.Assert(inUseNow, check.Equals, i)
		units := s.provisioner.GetUnits(&app)
		c.Assert(units, check.HasLen, i)
	}
}

func (s *S) TestAddUnitsQuotaExceeded(c *check.C) {
	ctx := context.Background()
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	app := appTypes.App{
		Name: "warpaint", Platform: "ruby",
		TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake"}},
		Quota: quota.Quota{Limit: 7, InUse: 7},
	}
	s.mockService.AppQuota.OnInc = func(item *appTypes.App, quantity int) error {
		c.Assert(item.Name, check.Equals, app.Name)
		c.Assert(quantity, check.Equals, 1)
		return &quota.QuotaExceededError{Available: 0, Requested: 1}
	}
	_, err = collection.InsertOne(ctx, app)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app)
	err = AddUnits(ctx, &app, 1, "web", "", nil)
	e, ok := pkgErrors.Cause(err).(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Available, check.Equals, uint(0))
	c.Assert(e.Requested, check.Equals, uint(1))
	units := s.provisioner.GetUnits(&app)
	c.Assert(units, check.HasLen, 0)
}

func (s *S) TestAddUnitsMultiple(c *check.C) {
	ctx := context.Background()
	app := appTypes.App{
		Name: "warpaint", Platform: "ruby",
		TeamOwner: s.team.Name,
		Quota:     quota.Quota{Limit: 11, InUse: 0},
	}
	s.mockService.AppQuota.OnInc = func(item *appTypes.App, quantity int) error {
		c.Assert(item.Name, check.Equals, app.Name)
		c.Assert(quantity, check.Equals, 10)
		return nil
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app)
	err = AddUnits(ctx, &app, 10, "web", "", nil)
	c.Assert(err, check.IsNil)
	units := s.provisioner.GetUnits(&app)
	c.Assert(units, check.HasLen, 10)
}

func (s *S) TestAddZeroUnits(c *check.C) {
	ctx := context.Background()
	app := appTypes.App{Name: "warpaint", Platform: "ruby"}
	err := AddUnits(ctx, &app, 0, "web", "", nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot add zero units.")
}

func (s *S) TestAddUnitsFailureInProvisioner(c *check.C) {
	ctx := context.Background()

	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	app := appTypes.App{
		Name:      "scars",
		Platform:  "golang",
		Quota:     quota.UnlimitedQuota,
		TeamOwner: s.team.Name,
		Routers:   []appTypes.AppRouter{{Name: "fake"}},
	}
	_, err = collection.InsertOne(ctx, app)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app)
	err = AddUnits(ctx, &app, 2, "web", "", nil)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "(?s).*App is not provisioned.*")
}

func (s *S) TestAddUnitsIsAtomic(c *check.C) {
	ctx := context.Background()
	app := appTypes.App{
		Name: "warpaint", Platform: "golang",
		Quota: quota.UnlimitedQuota,
	}
	err := AddUnits(ctx, &app, 2, "web", "", nil)
	c.Assert(err, check.NotNil)
	_, err = GetByName(context.TODO(), app.Name)
	c.Assert(err, check.Equals, appTypes.ErrAppNotFound)
}

func (s *S) TestRemoveUnitsWithQuota(c *check.C) {
	ctx := context.Background()
	a := appTypes.App{
		Name:      "ble",
		TeamOwner: s.team.Name,
	}
	s.mockService.AppQuota.OnSetLimit = func(item *appTypes.App, quantity int) error {
		c.Assert(item.Name, check.Equals, a.Name)
		c.Assert(quantity, check.Equals, 6)
		return nil
	}
	s.mockService.AppQuota.OnInc = func(item *appTypes.App, quantity int) error {
		c.Assert(item.Name, check.Equals, a.Name)
		c.Assert(quantity, check.Equals, 4)
		return nil
	}
	s.mockService.AppQuota.OnGet = func(item *appTypes.App) (*quota.Quota, error) {
		c.Assert(item.Name, check.Equals, a.Name)
		return &quota.Quota{Limit: 6, InUse: 2}, nil
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = SetQuotaLimit(ctx, &a, 6)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 6, "web", newSuccessfulAppVersion(c, &a), nil)
	err = RemoveUnits(context.TODO(), &a, 4, "web", "", nil)
	c.Assert(err, check.IsNil)
	quota, err := servicemanager.AppQuota.Get(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	err = tsurutest.WaitCondition(2e9, func() bool {
		return quota.InUse == 2
	})
	c.Assert(err, check.IsNil)
}

func (s *S) TestRemoveUnits(c *check.C) {
	ctx := context.Background()
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := service.Create(context.TODO(), srvc)
	c.Assert(err, check.IsNil)
	app := appTypes.App{
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
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app)
	err = AddUnits(ctx, &app, 2, "web", "", nil)
	c.Assert(err, check.IsNil)
	err = AddUnits(ctx, &app, 2, "worker", "", nil)
	c.Assert(err, check.IsNil)
	err = AddUnits(ctx, &app, 2, "web", "", nil)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(nil)
	err = RemoveUnits(context.TODO(), &app, 2, "worker", "", buf)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Matches, "(?s)removing 2 units.*")
	err = tsurutest.WaitCondition(2e9, func() bool {
		units, inErr := AppUnits(ctx, &app)
		c.Assert(inErr, check.IsNil)
		return len(units) == 4
	})
	c.Assert(err, check.IsNil)
	ts.Close()
	units, err := AppUnits(ctx, &app)
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
	app := appTypes.App{
		Name:      "chemistryii",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &app, 3, "web", newSuccessfulAppVersion(c, &app), nil)
	for _, test := range tests {
		err := RemoveUnits(context.TODO(), &app, test.n, "web", "", nil)
		c.Check(err, check.ErrorMatches, "(?s).*"+test.expected+".*")
	}
}

func (s *S) TestGrantAccessFailsIfTheTeamAlreadyHasAccessToTheApp(c *check.C) {
	a := appTypes.App{Name: "app-name", Platform: "django", Teams: []string{s.team.Name}}
	err := Grant(context.TODO(), &a, &s.team)
	c.Assert(err, check.Equals, ErrAlreadyHaveAccess)
}

func (s *S) TestRevokeAccessDoesntLeaveOrphanApps(c *check.C) {
	app := appTypes.App{Name: "app-name", Platform: "django", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	err = Revoke(context.TODO(), &app, &s.team)
	c.Assert(err, check.Equals, ErrCannotOrphanApp)
}

func (s *S) TestRevokeAccessFailsIfTheTeamsDoesNotHaveAccessToTheApp(c *check.C) {
	a := appTypes.App{Name: "app-name", Platform: "django", Teams: []string{}}
	err := Revoke(context.TODO(), &a, &s.team)
	c.Assert(err, check.Equals, ErrNoAccess)
}

func (s *S) TestSetEnvNewAppsTheMapIfItIsNil(c *check.C) {
	a := appTypes.App{Name: "how-many-more-times"}
	c.Assert(a.Env, check.IsNil)
	env := bindTypes.EnvVar{Name: "PATH", Value: "/"}
	setEnv(&a, env)
	c.Assert(a.Env, check.NotNil)
}

func (s *S) TestSetPublicEnvironmentVariableToApp(c *check.C) {
	a := appTypes.App{Name: "app-name", Platform: "django"}
	setEnv(&a, bindTypes.EnvVar{Name: "PATH", Value: "/", Public: true})
	env := a.Env["PATH"]
	c.Assert(env.Name, check.Equals, "PATH")
	c.Assert(env.Value, check.Equals, "/")
	c.Assert(env.Public, check.Equals, true)
}

func (s *S) TestSetPrivateEnvironmentVariableToApp(c *check.C) {
	a := appTypes.App{Name: "app-name", Platform: "django"}
	setEnv(&a, bindTypes.EnvVar{Name: "PATH", Value: "/", Public: false})
	env := a.Env["PATH"]
	c.Assert(env.Name, check.Equals, "PATH")
	c.Assert(env.Value, check.Equals, "/")
	c.Assert(env.Public, check.Equals, false)
}

func (s *S) TestSetMultiplePublicEnvironmentVariableToApp(c *check.C) {
	a := appTypes.App{Name: "app-name", Platform: "django"}
	setEnv(&a, bindTypes.EnvVar{Name: "PATH", Value: "/", Public: true})
	setEnv(&a, bindTypes.EnvVar{Name: "DATABASE", Value: "mongodb", Public: true})
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
	a := appTypes.App{Name: "app-name", Platform: "django"}
	setEnv(&a, bindTypes.EnvVar{Name: "PATH", Value: "/", Public: false})
	setEnv(&a, bindTypes.EnvVar{Name: "DATABASE", Value: "mongodb", Public: false})
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
	ctx := context.Background()
	a := appTypes.App{
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
	err = AddUnits(ctx, &a, 1, "web", "", nil)
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
	err = SetEnvs(ctx, &a, bindTypes.SetEnvArgs{
		Envs:          envs,
		ShouldRestart: true,
		Writer:        &buf,
	})
	c.Assert(err, check.ErrorMatches, "Environment variable \"DATABASE_HOST\" is already in use by service bind \"service/instance\"")
	newApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	expected := map[string]bindTypes.EnvVar{
		"DATABASE_HOST": {
			Name:      "DATABASE_HOST",
			Value:     "localhost",
			Public:    false,
			ManagedBy: "service/instance",
		},
	}
	newAppEnvs := provision.EnvsForApp(newApp)
	delete(newAppEnvs, tsuruEnvs.TsuruServicesEnvVar)
	delete(newAppEnvs, "TSURU_APPNAME")
	delete(newAppEnvs, "TSURU_APPDIR")

	c.Assert(newAppEnvs, check.DeepEquals, expected)
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 0)
	c.Assert(buf.String(), check.Equals, "---- Setting 2 new environment variables ----\n---- environment variables have conflicts with service binds: Environment variable \"DATABASE_HOST\" is already in use by service bind \"service/instance\" ----\n")
}

func (s *S) TestSetEnvWithNoRestartFlag(c *check.C) {
	ctx := context.Background()
	a := appTypes.App{
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
	err = SetEnvs(ctx, &a, bindTypes.SetEnvArgs{
		Envs:          envs,
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	newApp, err := GetByName(ctx, a.Name)
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
	ctx := context.Background()
	a := appTypes.App{
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
	err = SetEnvs(ctx, &a, bindTypes.SetEnvArgs{
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
	ctx := context.Background()
	a := appTypes.App{
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
		err = SetEnvs(ctx, &a, bindTypes.SetEnvArgs{Envs: envs})
		if test.isValid {
			c.Check(err, check.IsNil)
		} else {
			c.Check(err, check.ErrorMatches, fmt.Sprintf("Invalid environment variable name: '%s'", test.envName))
		}
	}
}

func (s *S) TestUnsetEnvKeepServiceVariables(c *check.C) {
	ctx := context.Background()
	a := appTypes.App{
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
	err = AddUnits(ctx, &a, 1, "web", "", nil)
	c.Assert(err, check.IsNil)
	err = UnsetEnvs(ctx, &a, bindTypes.UnsetEnvArgs{
		VariableNames: []string{"DATABASE_HOST", "DATABASE_PASSWORD"},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	newApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	expected := map[string]bindTypes.EnvVar{
		"DATABASE_HOST": {
			Name:      "DATABASE_HOST",
			Value:     "localhost",
			Public:    false,
			ManagedBy: "s1/si1",
		},

		"TSURU_APPNAME": {
			Name:      "TSURU_APPNAME",
			Value:     "myapp",
			Public:    true,
			ManagedBy: "tsuru",
		},

		"TSURU_APPDIR": {
			Name:      "TSURU_APPDIR",
			Value:     "/home/application/current",
			Public:    true,
			ManagedBy: "tsuru",
		},
	}
	newAppEnvs := provision.EnvsForApp(newApp)
	delete(newAppEnvs, tsuruEnvs.TsuruServicesEnvVar)
	c.Assert(newAppEnvs, check.DeepEquals, expected)
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 1)
	c.Assert(s.provisioner.RestartsByVersion(&a, ""), check.Equals, 1)
}

func (s *S) TestUnsetEnvWithNoRestartFlag(c *check.C) {
	ctx := context.Background()
	a := appTypes.App{
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
	err = AddUnits(ctx, &a, 1, "web", "", nil)
	c.Assert(err, check.IsNil)
	err = UnsetEnvs(ctx, &a, bindTypes.UnsetEnvArgs{
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
	ctx := context.Background()
	a := appTypes.App{
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
	err = UnsetEnvs(ctx, &a, bindTypes.UnsetEnvArgs{
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
	a := appTypes.App{Name: "hi-there", ServiceEnvs: envs}
	c.Assert(InstanceEnvs(&a, "srv1", "mysql"), check.DeepEquals, expected)
}

func (s *S) TestInstanceEnvironmentDoesNotPanicIfTheEnvMapIsNil(c *check.C) {
	a := appTypes.App{Name: "hi-there"}
	c.Assert(InstanceEnvs(&a, "srv1", "mysql"), check.DeepEquals, map[string]bindTypes.EnvVar{})
}

func (s *S) TestAddCName(c *check.C) {
	app := &appTypes.App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), app, s.user)
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), app, "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	app, err = GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{"ktulu.mycompany.com"})
	err = AddCName(context.TODO(), app, "ktulu2.mycompany.com")
	c.Assert(err, check.IsNil)
	app, err = GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{"ktulu.mycompany.com", "ktulu2.mycompany.com"})
}

func (s *S) TestAddCNameCantBeDuplicatedWithSameRouter(c *check.C) {
	app := &appTypes.App{Name: "ktulu", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake"}, {Name: "fake-tls"}}}
	err := CreateApp(context.TODO(), app, s.user)
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), app, "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), app, "ktulu.mycompany.com")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cname ktulu.mycompany.com already exists for this app")
	app2 := &appTypes.App{Name: "ktulu2", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}}
	err = CreateApp(context.TODO(), app2, s.user)
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), app2, "ktulu.mycompany.com")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cname ktulu.mycompany.com already exists for app ktulu using same router")
}

func (s *S) TestAddCNameErrorForDifferentTeamOwners(c *check.C) {
	app := &appTypes.App{Name: "ktulu", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake"}}}
	app2 := &appTypes.App{Name: "ktulu2", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}}
	err := CreateApp(context.TODO(), app, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), app2, s.user)
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), app, "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	app2.TeamOwner = "some-other-team"
	err = AddCName(context.TODO(), app2, "ktulu.mycompany.com")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cname ktulu.mycompany.com already exists for another app ktulu and belongs to a different team owner")
}

func (s *S) TestAddCNameDifferentAppsNoRouter(c *check.C) {
	app := &appTypes.App{Name: "ktulu", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{}}
	app2 := &appTypes.App{Name: "ktulu2", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}}
	err := CreateApp(context.TODO(), app, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), app2, s.user)
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), app, "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), app2, "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
}

func (s *S) TestAddCNameWithWildCard(c *check.C) {
	app := &appTypes.App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), app, s.user)
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), app, "*.mycompany.com")
	c.Assert(err, check.IsNil)
	app, err = GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{"*.mycompany.com"})
}

func (s *S) TestAddCNameErrsOnEmptyCName(c *check.C) {
	app := &appTypes.App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), app, s.user)
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), app, "")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Invalid cname")
}

func (s *S) TestAddCNameErrsOnInvalid(c *check.C) {
	app := &appTypes.App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), app, s.user)
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), app, "_ktulu.mycompany.com")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Invalid cname")
}

func (s *S) TestAddCNamePartialUpdate(c *check.C) {
	a := &appTypes.App{Name: "master", Platform: "puppet", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	other := appTypes.App{Name: a.Name, Routers: []appTypes.AppRouter{{Name: "fake"}}}
	err = AddCName(context.TODO(), &other, "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(a.Platform, check.Equals, "puppet")
	c.Assert(a.Name, check.Equals, "master")
	c.Assert(a.CName, check.DeepEquals, []string{"ktulu.mycompany.com"})
}

func (s *S) TestAddCNameUnknownApp(c *check.C) {
	a := appTypes.App{Name: "ktulu", TeamOwner: s.team.Name}
	err := AddCName(context.TODO(), &a, "ktulu.mycompany.com")
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
	a := appTypes.App{Name: "live-to-die", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	for _, t := range data {
		err := AddCName(context.TODO(), &a, t.input)
		if !t.valid {
			c.Check(err.Error(), check.Equals, "Invalid cname")
		} else {
			c.Check(err, check.IsNil)
		}
	}
}

func (s *S) TestAddCNameCallsRouterSetCName(c *check.C) {
	a := appTypes.App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), &a, "ktulu.mycompany.com", "ktulu2.mycompany.com")
	c.Assert(err, check.IsNil)
	hasCName := routertest.FakeRouter.HasCNameFor(a.Name, "ktulu.mycompany.com")
	c.Assert(hasCName, check.Equals, true)
	hasCName = routertest.FakeRouter.HasCNameFor(a.Name, "ktulu2.mycompany.com")
	c.Assert(hasCName, check.Equals, true)
}

func (s *S) TestAddCnameRollbackWithDuplicatedCName(c *check.C) {
	a := appTypes.App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), &a, "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), &a, "ktulu2.mycompany.com", "ktulu3.mycompany.com", "ktulu.mycompany.com")
	c.Assert(err, check.NotNil)
	c.Assert(a.CName, check.DeepEquals, []string{"ktulu.mycompany.com"})
	hasCName := routertest.FakeRouter.HasCNameFor(a.Name, "ktulu2.mycompany.com")
	c.Assert(hasCName, check.Equals, false)
	hasCName = routertest.FakeRouter.HasCNameFor(a.Name, "ktulu3.mycompany.com")
	c.Assert(hasCName, check.Equals, false)
}

func (s *S) TestAddCnameRollbackWithInvalidCName(c *check.C) {
	a := appTypes.App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), &a, "ktulu2.mycompany.com")
	c.Assert(err, check.IsNil)
	invalidCName := "-------"
	err = AddCName(context.TODO(), &a, "ktulu3.mycompany.com", invalidCName)
	c.Assert(err, check.NotNil)
	c.Assert(a.CName, check.DeepEquals, []string{"ktulu2.mycompany.com"})
	hasCName := routertest.FakeRouter.HasCNameFor(a.Name, "ktulu3.mycompany.com")
	c.Assert(hasCName, check.Equals, false)
}

func (s *S) TestAddCnameRollbackWithRouterFailure(c *check.C) {
	a1 := appTypes.App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a1, s.user)
	c.Assert(err, check.IsNil)

	a2 := appTypes.App{Name: "ktulu3", TeamOwner: s.team.Name}
	err = CreateApp(context.TODO(), &a2, s.user)
	c.Assert(err, check.IsNil)

	err = AddCName(context.TODO(), &a1, "ktulu2.mycompany.com")
	c.Assert(err, check.IsNil)

	err = AddCName(context.TODO(), &a2, "ktulu3.mycompany.com")
	c.Assert(err, check.IsNil)

	err = AddCName(context.TODO(), &a1, "ktulu3.mycompany.com")
	c.Assert(err, check.ErrorMatches, "cname ktulu3.mycompany.com already exists for app ktulu3 using same router")
	c.Assert(a1.CName, check.DeepEquals, []string{"ktulu2.mycompany.com"})
	hasCName := routertest.FakeRouter.HasCNameFor(a1.Name, "ktulu3.mycompany.com")
	c.Assert(hasCName, check.Equals, false)
}

func (s *S) TestAddCnameRollbackWithDatabaseFailure(c *check.C) {
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := appTypes.App{Name: "ktulu", TeamOwner: s.team.Name}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), &a, "ktulu2.mycompany.com")
	c.Assert(err, check.IsNil)
	collection.DeleteOne(context.TODO(), mongoBSON.M{"name": a.Name})
	err = AddCName(context.TODO(), &a, "ktulu3.mycompany.com")
	c.Assert(err, check.NotNil)
	hasCName := routertest.FakeRouter.HasCNameFor(a.Name, "ktulu3.mycompany.com")
	c.Assert(hasCName, check.Equals, false)
}

func (s *S) TestRemoveCNameRollback(c *check.C) {
	a := appTypes.App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), &a, "ktulu2.mycompany.com", "ktulu3.mycompany.com", "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = RemoveCName(context.TODO(), &a, "ktulu2.mycompany.com", "test.com")
	c.Assert(err, check.NotNil)
	c.Assert(a.CName, check.DeepEquals, []string{"ktulu2.mycompany.com", "ktulu3.mycompany.com", "ktulu.mycompany.com"})
	hasCName := routertest.FakeRouter.HasCNameFor(a.Name, "ktulu2.mycompany.com")
	c.Assert(hasCName, check.Equals, true)

	routertest.FakeRouter.FailuresByHost["ktulu3.mycompany.com"] = true
	err = RemoveCName(context.TODO(), &a, "ktulu2.mycompany.com")
	c.Assert(err, check.ErrorMatches, "Forced failure")
	c.Assert(a.CName, check.DeepEquals, []string{"ktulu2.mycompany.com", "ktulu3.mycompany.com", "ktulu.mycompany.com"})
}

func (s *S) TestRemoveCNameRemovesFromDatabase(c *check.C) {
	a := &appTypes.App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), a, "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = RemoveCName(context.TODO(), a, "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(a.CName, check.DeepEquals, []string{})
}

func (s *S) TestRemoveCNameWhichNoExists(c *check.C) {
	a := &appTypes.App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = RemoveCName(context.TODO(), a, "ktulu.mycompany.com")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cname ktulu.mycompany.com not exists in app")
}

func (s *S) TestRemoveMoreThanOneCName(c *check.C) {
	a := &appTypes.App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), a, "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), a, "ktulu2.mycompany.com")
	c.Assert(err, check.IsNil)
	err = RemoveCName(context.TODO(), a, "ktulu.mycompany.com", "ktulu2.mycompany.com")
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(a.CName, check.DeepEquals, []string{})
}

func (s *S) TestRemoveCNameRemovesFromRouter(c *check.C) {
	a := appTypes.App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), &a, "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = RemoveCName(context.TODO(), &a, "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	hasCName := routertest.FakeRouter.HasCNameFor(a.Name, "ktulu.mycompany.com")
	c.Assert(hasCName, check.Equals, false)
}

func (s *S) TestRemoveCNameAlsoRemovesCertIssuer(c *check.C) {
	a := appTypes.App{
		Name:      "ktulu",
		TeamOwner: s.team.Name,
		CName:     []string{"ktulu.mycompany.com"},
		CertIssuers: map[string]string{
			"ktulu.mycompany.com": "issuer",
		},
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = RemoveCName(context.TODO(), &a, "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	hasCName := routertest.FakeRouter.HasCNameFor(a.Name, "ktulu.mycompany.com")
	c.Assert(hasCName, check.Equals, false)
	hasIssuer := routertest.FakeRouter.HasCertIssuerForCName(a.Name, "ktulu.mycompany.com", "issuer")
	c.Assert(hasIssuer, check.Equals, false)
}

func (s *S) TestSetCertIssuer(c *check.C) {
	a := appTypes.App{
		Name:      "ktulu",
		TeamOwner: s.team.Name,
		CName:     []string{"ktulu.mycompany.com"},
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = SetCertIssuer(context.TODO(), &a, "ktulu.mycompany.com", "issuer")
	c.Assert(err, check.IsNil)
	hasIssuer := routertest.FakeRouter.HasCertIssuerForCName(a.Name, "ktulu.mycompany.com", "issuer")
	c.Assert(hasIssuer, check.Equals, true)
}

func (s *S) TestSetCertIssuerWithConstraints(c *check.C) {
	a := appTypes.App{
		Name:      "ktulu",
		TeamOwner: s.team.Name,
		CName:     []string{"ktulu.mycompany.com"},
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{
		PoolExpr:  "pool1",
		Field:     pool.ConstraintTypeCertIssuer,
		Values:    []string{"CorrectIssuer"},
		Blacklist: false,
	})
	c.Assert(err, check.IsNil)
	err = SetCertIssuer(context.TODO(), &a, "ktulu.mycompany.com", "InvalidIssuer")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cert issuer not allowed by constraints of this pool. allowed values: CorrectIssuer")
}

func (s *S) TestSetCertIssuerWithBlacklistConstraints(c *check.C) {
	a := appTypes.App{
		Name:      "ktulu",
		TeamOwner: s.team.Name,
		CName:     []string{"ktulu.mycompany.com"},
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{
		PoolExpr:  "pool1",
		Field:     pool.ConstraintTypeCertIssuer,
		Values:    []string{"InvalidIssuer"},
		Blacklist: true,
	})
	c.Assert(err, check.IsNil)
	err = SetCertIssuer(context.TODO(), &a, "ktulu.mycompany.com", "InvalidIssuer")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cert issuer not allowed by constraints of this pool. not allowed values: InvalidIssuer")
}

func (s *S) TestSetCertIssuerWithInvalidCName(c *check.C) {
	a := appTypes.App{
		Name:      "ktulu",
		TeamOwner: s.team.Name,
		CName:     []string{"ktulu.mycompany.com"},
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = SetCertIssuer(context.TODO(), &a, "invalid.mycompany.com", "issuer")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cname does not exist in app")
}

func (s *S) TestUnsetCertIssuer(c *check.C) {
	a := appTypes.App{
		Name:      "ktulu",
		TeamOwner: s.team.Name,
		CName:     []string{"ktulu.mycompany.com"},
		CertIssuers: map[string]string{
			"ktulu.mycompany.com": "issuer",
		},
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = UnsetCertIssuer(context.TODO(), &a, "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	hasIssuer := routertest.FakeRouter.HasCertIssuerForCName(a.Name, "ktulu.mycompany.com", "issuer")
	c.Assert(hasIssuer, check.Equals, false)
}

func (s *S) TestAddInstanceFirst(c *check.C) {
	a := &appTypes.App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
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
	allEnvs := provision.EnvsForApp(a)
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
			Name:      "DATABASE_HOST",
			Value:     "localhost",
			ManagedBy: "srv1/myinstance",
			Public:    false,
		},
		"DATABASE_PORT": {
			Name:      "DATABASE_PORT",
			Value:     "3306",
			ManagedBy: "srv1/myinstance",
			Public:    false,
		},
		"DATABASE_USER": {
			Name:      "DATABASE_USER",
			Value:     "root",
			ManagedBy: "srv1/myinstance",
			Public:    false,
		},
	})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 0)
}

func (s *S) TestAddInstanceDuplicated(c *check.C) {
	a := &appTypes.App{Name: "sith", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "ZMQ_PEER", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "srv1"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	// inserts duplicated
	err = AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "ZMQ_PEER", Value: "8.8.8.8"}, InstanceName: "myinstance", ServiceName: "srv1"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	allEnvs := provision.EnvsForApp(a)
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
		Name:      "ZMQ_PEER",
		Value:     "8.8.8.8",
		ManagedBy: "srv1/myinstance",
		Public:    false,
	})
}

func (s *S) TestAddInstanceWithUnits(c *check.C) {
	a := &appTypes.App{Name: "dark", Quota: quota.Quota{Limit: 10}, TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, a)
	err = AddUnits(context.TODO(), a, 1, "web", "", nil)
	c.Assert(err, check.IsNil)
	err = AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "myservice"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := provision.EnvsForApp(a)
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
		Name:      "DATABASE_HOST",
		Value:     "localhost",
		ManagedBy: "myservice/myinstance",
		Public:    false,
	})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 1)
	c.Assert(s.provisioner.RestartsByVersion(a, ""), check.Equals, 1)
}

func (s *S) TestAddInstanceWithUnitsNoRestart(c *check.C) {
	a := &appTypes.App{Name: "dark", Quota: quota.Quota{Limit: 10}, TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, a)
	err = AddUnits(context.TODO(), a, 1, "web", "", nil)
	c.Assert(err, check.IsNil)
	err = AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "myservice"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := provision.EnvsForApp(a)
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
		Name:      "DATABASE_HOST",
		Value:     "localhost",
		ManagedBy: "myservice/myinstance",
		Public:    false,
	})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 0)
}

func (s *S) TestAddInstanceMultipleServices(c *check.C) {
	a := &appTypes.App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "host1"}, InstanceName: "instance1", ServiceName: "mysql"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "host2"}, InstanceName: "instance2", ServiceName: "mysql"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "host3"}, InstanceName: "instance3", ServiceName: "mongodb"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	allEnvs := provision.EnvsForApp(a)
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
		Name:      "DATABASE_HOST",
		Value:     "host3",
		ManagedBy: "mongodb/instance3",
		Public:    false,
	})
}

func (s *S) TestAddInstanceAndRemoveInstanceMultipleServices(c *check.C) {
	a := &appTypes.App{Name: "fuchsia", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "host1"}, InstanceName: "instance1", ServiceName: "mysql"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_HOST", Value: "host2"}, InstanceName: "instance2", ServiceName: "mysql"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	allEnvs := provision.EnvsForApp(a)
	c.Assert(allEnvs["DATABASE_HOST"], check.DeepEquals, bindTypes.EnvVar{
		Name:      "DATABASE_HOST",
		Value:     "host2",
		ManagedBy: "mysql/instance2",
		Public:    false,
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
	err = RemoveInstance(context.TODO(), a, bindTypes.RemoveInstanceArgs{
		ServiceName:   "mysql",
		InstanceName:  "instance2",
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	allEnvs = provision.EnvsForApp(a)
	c.Assert(allEnvs["DATABASE_HOST"], check.DeepEquals, bindTypes.EnvVar{
		Name:      "DATABASE_HOST",
		Value:     "host1",
		ManagedBy: "mysql/instance1",
		Public:    false,
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
	a := &appTypes.App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, InstanceName: "mydb", ServiceName: "mysql"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = RemoveInstance(context.TODO(), a, bindTypes.RemoveInstanceArgs{
		ServiceName:   "mysql",
		InstanceName:  "mydb",
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	allEnvs := provision.EnvsForApp(a)
	c.Assert(allEnvs["DATABASE_HOST"], check.DeepEquals, bindTypes.EnvVar{})
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(allEnvs[tsuruEnvs.TsuruServicesEnvVar].Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 0)
}

func (s *S) TestRemoveInstanceShifts(c *check.C) {
	a := &appTypes.App{Name: "dark", TeamOwner: s.team.Name}
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
		err = AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
			Envs:          []bindTypes.ServiceEnvVar{env},
			ShouldRestart: false,
		})
		c.Assert(err, check.IsNil)
	}
	err = RemoveInstance(context.TODO(), a, bindTypes.RemoveInstanceArgs{
		ServiceName:   "mysql",
		InstanceName:  "hisdb",
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	allEnvs := provision.EnvsForApp(a)
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
		Name:      "DATABASE_NAME",
		Value:     "ourdb",
		ManagedBy: "mysql/ourdb",
	})
}

func (s *S) TestRemoveInstanceNotFound(c *check.C) {
	a := &appTypes.App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
		Envs:          []bindTypes.ServiceEnvVar{{EnvVar: bindTypes.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, InstanceName: "mydb", ServiceName: "mysql"}},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = RemoveInstance(context.TODO(), a, bindTypes.RemoveInstanceArgs{
		ServiceName:   "mysql",
		InstanceName:  "yourdb",
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := provision.EnvsForApp(a)
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
		Name:      "DATABASE_NAME",
		Value:     "mydb",
		ManagedBy: "mysql/mydb",
	})
}

func (s *S) TestRemoveInstanceServiceNotFound(c *check.C) {
	a := &appTypes.App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	err = AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
		Envs:          []bindTypes.ServiceEnvVar{{EnvVar: bindTypes.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, InstanceName: "mydb", ServiceName: "mysql"}},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = RemoveInstance(context.TODO(), a, bindTypes.RemoveInstanceArgs{
		ServiceName:   "mongodb",
		InstanceName:  "mydb",
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := provision.EnvsForApp(a)
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
		Name:      "DATABASE_NAME",
		Value:     "mydb",
		ManagedBy: "mysql/mydb",
	})
}

func (s *S) TestRemoveInstanceWithUnits(c *check.C) {
	a := &appTypes.App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, a)
	err = AddUnits(context.TODO(), a, 1, "web", "", nil)
	c.Assert(err, check.IsNil)
	err = AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
		Envs:          []bindTypes.ServiceEnvVar{{EnvVar: bindTypes.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, InstanceName: "mydb", ServiceName: "mysql"}},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = RemoveInstance(context.TODO(), a, bindTypes.RemoveInstanceArgs{
		ServiceName:   "mysql",
		InstanceName:  "mydb",
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := provision.EnvsForApp(a)
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(allEnvs[tsuruEnvs.TsuruServicesEnvVar].Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{})
	c.Assert(allEnvs["DATABASE_NAME"], check.DeepEquals, bindTypes.EnvVar{})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 1)
	c.Assert(s.provisioner.RestartsByVersion(a, ""), check.Equals, 1)
}

func (s *S) TestRemoveInstanceWithUnitsNoRestart(c *check.C) {
	a := &appTypes.App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, a)
	err = AddUnits(context.TODO(), a, 1, "web", "", nil)
	c.Assert(err, check.IsNil)
	err = AddInstance(context.TODO(), a, bindTypes.AddInstanceArgs{
		Envs:          []bindTypes.ServiceEnvVar{{EnvVar: bindTypes.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, InstanceName: "mydb", ServiceName: "mysql"}},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = RemoveInstance(context.TODO(), a, bindTypes.RemoveInstanceArgs{
		ServiceName:   "mysql",
		InstanceName:  "mydb",
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := provision.EnvsForApp(a)
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
	err := pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{
		PoolExpr:  "pool1",
		Field:     pool.ConstraintTypeTeam,
		Values:    []string{teamName},
		Blacklist: true,
	})
	c.Assert(err, check.IsNil)
	err = pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{
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
		a := appTypes.App{Name: d.name, Plan: appTypes.Plan{Name: d.plan}, TeamOwner: d.teamOwner, Pool: d.pool, Routers: []appTypes.AppRouter{{Name: d.router}}}
		if valid := validateNew(context.TODO(), &a); valid != nil && valid.Error() != d.expected {
			c.Errorf("Is %q a valid app? Expected: %v. Got: %v.", d.name, d.expected, valid)
		}
	}
}

func (s *S) TestRestart(c *check.C) {
	s.provisioner.PrepareOutput([]byte("not yaml")) // loadConf
	a := appTypes.App{
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
	err = Restart(context.TODO(), &a, "", "", &b)
	c.Assert(err, check.IsNil)
	c.Assert(b.String(), check.Matches, `(?s).*---- Restarting the app "someapp" ----.*`)
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 1)
	c.Assert(s.provisioner.RestartsByVersion(&a, ""), check.Equals, 1)
}

func (s *S) TestRestartWithVersion(c *check.C) {
	s.provisioner.PrepareOutput([]byte("not yaml")) // loadConf
	a := appTypes.App{
		Name:      "someapp",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Routers:   []appTypes.AppRouter{{Name: "fake"}},
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	version := newSuccessfulAppVersion(c, &a)
	versionString := strconv.Itoa(version.Version())
	var b bytes.Buffer
	err = Restart(context.TODO(), &a, "", versionString, &b)
	c.Assert(err, check.IsNil)
	c.Assert(b.String(), check.Matches, `(?s).*---- Restarting the app "someapp" ----.*`)
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 1)
	c.Assert(s.provisioner.RestartsByVersion(&a, ""), check.Equals, 0)
	c.Assert(s.provisioner.RestartsByVersion(&a, versionString), check.Equals, 1)
}

func (s *S) TestStop(c *check.C) {
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := appTypes.App{Name: "app", TeamOwner: s.team.Name}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	newSuccessfulAppVersion(c, &a)
	err = AddUnits(context.TODO(), &a, 2, "web", "", nil)
	c.Assert(err, check.IsNil)
	err = Stop(context.TODO(), &a, &buf, "", "")
	c.Assert(err, check.IsNil)
	err = collection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&a)
	c.Assert(err, check.IsNil)
	units, err := AppUnits(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 2)
	for _, u := range units {
		c.Assert(u.Status, check.Equals, provTypes.UnitStatusStopped)
	}
}

func (s *S) TestStopPastUnits(c *check.C) {
	a := appTypes.App{Name: "app", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	version := newSuccessfulAppVersion(c, &a)
	err = AddUnits(context.TODO(), &a, 2, "web", strconv.Itoa(version.Version()), nil)
	c.Assert(err, check.IsNil)
	err = Stop(context.TODO(), &a, &buf, "", "")
	c.Assert(err, check.IsNil)
	var updatedVersion appTypes.AppVersion
	updatedVersion, err = getVersion(context.TODO(), &a, strconv.Itoa(version.Version()))
	c.Assert(err, check.IsNil)
	pastUnits := updatedVersion.VersionInfo().PastUnits
	c.Assert(pastUnits, check.HasLen, 1)
	c.Assert(pastUnits, check.DeepEquals, map[string]int{"web": 2})
}

func (s *S) TestLastLogs(c *check.C) {
	app := appTypes.App{
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
	logs, err := LastLogs(context.TODO(), &app, servicemanager.LogService, appTypes.ListLogArgs{
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
	app := appTypes.App{
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
	logs, err := LastLogs(context.TODO(), &app, servicemanager.LogService, appTypes.ListLogArgs{
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

func (p *logDisabledFakeProvisioner) LogsEnabled(app *appTypes.App) (bool, string, error) {
	return false, "my doc msg", nil
}

func (s *S) TestLastLogsDisabled(c *check.C) {
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "log-disabled"
	provision.Register("log-disabled", func() (provision.Provisioner, error) {
		return &logDisabledFakeProvisioner{}, nil
	})
	defer provision.Unregister("log-disabled")
	app := appTypes.App{
		Name:     "app3",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	_, err = collection.InsertOne(context.TODO(), app)
	c.Assert(err, check.IsNil)
	_, err = LastLogs(context.TODO(), &app, servicemanager.LogService, appTypes.ListLogArgs{
		Limit: 10,
	})
	c.Assert(err, check.ErrorMatches, "my doc msg")
}

func (s *S) TestAppMarshalJSON(c *check.C) {
	config.Set("apps:dashboard-url:template", "http://mydashboard.com/pools/{{ .Pool }}/apps/{{ .Name }}")
	defer config.Unset("apps:dashboard-url:template")

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

	app := appTypes.App{
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
	}
	err = CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)

	newSuccessfulAppVersion(c, &app)
	_, err = s.provisioner.AddUnitsToNode(&app, 1, "web", nil, "addr1", nil)
	c.Assert(err, check.IsNil)

	err = routertest.FakeRouter.EnsureBackend(context.TODO(), &app, router.EnsureBackendOpts{})
	c.Assert(err, check.IsNil)

	units, err := AppUnits(context.TODO(), &app)
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
				"Domain":     "name-web.fake-cluster.local",
				"Protocol":   "TCP",
				"Port":       float64(80),
				"TargetPort": float64(8080),
				"Process":    "web",
				"Version":    "",
			},
			map[string]interface{}{
				"Domain":     "name-logs.fake-cluster.local",
				"Protocol":   "UDP",
				"Port":       float64(12201),
				"TargetPort": float64(12201),
				"Process":    "logs",
				"Version":    "",
			},
			map[string]interface{}{
				"Domain":     "name-logs-v2.fake-cluster.local",
				"Protocol":   "UDP",
				"Port":       float64(12201),
				"TargetPort": float64(12201),
				"Process":    "logs",
				"Version":    "2",
			},
			map[string]interface{}{
				"Domain":     "name-web-v2.fake-cluster.local",
				"Protocol":   "TCP",
				"Port":       float64(80),
				"TargetPort": float64(8080),
				"Process":    "web",
				"Version":    "2",
			},
		},
		"provisioner": "fake",
		"cname":       []interface{}{"name.mycompany.com"},
		"owner":       s.user.Email,
		"deploys":     float64(7),
		"pool":        "test",
		"description": "description",
		"teamowner":   "myteam",
		"plan": map[string]interface{}{
			"name":     "myplan",
			"memory":   float64(64),
			"cpumilli": float64(0),
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
		"dashboardURL":         "http://mydashboard.com/pools/test/apps/name",
	}
	appInfo, err := AppInfo(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	data, err := json.Marshal(appInfo)
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
	app := appTypes.App{
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
	}
	err = AutoScale(context.TODO(), &app, provTypes.AutoScaleSpec{Process: "p1"})
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
				"Domain":     "name-web.fake-cluster.local",
				"Port":       float64(80),
				"TargetPort": float64(8080),
				"Process":    "web",
				"Protocol":   "TCP",
				"Version":    "",
			},
			map[string]interface{}{
				"Domain":     "name-logs.fake-cluster.local",
				"Port":       float64(12201),
				"TargetPort": float64(12201),
				"Process":    "logs",
				"Protocol":   "UDP",
				"Version":    "",
			},
			map[string]interface{}{
				"Domain":     "name-logs-v2.fake-cluster.local",
				"Port":       float64(12201),
				"TargetPort": float64(12201),
				"Process":    "logs",
				"Protocol":   "UDP",
				"Version":    "2",
			},
			map[string]interface{}{
				"Domain":     "name-web-v2.fake-cluster.local",
				"Port":       float64(80),
				"TargetPort": float64(8080),
				"Process":    "web",
				"Protocol":   "TCP",
				"Version":    "2",
			},
		},
		"provisioner": "fake",
		"cname":       []interface{}{"name.mycompany.com"},
		"owner":       "appOwner",
		"deploys":     float64(7),
		"pool":        "test",
		"description": "description",
		"teamowner":   "myteam",
		"plan": map[string]interface{}{
			"name":     "myplan",
			"memory":   float64(64),
			"cpumilli": float64(0),
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
			map[string]interface{}{"process": "p1", "minUnits": float64(0), "maxUnits": float64(0), "version": float64(0), "behavior": map[string]interface{}{}},
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
	appInfo, err := AppInfo(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	data, err := json.Marshal(appInfo)
	c.Assert(err, check.IsNil)
	result := make(map[string]interface{})
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
}

func (s *S) TestAppMarshalJSONUnitsError(c *check.C) {
	provisiontest.ProvisionerInstance.PrepareFailure("Units", fmt.Errorf("my err"))
	app := appTypes.App{
		Name:    "name",
		Routers: []appTypes.AppRouter{{Name: "fake", Opts: map[string]string{}}},
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
		"plan": map[string]interface{}{
			"name":     "",
			"memory":   float64(0),
			"cpumilli": float64(0),
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
				"Domain":     "name-web.fake-cluster.local",
				"Port":       float64(80),
				"TargetPort": float64(8080),
				"Process":    "web",
				"Protocol":   "TCP",
				"Version":    "",
			},
			map[string]interface{}{
				"Domain":     "name-logs.fake-cluster.local",
				"Port":       float64(12201),
				"TargetPort": float64(12201),
				"Process":    "logs",
				"Protocol":   "UDP",
				"Version":    "",
			},
			map[string]interface{}{
				"Domain":     "name-logs-v2.fake-cluster.local",
				"Port":       float64(12201),
				"TargetPort": float64(12201),
				"Process":    "logs",
				"Protocol":   "UDP",
				"Version":    "2",
			},
			map[string]interface{}{
				"Domain":     "name-web-v2.fake-cluster.local",
				"Port":       float64(80),
				"TargetPort": float64(8080),
				"Process":    "web",
				"Protocol":   "TCP",
				"Version":    "2",
			},
		},
		"serviceInstanceBinds": []interface{}{},
	}
	appInfo, err := AppInfo(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	data, err := json.Marshal(appInfo)
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
	app := appTypes.App{
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
		"plan": map[string]interface{}{
			"name":     "myplan",
			"memory":   float64(64),
			"cpumilli": float64(0),
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
				"Domain":     "name-web.fake-cluster.local",
				"Port":       float64(80),
				"TargetPort": float64(8080),
				"Process":    "web",
				"Protocol":   "TCP",
				"Version":    "",
			},
			map[string]interface{}{
				"Domain":     "name-logs.fake-cluster.local",
				"Port":       float64(12201),
				"TargetPort": float64(12201),
				"Process":    "logs",
				"Protocol":   "UDP",
				"Version":    "",
			},
			map[string]interface{}{
				"Domain":     "name-logs-v2.fake-cluster.local",
				"Port":       float64(12201),
				"TargetPort": float64(12201),
				"Process":    "logs",
				"Protocol":   "UDP",
				"Version":    "2",
			},
			map[string]interface{}{
				"Domain":     "name-web-v2.fake-cluster.local",
				"Port":       float64(80),
				"TargetPort": float64(8080),
				"Process":    "web",
				"Protocol":   "TCP",
				"Version":    "2",
			},
		},
	}
	appInfo, err := AppInfo(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	data, err := json.Marshal(appInfo)
	c.Assert(err, check.IsNil)
	result := make(map[string]interface{})
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
}

func (s *S) TestAppMarshalJSONWithCustomQuota(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "my-pool", Default: false})
	c.Assert(err, check.IsNil)
	app := appTypes.App{
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
	err = routertest.FakeRouter.EnsureBackend(context.TODO(), &app, router.EnsureBackendOpts{})
	c.Assert(err, check.IsNil)
	s.mockService.AppQuota.OnGet = func(_ *appTypes.App) (*quota.Quota, error) {
		return &quota.Quota{InUse: 100, Limit: 777}, nil
	}
	appInfo, err := AppInfo(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	data, err := json.Marshal(appInfo)
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
		"plan": map[string]interface{}{
			"name":     "small",
			"cpumilli": float64(1000),
			"memory":   float64(128),
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
				"Domain":     "my-awesome-app-web.fake-cluster.local",
				"Port":       float64(80),
				"TargetPort": float64(8080),
				"Process":    "web",
				"Protocol":   "TCP",
				"Version":    "",
			},
			map[string]interface{}{
				"Domain":     "my-awesome-app-logs.fake-cluster.local",
				"Port":       float64(12201),
				"TargetPort": float64(12201),
				"Process":    "logs",
				"Protocol":   "UDP",
				"Version":    "",
			},
			map[string]interface{}{
				"Domain":     "my-awesome-app-logs-v2.fake-cluster.local",
				"Port":       float64(12201),
				"TargetPort": float64(12201),
				"Process":    "logs",
				"Protocol":   "UDP",
				"Version":    "2",
			},
			map[string]interface{}{
				"Domain":     "my-awesome-app-web-v2.fake-cluster.local",
				"Port":       float64(80),
				"TargetPort": float64(8080),
				"Process":    "web",
				"Protocol":   "TCP",
				"Version":    "2",
			},
		},
	})
}

func (s *S) TestAppMarshalJSONServiceInstanceBinds(c *check.C) {
	err := pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "my-pool", Default: false})
	c.Assert(err, check.IsNil)
	app := appTypes.App{
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
	err = service.Create(context.TODO(), service1)
	c.Assert(err, check.IsNil)
	instance1 := service.ServiceInstance{
		ServiceName: service1.Name,
		Name:        service1.Name + "-1",
		Teams:       []string{"team-one"},
		Apps:        []string{app.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance1)
	c.Assert(err, check.IsNil)
	instance2 := service.ServiceInstance{
		ServiceName: service1.Name,
		Name:        service1.Name + "-2",
		Teams:       []string{"team-one"},
		Apps:        []string{app.Name},
		PlanName:    "some-example",
	}
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance2)
	c.Assert(err, check.IsNil)
	service2 := service.Service{
		Name:       "service-2",
		Teams:      []string{"team-one"},
		OwnerTeams: []string{"team-one"},
		Endpoint:   map[string]string{"production": "http://localhost:1234"},
		Password:   "abcde",
	}
	err = service.Create(context.TODO(), service2)
	c.Assert(err, check.IsNil)
	instance3 := service.ServiceInstance{
		ServiceName: service2.Name,
		Name:        service2.Name + "-1",
		Teams:       []string{"team-one"},
		Apps:        []string{app.Name},
		PlanName:    "another-plan",
	}
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance3)
	c.Assert(err, check.IsNil)
	appInfo, err := AppInfo(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	data, err := json.Marshal(appInfo)
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
		"plan": map[string]interface{}{
			"name":     "small",
			"cpumilli": float64(1000),
			"memory":   float64(128),
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
				"Domain":     "my-awesome-app-web.fake-cluster.local",
				"Port":       float64(80),
				"TargetPort": float64(8080),
				"Process":    "web",
				"Protocol":   "TCP",
				"Version":    "",
			},
			map[string]interface{}{
				"Domain":     "my-awesome-app-logs.fake-cluster.local",
				"Port":       float64(12201),
				"TargetPort": float64(12201),
				"Process":    "logs",
				"Protocol":   "UDP",
				"Version":    "",
			},
			map[string]interface{}{
				"Domain":     "my-awesome-app-logs-v2.fake-cluster.local",
				"Port":       float64(12201),
				"TargetPort": float64(12201),
				"Process":    "logs",
				"Protocol":   "UDP",
				"Version":    "2",
			},
			map[string]interface{}{
				"Domain":     "my-awesome-app-web-v2.fake-cluster.local",
				"Port":       float64(80),
				"TargetPort": float64(8080),
				"Process":    "web",
				"Protocol":   "TCP",
				"Version":    "2",
			},
		},
	})
}

func (s *S) TestRun(c *check.C) {
	s.provisioner.PrepareOutput([]byte("a lot of files"))
	s.provisioner.PrepareOutput([]byte("a lot of files"))
	app := appTypes.App{Name: "myapp", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &app, 2, "web", newSuccessfulAppVersion(c, &app), nil)
	var buf bytes.Buffer
	args := provision.RunArgs{Once: false, Isolated: false}
	err = Run(context.TODO(), &app, "ls -lh", &buf, args)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "a lot of filesa lot of files")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls -lh"
	units, err := AppUnits(context.TODO(), &app)
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
		logs, err = LastLogs(context.TODO(), &app, servicemanager.LogService, appTypes.ListLogArgs{
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
	app := appTypes.App{
		Name:      "myapp",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &app, 2, "web", newSuccessfulAppVersion(c, &app), nil)
	var buf bytes.Buffer
	args := provision.RunArgs{Once: true, Isolated: false}
	err = Run(context.TODO(), &app, "ls -lh", &buf, args)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "a lot of files")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls -lh"
	units, err := AppUnits(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 2)
	allExecs := s.provisioner.AllExecs()
	c.Assert(allExecs, check.HasLen, 1)
	c.Assert(allExecs[units[0].GetID()], check.HasLen, 1)
	c.Assert(allExecs[units[0].GetID()][0].Cmds, check.DeepEquals, []string{"/bin/sh", "-c", expected})
}

func (s *S) TestRunIsolated(c *check.C) {
	s.provisioner.PrepareOutput([]byte("a lot of files"))
	app := appTypes.App{
		Name:      "myapp",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &app, 1, "web", newSuccessfulAppVersion(c, &app), nil)
	var buf bytes.Buffer
	args := provision.RunArgs{Once: false, Isolated: true}
	err = Run(context.TODO(), &app, "ls -lh", &buf, args)
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
	app := appTypes.App{
		Name:      "myapp",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	args := provision.RunArgs{Once: false, Isolated: false}
	err = Run(context.TODO(), &app, "ls -lh", &buf, args)
	c.Assert(err, check.ErrorMatches, `App must be available to run non-isolated commands`)
}

func (s *S) TestRunWithoutUnitsIsolated(c *check.C) {
	s.provisioner.PrepareOutput([]byte("a lot of files"))
	app := appTypes.App{
		Name:      "myapp",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	args := provision.RunArgs{Once: false, Isolated: true}
	err = Run(context.TODO(), &app, "ls -lh", &buf, args)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "a lot of files")
}

func (s *S) TestEnvs(c *check.C) {
	app := appTypes.App{
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
			Name:      "TSURU_SERVICES",
			Value:     "{}",
			ManagedBy: "tsuru",
		},
		"TSURU_APPNAME": {
			Name:      "TSURU_APPNAME",
			Value:     "time",
			ManagedBy: "tsuru",
			Public:    true,
		},
		"TSURU_APPDIR": {
			Name:      "TSURU_APPDIR",
			Value:     "/home/application/current",
			ManagedBy: "tsuru",
			Public:    true,
		},
	}
	env := provision.EnvsForApp(&app)
	c.Assert(env, check.DeepEquals, expected)
}

func (s *S) TestEnvsInterpolate(c *check.C) {
	app := appTypes.App{
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
		"DB_HOST":        {Name: "DB_HOST", Value: "host1", ManagedBy: "srv1/inst1"},
		"TSURU_SERVICES": {Name: "TSURU_SERVICES", Value: "{\"srv1\":[{\"instance_name\":\"inst1\",\"envs\":{\"DB_HOST\":\"host1\"}}]}", ManagedBy: "tsuru"},
		"TSURU_APPNAME":  {Name: "TSURU_APPNAME", Value: "time", ManagedBy: "tsuru", Public: true},
		"TSURU_APPDIR":   {Name: "TSURU_APPDIR", Value: "/home/application/current", ManagedBy: "tsuru", Public: true},
	}
	env := provision.EnvsForApp(&app)
	c.Assert(env, check.DeepEquals, expected)
}

func (s *S) TestEnvsWithServiceEnvConflict(c *check.C) {
	app := appTypes.App{
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
			Name:      "DB_HOST",
			Value:     "host2",
			ManagedBy: "srv1/inst2",
		},
	}
	env := provision.EnvsForApp(&app)
	serviceEnvsRaw := env[tsuruEnvs.TsuruServicesEnvVar]
	delete(env, tsuruEnvs.TsuruServicesEnvVar)
	delete(env, "TSURU_APPNAME")
	delete(env, "TSURU_APPDIR")
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
	a := appTypes.App{
		Name:      "testapp",
		TeamOwner: s.team.Name,
	}
	a2 := appTypes.App{
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
	a := appTypes.App{
		Name:      "app1",
		TeamOwner: s.team.Name,
	}
	a2 := appTypes.App{
		Name:      "app2",
		TeamOwner: s.team.Name,
	}
	a3 := appTypes.App{
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
	a := appTypes.App{
		Name:      "testapp",
		Platform:  "ruby",
		TeamOwner: s.team.Name,
	}
	a2 := appTypes.App{
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
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	apps := []any{
		appTypes.App{Name: "testapp", Platform: "ruby", TeamOwner: s.team.Name},
		appTypes.App{Name: "testapplatest", Platform: "ruby", TeamOwner: s.team.Name, PlatformVersion: "latest"},
		appTypes.App{Name: "othertestapp", Platform: "ruby", PlatformVersion: "v1", TeamOwner: s.team.Name},
		appTypes.App{Name: "testappwithoutversion", Platform: "ruby", TeamOwner: s.team.Name},
		appTypes.App{Name: "testappwithoutversionfield", Platform: "ruby", TeamOwner: s.team.Name},
	}

	_, err = collection.InsertMany(context.TODO(), apps)
	c.Assert(err, check.IsNil)

	_, err = collection.UpdateOne(context.TODO(), mongoBSON.M{"name": "testappwithoutversionfield"}, mongoBSON.M{"$unset": mongoBSON.M{"platformversion": ""}})
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
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := appTypes.App{
		Name:      "testapp",
		Teams:     []string{s.team.Name},
		TeamOwner: "foo",
	}
	a2 := appTypes.App{
		Name:      "othertestapp",
		Teams:     []string{"commonteam", s.team.Name},
		TeamOwner: "bar",
	}

	_, err = collection.InsertMany(context.TODO(), []any{a, a2})
	c.Assert(err, check.IsNil)

	apps, err := List(context.TODO(), &Filter{TeamOwner: "foo"})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
}

func (s *S) TestListReturnsAppsForAGivenUserFilteringByOwner(c *check.C) {
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := appTypes.App{
		Name:  "testapp",
		Teams: []string{s.team.Name},
		Owner: "foo",
	}
	a2 := appTypes.App{
		Name:  "othertestapp",
		Teams: []string{"commonteam", s.team.Name},
		Owner: "bar",
	}

	_, err = collection.InsertMany(context.TODO(), []any{a, a2})
	c.Assert(err, check.IsNil)

	apps, err := List(context.TODO(), &Filter{UserOwner: "foo"})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
}

func (s *S) TestListAll(c *check.C) {
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := appTypes.App{
		Name:  "testapp",
		Teams: []string{s.team.Name},
	}
	a2 := appTypes.App{
		Name:  "othertestapp",
		Teams: []string{"commonteam", s.team.Name},
	}

	_, err = collection.InsertMany(context.TODO(), []any{a, a2})
	c.Assert(err, check.IsNil)

	apps, err := List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 2)
}

func (s *S) TestListUsesCachedRouterAddrs(c *check.C) {
	a := appTypes.App{
		Name:      "app1",
		TeamOwner: s.team.Name,
	}
	a2 := appTypes.App{
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
	c.Assert(apps, tsuruTest.JSONEquals, []appTypes.App{
		{
			Name:        "app1",
			CName:       []string{},
			CertIssuers: map[string]string{},
			Teams:       []string{"tsuruteam"},
			TeamOwner:   "tsuruteam",
			Owner:       "whydidifall@thewho.com",
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
			Processes: []appTypes.Process{},
			Quota:     quota.UnlimitedQuota,
		},
		{
			Name:        "app2",
			CName:       []string{},
			CertIssuers: map[string]string{},
			Teams:       []string{"tsuruteam"},
			TeamOwner:   "tsuruteam",
			Owner:       "whydidifall@thewho.com",
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
			Processes: []appTypes.Process{},
			Quota:     quota.UnlimitedQuota,
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
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := appTypes.App{
		Name:      "app1",
		TeamOwner: s.team.Name,
		Teams:     []string{s.team.Name},
		Router:    "fake",
	}
	_, err = collection.InsertOne(context.TODO(), a)
	c.Assert(err, check.IsNil)
	err = routertest.FakeRouter.EnsureBackend(context.TODO(), &a, router.EnsureBackendOpts{})
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps, tsuruTest.JSONEquals, []appTypes.App{
		{
			Name:        "app1",
			CName:       []string{},
			CertIssuers: map[string]string{},
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
			Processes: []appTypes.Process{},
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
	a := appTypes.App{
		Name:      "app1",
		TeamOwner: s.team.Name,
	}
	a2 := appTypes.App{
		Name:      "app2",
		TeamOwner: s.team.Name,
	}
	a3 := appTypes.App{
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
	a := appTypes.App{
		Name:      "app1",
		TeamOwner: s.team.Name,
	}
	a2 := appTypes.App{
		Name:      "app1-dev",
		TeamOwner: s.team.Name,
	}
	a3 := appTypes.App{
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
	a := appTypes.App{
		Name:      "testapp",
		Platform:  "ruby",
		TeamOwner: s.team.Name,
	}
	a2 := appTypes.App{
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
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := appTypes.App{
		Name:  "testapp",
		Owner: "foo",
	}
	a2 := appTypes.App{
		Name:  "othertestapp",
		Owner: "bar",
	}
	_, err = collection.InsertMany(context.TODO(), []any{a, a2})
	c.Assert(err, check.IsNil)

	apps, err := List(context.TODO(), &Filter{UserOwner: "foo"})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
}

func (s *S) TestListFilteringByTeamOwner(c *check.C) {
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := appTypes.App{
		Name:      "testapp",
		Teams:     []string{s.team.Name},
		TeamOwner: "foo",
	}
	a2 := appTypes.App{
		Name:      "othertestapp",
		Teams:     []string{s.team.Name},
		TeamOwner: "bar",
	}

	_, err = collection.InsertMany(context.TODO(), []any{a, a2})
	c.Assert(err, check.IsNil)

	apps, err := List(context.TODO(), &Filter{TeamOwner: "foo"})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
}

func (s *S) TestListFilteringByPool(c *check.C) {
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	opts := pool.AddPoolOptions{Name: "test2", Default: false}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	a := appTypes.App{
		Name:  "testapp",
		Owner: "foo",
		Pool:  opts.Name,
	}
	a2 := appTypes.App{
		Name:  "othertestapp",
		Owner: "bar",
		Pool:  s.Pool,
	}
	_, err = collection.InsertMany(context.TODO(), []any{a, a2})
	c.Assert(err, check.IsNil)

	apps, err := List(context.TODO(), &Filter{Pool: s.Pool})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].Name, check.Equals, a2.Name)
	c.Assert(apps[0].Pool, check.Equals, a2.Pool)
}

func (s *S) TestListFilteringByPools(c *check.C) {
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	opts := pool.AddPoolOptions{Name: "test2", Default: false}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "test3", Default: false}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	a := appTypes.App{
		Name:  "testapp",
		Owner: "foo",
		Pool:  s.Pool,
	}
	a2 := appTypes.App{
		Name:  "testapp2",
		Owner: "bar",
		Pool:  "test2",
	}
	a3 := appTypes.App{
		Name:  "testapp3",
		Owner: "bar",
		Pool:  "test3",
	}

	_, err = collection.InsertMany(context.TODO(), []any{a, a2, a3})
	c.Assert(err, check.IsNil)

	apps, err := List(context.TODO(), &Filter{Pools: []string{s.Pool, "test2"}})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 2)
	appNames := []string{apps[0].Name, apps[1].Name}
	sort.Strings(appNames)
	c.Assert(appNames, check.DeepEquals, []string{"testapp", "testapp2"})
}

func (s *S) TestListFilteringByStatuses(c *check.C) {
	var apps []*appTypes.App
	appNames := []string{"ta1", "ta2", "ta3"}
	for _, name := range appNames {
		a := appTypes.App{
			Name:      name,
			Teams:     []string{s.team.Name},
			Quota:     quota.Quota{Limit: 10},
			TeamOwner: s.team.Name,
			Routers:   []appTypes.AppRouter{{Name: "fake"}},
		}
		err := CreateApp(context.TODO(), &a, s.user)
		c.Assert(err, check.IsNil)
		newSuccessfulAppVersion(c, &a)
		err = AddUnits(context.TODO(), &a, 1, "", "", nil)
		c.Assert(err, check.IsNil)
		apps = append(apps, &a)
	}
	var buf bytes.Buffer
	err := Stop(context.TODO(), apps[1], &buf, "", "")
	c.Assert(err, check.IsNil)
	resultApps, err := List(context.TODO(), &Filter{Statuses: []string{"stopped"}})
	c.Assert(err, check.IsNil)
	c.Assert(resultApps, check.HasLen, 1)
	c.Assert([]string{resultApps[0].Name}, check.DeepEquals, []string{"ta2"})
}

func (s *S) TestListFilteringByTag(c *check.C) {
	app1 := appTypes.App{Name: "app1", TeamOwner: s.team.Name, Tags: []string{"tag 1"}}
	err := CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := appTypes.App{Name: "app2", TeamOwner: s.team.Name, Tags: []string{"tag 1", "tag 2", "tag 3"}}
	err = CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	app3 := appTypes.App{Name: "app3", TeamOwner: s.team.Name, Tags: []string{"tag 4"}}
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
	c.Assert(apps, check.DeepEquals, []*appTypes.App{})
}

func (s *S) TestListReturnsAllAppsWhenUsedWithNoFilters(c *check.C) {
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := appTypes.App{Name: "testApp", Teams: []string{"notAdmin", "noSuperUser"}}
	_, err = collection.InsertOne(context.TODO(), a)
	c.Assert(err, check.IsNil)
	apps, err := List(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(len(apps), Greater, 0)
	c.Assert(apps[0].Name, check.Equals, "testApp")
	c.Assert(apps[0].Teams, check.DeepEquals, []string{"notAdmin", "noSuperUser"})
}

func (s *S) TestListFilteringExtraWithOr(c *check.C) {
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	opts := pool.AddPoolOptions{Name: "test2", Default: false}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	a := appTypes.App{
		Name:  "testapp1",
		Owner: "foo",
		Pool:  opts.Name,
	}
	a2 := appTypes.App{
		Name:  "testapp2",
		Teams: []string{s.team.Name},
		Owner: "bar",
		Pool:  s.Pool,
	}
	a3 := appTypes.App{
		Name:  "testapp3",
		Teams: []string{"otherteam"},
		Owner: "bar",
		Pool:  opts.Name,
	}

	_, err = collection.InsertMany(context.TODO(), []any{a, a2, a3})
	c.Assert(err, check.IsNil)

	f := &Filter{}
	f.ExtraIn("pool", s.Pool)
	f.ExtraIn("teams", "otherteam")
	apps, err := List(context.TODO(), f)
	c.Assert(err, check.IsNil)
	var appNames []string
	for _, a := range apps {
		appNames = append(appNames, a.Name)
	}
	sort.Strings(appNames)
	c.Assert(appNames, check.DeepEquals, []string{a2.Name, a3.Name})
}

func (s *S) TestGetName(c *check.C) {
	a := appTypes.App{Name: "something"}
	c.Assert(a.Name, check.Equals, a.Name)
}

func (s *S) TestGetQuota(c *check.C) {
	s.mockService.AppQuota.OnGet = func(item *appTypes.App) (*quota.Quota, error) {
		c.Assert(item.Name, check.Equals, "app1")
		return &quota.Quota{InUse: 1, Limit: 2}, nil
	}
	a := appTypes.App{Name: "app1"}
	q, err := GetQuota(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	c.Assert(q, check.DeepEquals, &quota.Quota{InUse: 1, Limit: 2})
}

func (s *S) TestGetMetadata(c *check.C) {
	a := appTypes.App{}
	c.Assert(provision.GetAppMetadata(&a, ""), check.DeepEquals, appTypes.Metadata{})

	a = appTypes.App{
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
	c.Assert(provision.GetAppMetadata(&a, ""), check.DeepEquals, appTypes.Metadata{
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

	a = appTypes.App{
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

	c.Assert(provision.GetAppMetadata(&a, "web"), check.DeepEquals, appTypes.Metadata{
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
	a := appTypes.App{Name: "anycolor", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 1, "web", newSuccessfulAppVersion(c, &a), nil)
	units, err := AppUnits(context.TODO(), &a)
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

	a := appTypes.App{Name: "anycolor", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	err = provisioner.SetAutoScale(context.TODO(), &a, provTypes.AutoScaleSpec{
		Process:    "web",
		Version:    1,
		MinUnits:   1,
		MaxUnits:   5,
		AverageCPU: "70",
	})
	c.Assert(err, check.IsNil)

	err = AddUnits(context.TODO(), &a, 1, "web", "1", io.Discard)
	c.Assert(err.Error(), check.Equals, "cannot add units to an app with autoscaler configured, please update autoscale settings")
}

func (s *S) TestAppAvailable(c *check.C) {
	a := appTypes.App{
		Name:      "anycolor",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	version := newSuccessfulAppVersion(c, &a)
	s.provisioner.AddUnits(context.TODO(), &a, 1, "web", version, nil)
	c.Assert(available(context.TODO(), &a), check.Equals, true)
	s.provisioner.Stop(context.TODO(), &a, "", version, nil)
	c.Assert(available(context.TODO(), &a), check.Equals, false)
}

func (s *S) TestStart(c *check.C) {
	s.provisioner.PrepareOutput([]byte("not yaml")) // loadConf
	a := appTypes.App{
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
	err = Start(context.TODO(), &a, &b, "", "")
	c.Assert(err, check.IsNil)
	starts := s.provisioner.Starts(&a, "")
	c.Assert(starts, check.Equals, 1)
}

func (s *S) TestAppSetUpdatePlatform(c *check.C) {
	a := appTypes.App{
		Name:      "someapp",
		Platform:  "django",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	SetUpdatePlatform(context.TODO(), &a, true)
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
	a := appTypes.App{Name: "test", Platform: "python", TeamOwner: team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
}

func (s *S) TestAppValidateProcesses(c *check.C) {
	a := appTypes.App{
		Name: "test",
		Processes: []appTypes.Process{
			{Name: "web"},
		},
	}
	err := validateProcesses(&a)
	c.Assert(err, check.IsNil)

	a = appTypes.App{
		Name: "test",
		Processes: []appTypes.Process{
			{Name: "web"},
			{Name: "web"},
		},
	}
	err = validateProcesses(&a)
	c.Assert(err.Error(), check.Equals, "process \"web\" is duplicated")

	a = appTypes.App{
		Name: "test",
		Processes: []appTypes.Process{
			{},
		},
	}
	err = validateProcesses(&a)
	c.Assert(err.Error(), check.Equals, "empty process name is not allowed")
}

func (s *S) TestAppUpdateProcessesWhenAppend(c *check.C) {
	a := appTypes.App{
		Name: "test",
		Processes: []appTypes.Process{
			{Name: "web"},
		},
	}
	updateProcesses(context.TODO(), &a, []appTypes.Process{
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
	a := appTypes.App{
		Name:      "test",
		Processes: []appTypes.Process{},
	}
	updateProcesses(context.TODO(), &a, []appTypes.Process{
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
	a := appTypes.App{
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
	updateProcesses(context.TODO(), &a, []appTypes.Process{
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
	a := appTypes.App{
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
	updateProcesses(context.TODO(), &a, []appTypes.Process{
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

	a := appTypes.App{
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
	changed, err := updateProcesses(context.TODO(), &a, []appTypes.Process{
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
	app := appTypes.App{Name: "fyrone-flats", Platform: "python", TeamOwner: s.team.Name, Pool: s.Pool}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	serv := service.Service{
		Name:       "healthcheck",
		Password:   "nidalee",
		Endpoint:   map[string]string{"production": "somehost.com"},
		OwnerTeams: []string{s.team.Name},
	}
	err = service.Create(context.TODO(), serv)
	c.Assert(err, check.IsNil)
	err = ValidateService(context.TODO(), &app, serv.Name)
	c.Assert(err, check.IsNil)
	err = ValidateService(context.TODO(), &app, "invalidService")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "service \"invalidService\" is not available for pool \"pool1\". Available services are: \"my, mysql, healthcheck\"")
}

func (s *S) TestValidateBlacklistedAppService(c *check.C) {
	app := appTypes.App{Name: "urgot", Platform: "python", TeamOwner: s.team.Name, Pool: s.Pool}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	serv := service.Service{
		Name:       "healthcheck",
		Password:   "nidalee",
		Endpoint:   map[string]string{"production": "somehost.com"},
		OwnerTeams: []string{s.team.Name},
	}
	err = service.Create(context.TODO(), serv)
	c.Assert(err, check.IsNil)
	err = ValidateService(context.TODO(), &app, serv.Name)
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
	err = pool.SetPoolConstraint(context.TODO(), &poolConstraint)
	c.Assert(err, check.IsNil)
	err = ValidateService(context.TODO(), &app, serv.Name)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "service \"healthcheck\" is not available for pool \"pool1\".")
	opts := pool.AddPoolOptions{Name: "poolz"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	app2 := appTypes.App{Name: "nidalee", Platform: "python", TeamOwner: s.team.Name, Pool: "poolz"}
	err = CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	err = ValidateService(context.TODO(), &app2, serv.Name)
	c.Assert(err, check.IsNil)
}

func (s *S) TestAppCreateValidateTeamOwnerSetAnTeamWhichNotExists(c *check.C) {
	a := appTypes.App{Name: "test", Platform: "python", TeamOwner: "not-exists"}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.DeepEquals, &errors.ValidationError{Message: authTypes.ErrTeamNotFound.Error()})
}

func (s *S) TestAppCreateValidateRouterNotAvailableForPool(c *check.C) {
	pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{
		PoolExpr:  "pool1",
		Field:     pool.ConstraintTypeRouter,
		Values:    []string{"fake-tls"},
		Blacklist: true,
	})
	a := appTypes.App{Name: "test", Platform: "python", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.DeepEquals, &errors.ValidationError{
		Message: "router \"fake-tls\" is not available for pool \"pool1\". Available routers are: \"fake\"",
	})
}

func (s *S) TestAppSetPoolByTeamOwner(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test"}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), "test", []string{"tsuruteam"})
	c.Assert(err, check.IsNil)
	app := appTypes.App{
		Name:      "test",
		TeamOwner: "tsuruteam",
	}
	err = SetPool(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	c.Assert(app.Pool, check.Equals, "test")
}

func (s *S) TestAppSetPoolDefault(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test", Public: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	app := appTypes.App{
		Name:      "test",
		TeamOwner: "tsuruteam",
	}
	err = SetPool(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	c.Assert(app.Pool, check.Equals, "pool1")
}

func (s *S) TestAppSetPoolByPool(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test"}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), "test", []string{"tsuruteam"})
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "pool2"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), "pool2", []string{"tsuruteam"})
	c.Assert(err, check.IsNil)
	app := appTypes.App{
		Name:      "test",
		Pool:      "pool2",
		TeamOwner: "tsuruteam",
	}
	err = SetPool(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	c.Assert(app.Pool, check.Equals, "pool2")
}

func (s *S) TestAppSetPoolManyPools(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test"}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), "test", []string{"test"})
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "pool2"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), "pool2", []string{"test"})
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "pool3", Public: true}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	app := appTypes.App{
		Name:      "test",
		TeamOwner: "test",
	}
	err = SetPool(context.TODO(), &app)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "you have access to \"test\",\"pool2\",\"pool1\",\"pool3\" pools. Please choose one in app creation")
}

func (s *S) TestAppSetPoolNoDefault(c *check.C) {
	err := pool.RemovePool(context.TODO(), "pool1")
	c.Assert(err, check.IsNil)
	app := appTypes.App{
		Name: "test",
	}
	err = SetPool(context.TODO(), &app)
	c.Assert(err, check.NotNil)
	c.Assert(app.Pool, check.Equals, "")
}

func (s *S) TestAppSetPoolUserDontHaveAccessToPool(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test"}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), "test", []string{"nopool"})
	c.Assert(err, check.IsNil)
	app := appTypes.App{
		Name:      "test",
		TeamOwner: "tsuruteam",
		Pool:      "test",
	}
	err = SetPool(context.TODO(), &app)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, `App team owner ".*" has no access to pool ".*"`)
}

func (s *S) TestAppSetPoolToPublicPool(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test", Public: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	app := appTypes.App{
		Name:      "testapp",
		TeamOwner: "tsuruteam",
		Pool:      "test",
	}
	err = SetPool(context.TODO(), &app)
	c.Assert(err, check.IsNil)
}

func (s *S) TestAppSetPoolPriorityTeamOwnerOverPublicPools(c *check.C) {
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	opts := pool.AddPoolOptions{Name: "test", Public: true}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "nonpublic"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), "nonpublic", []string{"tsuruteam"})
	c.Assert(err, check.IsNil)
	a := appTypes.App{
		Name:      "testapp",
		TeamOwner: "tsuruteam",
	}
	err = SetPool(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	_, err = collection.InsertOne(context.TODO(), a)
	c.Assert(err, check.IsNil)
	app, _ := GetByName(context.TODO(), a.Name)
	c.Assert("nonpublic", check.Equals, app.Pool)
}

func (s *S) TestShellToUnit(c *check.C) {
	a := appTypes.App{Name: "my-test-app", TeamOwner: s.team.Name}
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
	err = Shell(context.TODO(), &a, opts)
	c.Assert(err, check.IsNil)
	allExecs := s.provisioner.AllExecs()
	c.Assert(allExecs, check.HasLen, 1)
	c.Assert(allExecs[unit.GetID()], check.HasLen, 1)
	c.Assert(allExecs[unit.GetID()][0].Cmds, check.DeepEquals, []string{"/bin/sh", "-c", "[ -f /home/application/apprc ] && source /home/application/apprc; [ -d /home/application/current ] && cd /home/application/current; [ $(command -v bash) ] && exec bash -l || exec sh -l"})
}

func (s *S) TestShellNoUnits(c *check.C) {
	a := appTypes.App{Name: "my-test-app", TeamOwner: s.team.Name}
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
	err = Shell(context.TODO(), &a, opts)
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
	a := appTypes.App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}, CName: []string{cname}}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = SetCertificate(context.TODO(), &a, cname, string(cert), string(key))
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
	a := appTypes.App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}, CName: []string{cname}}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = SetCertificate(context.TODO(), &a, cname, string(cert), string(key))
	c.Assert(err, check.IsNil)
	c.Assert(routertest.TLSRouter.Certs[cname], check.Equals, string(cert))
	c.Assert(routertest.TLSRouter.Keys[cname], check.Equals, string(key))
}

func (s *S) TestSetCertificateNonTLSRouter(c *check.C) {
	cert, err := os.ReadFile("testdata/certificate.crt")
	c.Assert(err, check.IsNil)
	key, err := os.ReadFile("testdata/private.key")
	c.Assert(err, check.IsNil)
	a := appTypes.App{Name: "my-test-app", TeamOwner: s.team.Name, CName: []string{"app.io"}}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = SetCertificate(context.TODO(), &a, "app.io", string(cert), string(key))
	c.Assert(err, check.ErrorMatches, "no router with tls support")
}

func (s *S) TestSetCertificateInvalidCName(c *check.C) {
	a := appTypes.App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = SetCertificate(context.TODO(), &a, "example.com", "cert", "key")
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
	a := appTypes.App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}, CName: []string{cname}}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = SetCertificate(context.TODO(), &a, cname, string(cert), string(key))
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
	a := appTypes.App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}, CName: []string{cname}}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = SetCertificate(context.TODO(), &a, cname, string(cert), string(key))
	c.Assert(err, check.IsNil)
	err = RemoveCertificate(context.TODO(), &a, cname)
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
	a := appTypes.App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}, CName: []string{cname}}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = SetCertificate(context.TODO(), &a, cname, string(cert), string(key))
	c.Assert(err, check.IsNil)
	err = RemoveCertificate(context.TODO(), &a, cname)
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

	a := appTypes.App{
		Name:        "my-test-app",
		TeamOwner:   s.team.Name,
		Routers:     []appTypes.AppRouter{{Name: "fake-tls"}},
		CName:       []string{cname},
		CertIssuers: map[string]string{"app.io": "letsencrypt"},
	}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	newSuccessfulAppVersion(c, &a)
	err = AddUnits(context.TODO(), &a, 1, "", "", nil)
	c.Assert(err, check.IsNil)

	err = SetCertificate(context.TODO(), &a, cname, string(cert), string(key))
	c.Assert(err, check.IsNil)
	expectedCerts := &appTypes.CertificateSetInfo{
		Routers: map[string]appTypes.RouterCertificateInfo{
			"fake-tls": {
				CNames: map[string]appTypes.CertificateInfo{
					"app.io": {
						Certificate: string(cert),
						Issuer:      "letsencrypt",
					},

					"my-test-app.faketlsrouter.com": {
						Certificate: string("<mock cert>"),
					},
				},
			},
		},
	}
	certs, err := GetCertificates(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	c.Assert(certs, check.DeepEquals, expectedCerts)
}

func (s *S) TestGetCertificatesWithCertNotReady(c *check.C) {
	cname1 := "certissuer-not-ready.io"
	cname2 := "app.io" // cert without certissuer

	a := appTypes.App{
		Name:        "my-test-app",
		TeamOwner:   s.team.Name,
		Routers:     []appTypes.AppRouter{{Name: "fake-tls"}},
		CName:       []string{cname1, cname2},
		CertIssuers: map[string]string{cname1: "letsencrypt"},
	}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)

	newSuccessfulAppVersion(c, &a)
	err = AddUnits(context.TODO(), &a, 1, "", "", nil)
	c.Assert(err, check.IsNil)

	expectedCerts := &appTypes.CertificateSetInfo{
		Routers: map[string]appTypes.RouterCertificateInfo{
			"fake-tls": {
				CNames: map[string]appTypes.CertificateInfo{
					cname1: {
						Certificate: "",
						Issuer:      "letsencrypt",
					},

					"my-test-app.faketlsrouter.com": {
						Certificate: string("<mock cert>"),
					},
				},
			},
		},
	}
	certs, err := GetCertificates(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	c.Assert(certs, check.DeepEquals, expectedCerts)

	cert, err := os.ReadFile("testdata/certificate.crt")
	c.Assert(err, check.IsNil)
	key, err := os.ReadFile("testdata/private.key")
	c.Assert(err, check.IsNil)

	err = SetCertificate(context.TODO(), &a, cname2, string(cert), string(key))
	c.Assert(err, check.IsNil)

	expectedCerts = &appTypes.CertificateSetInfo{
		Routers: map[string]appTypes.RouterCertificateInfo{
			"fake-tls": {
				CNames: map[string]appTypes.CertificateInfo{
					cname1: {
						Certificate: "",
						Issuer:      "letsencrypt",
					},

					cname2: {
						Certificate: string(cert),
					},

					"my-test-app.faketlsrouter.com": {
						Certificate: string("<mock cert>"),
					},
				},
			},
		},
	}
	certs, err = GetCertificates(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	c.Assert(certs, check.DeepEquals, expectedCerts)
}

func (s *S) TestGetCertificatesNonTLSRouter(c *check.C) {
	a := appTypes.App{Name: "my-test-app", TeamOwner: s.team.Name, CName: []string{"app.io"}}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	certs, err := GetCertificates(context.TODO(), &a)
	c.Assert(err, check.ErrorMatches, "no router with tls support")
	c.Assert(certs, check.IsNil)
}

func (s *S) TestUpdateAppWithInvalidName(c *check.C) {
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	app := appTypes.App{Name: "app with invalid name", Plan: s.defaultPlan, Platform: "python", TeamOwner: s.team.Name, Pool: s.Pool}
	_, err = collection.InsertOne(context.TODO(), app)
	c.Assert(err, check.IsNil)

	updateData := appTypes.App{Name: app.Name, Description: "bleble"}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Description, check.Equals, "bleble")
}

func (s *S) TestUpdateDescription(c *check.C) {
	app := appTypes.App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla"}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	updateData := appTypes.App{Name: "example", Description: "bleble"}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Description, check.Equals, "bleble")
}

func (s *S) TestUpdateAppPlatform(c *check.C) {
	app := appTypes.App{Name: "example", Platform: "python", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	updateData := appTypes.App{Name: "example", Platform: "heimerdinger"}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Platform, check.Equals, updateData.Platform)
	c.Assert(dbApp.UpdatePlatform, check.Equals, true)
}

func (s *S) TestUpdateAppPlatformWithVersion(c *check.C) {
	app := appTypes.App{Name: "example", Platform: "python", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	updateData := appTypes.App{Name: "example", Platform: "python:v3"}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Platform, check.Equals, "python")
	c.Assert(dbApp.PlatformVersion, check.Equals, "v3")
	c.Assert(dbApp.UpdatePlatform, check.Equals, true)
}

func (s *S) TestUpdateTeamOwner(c *check.C) {
	app := appTypes.App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla"}
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
	updateData := appTypes.App{Name: "example", TeamOwner: teamName}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.TeamOwner, check.Equals, teamName)
}

func (s *S) TestUpdateTeamOwnerNotExists(c *check.C) {
	app := appTypes.App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla"}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	updateData := appTypes.App{Name: "example", TeamOwner: "newowner"}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
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
	err = pool.AddTeamsToPool(context.TODO(), "test", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "test2"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), "test2", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	app := appTypes.App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
	err = CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app)
	updateData := appTypes.App{Name: "test", Pool: "test2"}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Pool, check.Equals, "test2")
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 1)
	c.Assert(s.provisioner.RestartsByVersion(dbApp, ""), check.Equals, 1)
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
	app := appTypes.App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
	err = CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app)
	err = AddUnits(context.TODO(), &app, 1, "", "", nil)
	c.Assert(err, check.IsNil)
	prov, err := getProvisioner(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	c.Assert(prov.GetName(), check.Equals, "fake1")
	c.Assert(p1.GetUnits(&app), check.HasLen, 1)
	c.Assert(p2.GetUnits(&app), check.HasLen, 0)
	c.Assert(p1.Provisioned(&app), check.Equals, true)
	c.Assert(p2.Provisioned(&app), check.Equals, false)
	updateData := appTypes.App{Name: "test", Pool: "test2"}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Pool, check.Equals, "test2")
	prov, err = getProvisioner(context.TODO(), dbApp)
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
	err = pool.AddTeamsToPool(context.TODO(), "test", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	app := appTypes.App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
	err = CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	updateData := appTypes.App{Name: "test", Pool: "test2"}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
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

	app := appTypes.App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
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
	err = AddUnits(context.TODO(), &app, 1, "", "", nil)
	c.Assert(err, check.IsNil)
	prov, err := getProvisioner(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	c.Assert(prov.GetName(), check.Equals, "fake1")
	c.Assert(p1.GetUnits(&app), check.HasLen, 1)
	c.Assert(p2.GetUnits(&app), check.HasLen, 0)
	c.Assert(p1.Provisioned(&app), check.Equals, true)
	c.Assert(p2.Provisioned(&app), check.Equals, false)
	updateData := appTypes.App{Name: "test", Pool: "test2"}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
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

	app := appTypes.App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
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
	err = AddUnits(context.TODO(), &app, 1, "", "", nil)
	c.Assert(err, check.IsNil)
	prov, err := getProvisioner(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	c.Assert(prov.GetName(), check.Equals, "fake1")
	c.Assert(p1.GetUnits(&app), check.HasLen, 1)
	c.Assert(p1.Provisioned(&app), check.Equals, true)
	updateData := appTypes.App{Name: "test", Pool: "test2"}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
}

func (s *S) TestUpdatePlan(c *check.C) {
	s.plan = appTypes.Plan{Name: "something", Memory: 268435456}
	a := appTypes.App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Memory: 536870912}, TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 3, "web", newSuccessfulAppVersion(c, &a), nil)
	updateData := appTypes.App{Name: "my-test-app", Plan: appTypes.Plan{Name: "something"}}
	err = Update(context.TODO(), &a, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Plan, check.DeepEquals, s.plan)
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 0)
}

func (s *S) TestUpdatePlanShouldRestart(c *check.C) {
	s.plan = appTypes.Plan{Name: "something", Memory: 268435456}
	a := appTypes.App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Memory: 536870912}, TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 3, "web", newSuccessfulAppVersion(c, &a), nil)
	updateData := appTypes.App{Name: "my-test-app", Plan: appTypes.Plan{Name: "something"}}
	err = Update(context.TODO(), &a, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer), ShouldRestart: true})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Plan, check.DeepEquals, s.plan)
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 1)
	c.Assert(s.provisioner.RestartsByVersion(dbApp, ""), check.Equals, 1)
}

func (s *S) TestUpdatePlanWithConstraint(c *check.C) {
	s.plan = appTypes.Plan{Name: "something", Memory: 268435456}
	err := pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{
		PoolExpr:  "pool1",
		Field:     pool.ConstraintTypePlan,
		Values:    []string{s.plan.Name},
		Blacklist: true,
	})
	c.Assert(err, check.IsNil)
	a := appTypes.App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Memory: 536870912}, TeamOwner: s.team.Name}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	updateData := appTypes.App{Name: "my-test-app", Plan: appTypes.Plan{Name: "something"}}
	err = Update(context.TODO(), &a, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.ErrorMatches, `App plan "something" is not allowed on pool "pool1"`)
}

func (s *S) TestUpdatePlanWithCPUBurstExceeds(c *check.C) {
	s.plan = appTypes.Plan{
		Name:     "something",
		Memory:   268435456,
		CPUBurst: &appTypes.CPUBurst{MaxAllowed: 1.8},
		Override: &appTypes.PlanOverride{CPUBurst: func(f float64) *float64 { return &f }(2)},
	}
	err := pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{
		PoolExpr:  "pool1",
		Field:     pool.ConstraintTypePlan,
		Values:    []string{s.plan.Name},
		Blacklist: true,
	})
	c.Assert(err, check.IsNil)
	a := appTypes.App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Memory: 536870912}, TeamOwner: s.team.Name}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	updateData := appTypes.App{Name: "my-test-app", Plan: appTypes.Plan{Name: "something"}}
	err = Update(context.TODO(), &a, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.ErrorMatches, `CPU burst exceeds the maximum allowed by plan \"something\"`)
}

func (s *S) TestUpdatePlanNoRouteChangeShouldRestart(c *check.C) {
	s.plan = appTypes.Plan{Name: "something", Memory: 268435456}
	a := appTypes.App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Memory: 536870912}, TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 3, "web", newSuccessfulAppVersion(c, &a), nil)
	updateData := appTypes.App{Name: "my-test-app", Plan: appTypes.Plan{Name: "something"}}
	err = Update(context.TODO(), &a, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer), ShouldRestart: true})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Plan, check.DeepEquals, s.plan)
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 1)
	c.Assert(s.provisioner.RestartsByVersion(dbApp, ""), check.Equals, 1)
}

func (s *S) TestUpdatePlanNotFound(c *check.C) {
	var app appTypes.App
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		return nil, appTypes.ErrPlanNotFound
	}
	updateData := appTypes.App{Name: "my-test-app", Plan: appTypes.Plan{Name: "some-unknown-plan"}}
	err := Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
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
	a := appTypes.App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Name: "old"}, TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 3, "web", newSuccessfulAppVersion(c, &a), nil)
	s.provisioner.PrepareFailure("Restart", fmt.Errorf("cannot restart app, I'm sorry"))
	updateData := appTypes.App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Name: "something"}}
	err = Update(context.TODO(), &a, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer), ShouldRestart: true})
	c.Assert(err, check.NotNil)
	dbApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Plan.Name, check.Equals, "old")
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 0)
}

func (s *S) TestUpdateIgnoresEmptyAndDuplicatedTags(c *check.C) {
	app := appTypes.App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla", Tags: []string{"tag1"}}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	updateData := appTypes.App{Tags: []string{"tag2 ", "  tag3  ", "", " tag3", "  "}}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Tags, check.DeepEquals, []string{"tag2", "tag3"})
}

func (s *S) TestUpdatePlatform(c *check.C) {
	app := appTypes.App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla"}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	updateData := appTypes.App{UpdatePlatform: true}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.UpdatePlatform, check.Equals, true)
}

func (s *S) TestUpdateWithEmptyTagsRemovesAllTags(c *check.C) {
	app := appTypes.App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla", Tags: []string{"tag1"}}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	updateData := appTypes.App{Description: "ble", Tags: []string{}}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Tags, check.DeepEquals, []string{})
}

func (s *S) TestUpdateWithoutTagsKeepsOriginalTags(c *check.C) {
	app := appTypes.App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla", Tags: []string{"tag1", "tag2"}}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	updateData := appTypes.App{Description: "ble", Tags: nil}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Tags, check.DeepEquals, []string{"tag1", "tag2"})
}

func (s *S) TestUpdateDescriptionPoolPlan(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test"}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), "test", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "test2"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), "test2", []string{s.team.Name})
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
	a := appTypes.App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Memory: 536870912}, Description: "blablabla", Pool: "test"}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(context.TODO(), &a, 3, "web", newSuccessfulAppVersion(c, &a), nil)
	updateData := appTypes.App{Name: "my-test-app", Plan: appTypes.Plan{Name: "something"}, Description: "bleble", Pool: "test2"}
	err = Update(context.TODO(), &a, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Plan, check.DeepEquals, plan)
	c.Assert(dbApp.Description, check.Equals, "bleble")
	c.Assert(dbApp.Pool, check.Equals, "test2")
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 1)
	c.Assert(s.provisioner.RestartsByVersion(dbApp, ""), check.Equals, 1)
}

func (s *S) TestUpdateMetadataWhenEmpty(c *check.C) {
	app := appTypes.App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla"}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)

	expectedMetadata := appTypes.Metadata{
		Annotations: []appTypes.MetadataItem{{Name: "a", Value: "b"}},
		Labels:      []appTypes.MetadataItem{{Name: "c", Value: "d"}},
	}

	updateData := appTypes.App{Metadata: expectedMetadata}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer), ShouldRestart: true})
	c.Assert(err, check.IsNil)

	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)

	expectedMetadataJSON, _ := json.Marshal(expectedMetadata)
	newMetadataJSON, err := json.Marshal(dbApp.Metadata)
	c.Assert(err, check.IsNil)

	c.Assert(string(expectedMetadataJSON), check.Equals, string(newMetadataJSON))
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 1)
}

func (s *S) TestUpdateMetadataWhenAlreadySet(c *check.C) {
	app := appTypes.App{
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

	expectedMetadata := appTypes.Metadata{
		Annotations: []appTypes.MetadataItem{{Name: "a", Value: "new"}},
		Labels:      []appTypes.MetadataItem{{Name: "c", Value: "d"}},
	}

	updateData := appTypes.App{Metadata: appTypes.Metadata{Annotations: []appTypes.MetadataItem{{Name: "a", Value: "new"}}}}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)

	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)

	expectedMetadataJSON, _ := json.Marshal(expectedMetadata)
	newMetadataJSON, err := json.Marshal(dbApp.Metadata)
	c.Assert(err, check.IsNil)

	c.Assert(string(expectedMetadataJSON), check.Equals, string(newMetadataJSON))
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 0)
}

func (s *S) TestUpdateMetadataCanRemoveAnnotation(c *check.C) {
	app := appTypes.App{
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

	updateData := appTypes.App{Metadata: appTypes.Metadata{Annotations: []appTypes.MetadataItem{{Name: "a", Delete: true}}}}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.IsNil)

	dbApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Metadata.Annotations, check.DeepEquals, []appTypes.MetadataItem{})
	c.Assert(dbApp.Metadata.Labels, check.DeepEquals, []appTypes.MetadataItem{{Name: "c", Value: "d"}})
}

func (s *S) TestUpdateMetadataAnnotationValidation(c *check.C) {
	app := appTypes.App{
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

	updateData := appTypes.App{Metadata: appTypes.Metadata{
		Annotations: []appTypes.MetadataItem{{Name: "_invalidName", Value: "asdf"}},
		Labels:      []appTypes.MetadataItem{{Name: "tsuru.io/app-name", Value: "asdf"}},
	}}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: new(bytes.Buffer)})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "multiple errors reported (2):\n"+
		"error #0: metadata.annotations: Invalid value: \"_invalidName\": name part must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyName',  or 'my.name',  or '123-abc', regex used for validation is '([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]')\n"+
		"error #1: prefix tsuru.io/ is private\n")
}

func (s *S) TestRenameTeam(c *check.C) {
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	apps := []any{
		appTypes.App{Name: "test1", TeamOwner: "t1", Routers: []appTypes.AppRouter{{Name: "fake"}}, Teams: []string{"t2", "t3", "t1"}},
		appTypes.App{Name: "test2", TeamOwner: "t2", Routers: []appTypes.AppRouter{{Name: "fake"}}, Teams: []string{"t3", "t1"}},
	}
	_, err = collection.InsertMany(context.TODO(), apps)
	c.Assert(err, check.IsNil)
	err = RenameTeam(context.TODO(), "t2", "t9000")
	c.Assert(err, check.IsNil)
	var dbApps []appTypes.App

	cursor, err := collection.Find(context.TODO(), mongoBSON.M{}, options.Find().SetSort(mongoBSON.M{"name": 1}))
	c.Assert(err, check.IsNil)

	err = cursor.All(context.TODO(), &dbApps)
	c.Assert(err, check.IsNil)

	c.Assert(dbApps, check.HasLen, 2)

	slices.Sort(dbApps[0].Teams)
	slices.Sort(dbApps[1].Teams)

	c.Assert(dbApps[0].TeamOwner, check.Equals, "t1")
	c.Assert(dbApps[1].TeamOwner, check.Equals, "t9000")
	c.Assert(dbApps[0].Teams, check.DeepEquals, []string{"t1", "t3", "t9000"})
	c.Assert(dbApps[1].Teams, check.DeepEquals, []string{"t1", "t3"})
}

func (s *S) TestRenameTeamLockedApp(c *check.C) {
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	apps := []any{
		appTypes.App{Name: "test1", TeamOwner: "t1", Routers: []appTypes.AppRouter{{Name: "fake"}}, Teams: []string{"t2", "t3", "t1"}},
		appTypes.App{Name: "test2", TeamOwner: "t2", Routers: []appTypes.AppRouter{{Name: "fake"}}, Teams: []string{"t3", "t1"}},
	}
	_, err = collection.InsertMany(context.TODO(), apps)
	c.Assert(err, check.IsNil)

	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: "test2"},
		Kind:     permission.PermAppUpdate,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	defer evt.Done(context.TODO(), nil)
	err = RenameTeam(context.TODO(), "t2", "t9000")
	c.Assert(err, check.ErrorMatches, `unable to create event: event locked: app\(test2\).*`)
	var dbApps []appTypes.App

	cursor, err := collection.Find(context.TODO(), mongoBSON.M{}, options.Find().SetSort(mongoBSON.M{"name": 1}))
	c.Assert(err, check.IsNil)

	err = cursor.All(context.TODO(), &dbApps)
	c.Assert(err, check.IsNil)

	c.Assert(dbApps, check.HasLen, 2)

	slices.Sort(dbApps[0].Teams)
	slices.Sort(dbApps[1].Teams)

	c.Assert(dbApps[0].TeamOwner, check.Equals, "t1")
	c.Assert(dbApps[1].TeamOwner, check.Equals, "t2")
	c.Assert(dbApps[0].Teams, check.DeepEquals, []string{"t1", "t2", "t3"})
	c.Assert(dbApps[1].Teams, check.DeepEquals, []string{"t1", "t3"})
}

func (s *S) TestRenameTeamUnchangedLockedApp(c *check.C) {
	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	apps := []any{
		appTypes.App{Name: "test1", TeamOwner: "t1", Routers: []appTypes.AppRouter{{Name: "fake"}}, Teams: []string{"t2", "t3", "t1"}},
		appTypes.App{Name: "test2", TeamOwner: "t2", Routers: []appTypes.AppRouter{{Name: "fake"}}, Teams: []string{"t3", "t1"}},
		appTypes.App{Name: "test3", TeamOwner: "t3", Routers: []appTypes.AppRouter{{Name: "fake"}}, Teams: []string{"t3", "t1"}},
	}
	_, err = collection.InsertMany(context.TODO(), apps)
	c.Assert(err, check.IsNil)

	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   eventTypes.Target{Type: "app", Value: "test3"},
		Kind:     permission.PermAppUpdate,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	defer evt.Done(context.TODO(), nil)
	err = RenameTeam(context.TODO(), "t2", "t9000")
	c.Assert(err, check.IsNil)
	var dbApps []appTypes.App

	cursor, err := collection.Find(context.TODO(), mongoBSON.M{}, options.Find().SetSort(mongoBSON.M{"name": 1}))
	c.Assert(err, check.IsNil)

	err = cursor.All(context.TODO(), &dbApps)
	c.Assert(err, check.IsNil)

	c.Assert(dbApps, check.HasLen, 3)

	slices.Sort(dbApps[0].Teams)
	slices.Sort(dbApps[1].Teams)

	c.Assert(dbApps[0].TeamOwner, check.Equals, "t1")
	c.Assert(dbApps[1].TeamOwner, check.Equals, "t9000")
	c.Assert(dbApps[0].Teams, check.DeepEquals, []string{"t1", "t3", "t9000"})
	c.Assert(dbApps[1].Teams, check.DeepEquals, []string{"t1", "t3"})
}

func (s *S) TestUpdateRouter(c *check.C) {
	config.Set("routers:fake:type", "fake")
	defer config.Unset("routers:fake:type")
	app := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name, Router: "none"}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	err = AddRouter(context.TODO(), &app, appTypes.AppRouter{
		Name: "fake",
		Opts: map[string]string{
			"a": "b",
		},
	})
	c.Assert(err, check.IsNil)
	err = UpdateRouter(context.TODO(), &app, appTypes.AppRouter{Name: "fake", Opts: map[string]string{
		"c": "d",
	}})
	c.Assert(err, check.IsNil)
	routers := GetRouters(&app)
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
	app := appTypes.App{Name: "myapp-with-error", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(context.TODO(), &app, 1, "web", nil, nil)
	c.Assert(err, check.IsNil)
	err = AddRouter(context.TODO(), &app, appTypes.AppRouter{
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
	app := appTypes.App{Name: "myapp-with-error", Platform: "go", TeamOwner: s.team.Name, Router: "none"}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	err = AddRouter(context.TODO(), &app, appTypes.AppRouter{
		Name: "fake",
		Opts: map[string]string{
			"a": "b",
		},
	})
	c.Assert(err, check.IsNil)
}

func (s *S) TestUpdateRouterNotFound(c *check.C) {
	app := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	err = AddRouter(context.TODO(), &app, appTypes.AppRouter{
		Name: "fake-tls",
		Opts: map[string]string{
			"a": "b",
		},
	})
	c.Assert(err, check.IsNil)
	err = UpdateRouter(context.TODO(), &app, appTypes.AppRouter{Name: "fake-opts", Opts: map[string]string{
		"c": "d",
	}})
	c.Assert(err, check.DeepEquals, &router.ErrRouterNotFound{Name: "fake-opts"})
}

func (s *S) TestAppAddRouter(c *check.C) {
	app := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)

	newSuccessfulAppVersion(c, &app)
	err = AddUnits(context.TODO(), &app, 1, "web", "", nil)
	c.Assert(err, check.IsNil)

	AddRouter(context.TODO(), &app, appTypes.AppRouter{
		Name: "fake-tls",
	})

	c.Assert(err, check.IsNil)
	routers, err := GetRoutersWithAddr(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Address: "myapp.fakerouter.com", Addresses: []string{"myapp.fakerouter.com"}, Type: "fake", Status: "ready"},
		{Name: "fake-tls", Address: "https://myapp.faketlsrouter.com", Addresses: []string{"https://myapp.faketlsrouter.com"}, Type: "fake-tls", Status: "ready"},
	})
	addrs, err := GetAddresses(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	c.Assert(addrs, check.DeepEquals, []string{"myapp.fakerouter.com", "https://myapp.faketlsrouter.com"})
}

func (s *S) TestAppAddRouterWithAlreadyLinkedRouter(c *check.C) {
	app := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	routers, err := GetRoutersWithAddr(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Status: "ready", Type: "fake", Address: "myapp.fakerouter.com", Addresses: []string{"myapp.fakerouter.com"}},
	})
	err = AddRouter(context.TODO(), &app, appTypes.AppRouter{Name: "fake"})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.DeepEquals, ErrRouterAlreadyLinked)
}

func (s *S) TestAppAddRouterWithAppCNameUsingSameRouterOnAnotherApp(c *check.C) {
	app1 := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake"}}}
	app2 := appTypes.App{Name: "myapp2", Platform: "go", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}}
	err := CreateApp(context.TODO(), &app1, s.user)
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), &app1, "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = CreateApp(context.TODO(), &app2, s.user)
	c.Assert(err, check.IsNil)
	err = AddCName(context.TODO(), &app2, "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = AddRouter(context.TODO(), &app1, appTypes.AppRouter{Name: "fake-tls"})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cname ktulu.mycompany.com already exists for app myapp2 using router fake-tls")
	err = AddRouter(context.TODO(), &app2, appTypes.AppRouter{Name: "fake"})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cname ktulu.mycompany.com already exists for app myapp using router fake")
}

func (s *S) TestAppRemoveRouter(c *check.C) {
	app := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)

	newSuccessfulAppVersion(c, &app)
	err = AddUnits(context.TODO(), &app, 1, "web", "", nil)
	c.Assert(err, check.IsNil)

	err = AddRouter(context.TODO(), &app, appTypes.AppRouter{
		Name: "fake-tls",
	})
	c.Assert(err, check.IsNil)
	err = RemoveRouter(context.TODO(), &app, "fake")
	c.Assert(err, check.IsNil)
	routers, err := GetRoutersWithAddr(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{
			Name:      "fake-tls",
			Addresses: []string{"https://myapp.faketlsrouter.com"},
			Address:   "https://myapp.faketlsrouter.com",
			Type:      "fake-tls",
			Status:    "ready",
		},
	})
	addrs, err := GetAddresses(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	c.Assert(addrs, check.DeepEquals, []string{"https://myapp.faketlsrouter.com"})
}

func (s *S) TestGetCertIssuers(c *check.C) {
	app := appTypes.App{
		Name:      "myapp",
		Platform:  "go",
		TeamOwner: s.team.Name,
		CName:     []string{"myapp.io", "myapp.another.io"},
		CertIssuers: map[string]string{
			"myapp.io":         "myissuer",
			"myapp.another.io": "myotherissuer",
		},
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	issuers := app.CertIssuers
	c.Assert(err, check.IsNil)
	c.Assert(issuers, check.DeepEquals, appTypes.CertIssuers{
		"myapp.io":         "myissuer",
		"myapp.another.io": "myotherissuer",
	})
}

func (s *S) TestGetRoutersWithAddr(c *check.C) {
	app := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)

	newSuccessfulAppVersion(c, &app)
	err = AddUnits(context.TODO(), &app, 1, "web", "", nil)
	c.Assert(err, check.IsNil)

	err = AddRouter(context.TODO(), &app, appTypes.AppRouter{
		Name: "fake-tls",
	})
	c.Assert(err, check.IsNil)
	s.mockService.Cache.OnCreate = func(entry cache.CacheEntry) error {
		if entry.Value != "myapp.fakerouter.com" && entry.Value != "https://myapp.faketlsrouter.com" {
			c.Errorf("unexpected cache entry: %v", entry)
		}
		return nil
	}
	routers, err := GetRoutersWithAddr(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Address: "myapp.fakerouter.com", Addresses: []string{"myapp.fakerouter.com"}, Type: "fake", Status: "ready"},
		{Name: "fake-tls", Address: "https://myapp.faketlsrouter.com", Addresses: []string{"https://myapp.faketlsrouter.com"}, Type: "fake-tls", Status: "ready"},
	})
}

func (s *S) TestGetRoutersWithAddrError(c *check.C) {
	routertest.FakeRouter.Reset()
	app := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)

	newSuccessfulAppVersion(c, &app)
	err = AddUnits(context.TODO(), &app, 1, "web", "", nil)
	c.Assert(err, check.IsNil)

	err = AddRouter(context.TODO(), &app, appTypes.AppRouter{
		Name: "fake-tls",
	})
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.FailuresByHost["fake:myapp"] = true

	s.mockService.Cache.OnCreate = func(entry cache.CacheEntry) error {
		if entry.Value != "https://myapp.faketlsrouter.com" {
			c.Errorf("unexpected cache entry: %v", entry)
		}
		return nil
	}
	routers, err := GetRoutersWithAddr(context.TODO(), &app)
	c.Assert(strings.Contains(err.Error(), "Forced failure"), check.Equals, true)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Address: "", Type: "fake", Status: "not ready", StatusDetail: "Forced failure"},
		{Name: "fake-tls", Address: "https://myapp.faketlsrouter.com", Addresses: []string{"https://myapp.faketlsrouter.com"}, Type: "fake-tls", Status: "ready"},
	})
}

func (s *S) TestGetRoutersWithAddrWithStatus(c *check.C) {
	routertest.FakeRouter.Status.Status = router.BackendStatusNotReady
	routertest.FakeRouter.Status.Detail = "burn"
	defer routertest.FakeRouter.Reset()
	app := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name, Router: "none"}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)

	newSuccessfulAppVersion(c, &app)
	err = AddUnits(context.TODO(), &app, 1, "web", "", nil)
	c.Assert(err, check.IsNil)

	err = AddRouter(context.TODO(), &app, appTypes.AppRouter{
		Name: "fake",
	})
	c.Assert(err, check.IsNil)
	s.mockService.Cache.OnCreate = func(entry cache.CacheEntry) error {
		if entry.Value != "myapp.fakerouter.com" {
			c.Errorf("unexpected cache entry: %v", entry)
		}
		return nil
	}
	routers, err := GetRoutersWithAddr(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Address: "myapp.fakerouter.com", Addresses: []string{"myapp.fakerouter.com"}, Type: "fake", Status: "not ready", StatusDetail: "burn"},
	})
}

func (s *S) TestGetRoutersIgnoresDuplicatedEntry(c *check.C) {
	app := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	err = AddRouter(context.TODO(), &app, appTypes.AppRouter{
		Name: "fake-tls",
	})
	c.Assert(err, check.IsNil)
	app.Router = "fake-tls"
	routers := GetRouters(&app)
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
	app := appTypes.App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
	err = CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &app)
	err = AddUnits(context.TODO(), &app, 1, "web", "", nil)
	c.Assert(err, check.IsNil)
	updateData := appTypes.App{Name: "test", Description: "updated description"}
	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &updateData, Writer: nil})
	c.Assert(err, check.IsNil)
	units, err := AppUnits(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	updatedApp, err := p1.GetAppFromUnitID(units[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(updatedApp.Description, check.Equals, "updated description")
}

func (s *S) TestUpdateAppPoolWithInvalidConstraint(c *check.C) {
	p1 := provisiontest.NewFakeProvisioner()
	p1.Name = "fake1"
	provision.Register("fake1", func() (provision.Provisioner, error) {
		return p1, nil
	})
	app := appTypes.App{Name: "test", TeamOwner: s.team.Name, Pool: s.Pool}
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
	err = service.Create(context.TODO(), svc)
	c.Assert(err, check.IsNil)
	si1 := service.ServiceInstance{
		Name:        "mydb",
		ServiceName: svc.Name,
		Apps:        []string{app.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), si1)
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

	err = Update(context.TODO(), &app, UpdateAppArgs{UpdateData: &appTypes.App{Pool: optsPool2.Name}, Writer: nil})
	c.Assert(err, check.NotNil)
}

func (s *S) TestInternalAddresses(c *check.C) {
	app := appTypes.App{Name: "test", TeamOwner: s.team.Name, Pool: s.Pool}

	addresses, err := internalAddresses(context.TODO(), &app)
	c.Assert(err, check.IsNil)

	c.Assert(addresses, check.HasLen, 4)
	c.Assert(addresses[0], check.DeepEquals, appTypes.AppInternalAddress{
		Domain:     "test-web.fake-cluster.local",
		Protocol:   "TCP",
		Process:    "web",
		Port:       80,
		TargetPort: 8080,
	})
	c.Assert(addresses[1], check.DeepEquals, appTypes.AppInternalAddress{
		Domain:     "test-logs.fake-cluster.local",
		Protocol:   "UDP",
		Process:    "logs",
		Port:       12201,
		TargetPort: 12201,
	})
	c.Assert(addresses[2], check.DeepEquals, appTypes.AppInternalAddress{
		Domain:     "test-logs-v2.fake-cluster.local",
		Protocol:   "UDP",
		Process:    "logs",
		Version:    "2",
		Port:       12201,
		TargetPort: 12201,
	})
	c.Assert(addresses[3], check.DeepEquals, appTypes.AppInternalAddress{
		Domain:     "test-web-v2.fake-cluster.local",
		Protocol:   "TCP",
		Process:    "web",
		Version:    "2",
		Port:       80,
		TargetPort: 8080,
	})
}

func (s *S) TestGetHealthcheckData(c *check.C) {
	a := appTypes.App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	hcData, err := GetHealthcheckData(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	c.Assert(hcData, check.DeepEquals, routerTypes.HealthcheckData{})
	newSuccessfulAppVersion(c, &a)
	hcData, err = GetHealthcheckData(context.TODO(), &a)
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
	a := appTypes.App{Name: "my-test-app", TeamOwner: s.team.Name}

	collection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = collection.InsertOne(context.TODO(), a)
	c.Assert(err, check.IsNil)
	newSuccessfulAppVersion(c, &a)
	hcData, err := GetHealthcheckData(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	c.Assert(hcData, check.DeepEquals, routerTypes.HealthcheckData{
		TCPOnly: true,
	})
}

func (s *S) TestAutoscaleWithAutoscaleProvisioner(c *check.C) {
	oldProvisioner := provision.DefaultProvisioner
	defer func() { provision.DefaultProvisioner = oldProvisioner }()
	provision.DefaultProvisioner = "autoscaleProv"
	autoScaleProv := &provisiontest.AutoScaleProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}
	provision.Register("autoscaleProv", func() (provision.Provisioner, error) {
		return autoScaleProv, nil
	})
	defer provision.Unregister("autoscaleProv")
	a := appTypes.App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := AutoScale(context.TODO(), &a, provTypes.AutoScaleSpec{Process: "p1"})
	c.Assert(err, check.IsNil)
	err = AutoScale(context.TODO(), &a, provTypes.AutoScaleSpec{Process: "p2"})
	c.Assert(err, check.IsNil)
	scales, err := AutoScaleInfo(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	c.Assert(scales, check.DeepEquals, []provTypes.AutoScaleSpec{
		{Process: "p1"},
		{Process: "p2"},
	})
	err = RemoveAutoScale(context.TODO(), &a, "p1")
	c.Assert(err, check.IsNil)
	scales, err = AutoScaleInfo(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	c.Assert(scales, check.DeepEquals, []provTypes.AutoScaleSpec{
		{Process: "p2"},
	})
}

func (s *S) TestGetInternalBindableAddresses(c *check.C) {
	app := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	addresses, err := GetInternalBindableAddresses(context.TODO(), &app)
	c.Assert(err, check.IsNil)
	c.Assert(addresses, check.DeepEquals, []string{
		"tcp://myapp-web.fake-cluster.local:80",
		"udp://myapp-logs.fake-cluster.local:12201",
	})
}
