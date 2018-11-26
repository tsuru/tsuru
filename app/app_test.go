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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/healer"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/safe"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/tsurutest"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/types/cache"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/quota"
	"github.com/tsuru/tsuru/volume"
	"gopkg.in/check.v1"
)

func (s *S) TestGetAppByName(c *check.C) {
	newApp := App{Name: "my-app", Platform: "Django", TeamOwner: s.team.Name}
	err := CreateApp(&newApp, s.user)
	c.Assert(err, check.IsNil)
	newApp.Env = map[string]bind.EnvVar{}
	err = s.conn.Apps().Update(bson.M{"name": newApp.Name}, &newApp)
	c.Assert(err, check.IsNil)
	myApp, err := GetByName("my-app")
	c.Assert(err, check.IsNil)
	c.Assert(myApp.Name, check.Equals, newApp.Name)
}

func (s *S) TestGetAppByNameNotFound(c *check.C) {
	app, err := GetByName("wat")
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	app, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	err = app.Log("msg", "src", "unit")
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(app.Name, "testimage")
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDelete,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	err = Delete(app, evt, "")
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasBackend(app.Name), check.Equals, false)
	_, err = GetByName(app.Name)
	c.Assert(err, check.Equals, appTypes.ErrAppNotFound)
	c.Assert(s.provisioner.Provisioned(&a), check.Equals, false)
	err = servicemanager.UserQuota.Inc(s.user.Email, 1)
	c.Assert(err, check.IsNil)
	count, err := s.logConn.Logs(app.Name).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
	_, err = repository.Manager().GetRepository(a.Name)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "repository not found")
	_, err = router.Retrieve(a.Name)
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
	imgs, err := image.ListAppImages(a.Name)
	c.Assert(err, check.NotNil)
	c.Assert(imgs, check.HasLen, 0)
}

func (s *S) TestDeleteWithEvents(c *check.C) {
	a := App{
		Name:      "ritual",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	app, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	evt1, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppUpdateEnvSet,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	err = evt1.Done(nil)
	c.Assert(err, check.IsNil)
	evt2, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: "other"},
		Kind:     permission.PermAppUpdateEnvSet,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	deleteEvt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDelete,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	Delete(app, deleteEvt, "")
	evts, err := event.List(&event.Filter{})
	c.Assert(err, check.IsNil)
	c.Assert(evts, eventtest.EvtEquals, evt2)
	evts, err = event.List(&event.Filter{IncludeRemoved: true})
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 3)
	_, err = repository.Manager().GetRepository(a.Name)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "repository not found")
}

func (s *S) TestDeleteWithoutUnits(c *check.C) {
	app := App{Name: "x4", Platform: "python", TeamOwner: s.team.Name}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	a, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDelete,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	Delete(a, evt, "")
	_, err = repository.Manager().GetRepository(app.Name)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "repository not found")
}

func (s *S) TestDeleteSwappedApp(c *check.C) {
	a := App{
		Name:      "ritual",
		Platform:  "ruby",
		Owner:     s.user.Email,
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	app2 := &App{Name: "app2", TeamOwner: s.team.Name}
	err = CreateApp(app2, s.user)
	c.Assert(err, check.IsNil)
	err = Swap(&a, app2, false)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDelete,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	err = Delete(&a, evt, "")
	c.Assert(err, check.ErrorMatches, "application is swapped with \"app2\", cannot remove it")
	c.Assert(s.provisioner.Provisioned(&a), check.Equals, true)
}

func (s *S) TestDeleteSwappedAppOnlyCname(c *check.C) {
	a := App{
		Name:      "ritual",
		Platform:  "ruby",
		Owner:     s.user.Email,
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	app2 := &App{Name: "app2", TeamOwner: s.team.Name}
	err = CreateApp(app2, s.user)
	c.Assert(err, check.IsNil)
	err = Swap(&a, app2, true)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDelete,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	err = Delete(&a, evt, "")
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.Provisioned(&a), check.Equals, false)
}

func (s *S) TestDeleteWithBoundVolumes(c *check.C) {
	a := App{
		Name:      "ritual",
		Platform:  "ruby",
		Owner:     s.user.Email,
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	v1 := volume.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volume.VolumePlan{Name: "nfs"}}
	err = v1.Create()
	c.Assert(err, check.IsNil)
	err = v1.BindApp(a.Name, "/mnt", false)
	c.Assert(err, check.IsNil)
	err = v1.BindApp(a.Name, "/mnt2", false)
	c.Assert(err, check.IsNil)
	app, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDelete,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	err = Delete(app, evt, "")
	c.Assert(err, check.IsNil)
	dbV, err := volume.Load(v1.Name)
	c.Assert(err, check.IsNil)
	binds, err := dbV.LoadBinds()
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
	s.mockService.UserQuota.OnInc = func(email string, q int) error {
		c.Assert(email, check.Equals, s.user.Email)
		return nil
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": 1}})
	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasBackend(a.Name), check.Equals, true)
	retrievedApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(retrievedApp.Name, check.Equals, a.Name)
	c.Assert(retrievedApp.Platform, check.Equals, a.Platform)
	c.Assert(retrievedApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(retrievedApp.Owner, check.Equals, s.user.Email)
	c.Assert(retrievedApp.Tags, check.DeepEquals, []string{"test a", "test b"})
	env := retrievedApp.Envs()
	c.Assert(env["TSURU_APPNAME"].Value, check.Equals, a.Name)
	c.Assert(env["TSURU_APPNAME"].Public, check.Equals, false)
	_, err = repository.Manager().GetRepository(a.Name)
	c.Assert(err, check.IsNil)
}

func (s *S) TestCreateAppAlreadyExists(c *check.C) {
	a := App{
		Name:      "appname",
		Platform:  "python",
		TeamOwner: s.team.Name,
		Tags:      []string{"", " test a  ", "  ", "test b ", " test a "},
	}
	s.mockService.UserQuota.OnInc = func(email string, q int) error {
		c.Assert(email, check.Equals, s.user.Email)
		return nil
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": 1}})
	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	ra := App{Name: "appname", Platform: "python", TeamOwner: s.team.Name, Pool: "invalid"}
	err = CreateApp(&ra, s.user)
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	retrievedApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(retrievedApp.Plan, check.DeepEquals, s.defaultPlan)
	_, err = repository.Manager().GetRepository(a.Name)
	c.Assert(err, check.IsNil)
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	retrievedApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(retrievedApp.GetRouters(), check.DeepEquals, []appTypes.AppRouter{{Name: "fake-tls", Opts: map[string]string{}}})
}

func (s *S) TestCreateAppWithExplicitPlan(c *check.C) {
	myPlan := appTypes.Plan{
		Name:     "myplan",
		Memory:   4194304,
		Swap:     2,
		CpuShare: 3,
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	retrievedApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(retrievedApp.Plan, check.DeepEquals, myPlan)
	_, err = repository.Manager().GetRepository(a.Name)
	c.Assert(err, check.IsNil)
}

func (s *S) TestCreateAppWithExplicitPlanConstraint(c *check.C) {
	myPlan := appTypes.Plan{
		Name:     "myplan",
		Memory:   4194304,
		Swap:     2,
		CpuShare: 3,
	}
	err := pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr:  "pool1",
		Field:     pool.ConstraintTypePlan,
		Values:    []string{myPlan.Name},
		Blacklist: true,
	})
	c.Assert(err, check.IsNil)
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
	err = CreateApp(&a, s.user)
	c.Assert(err, check.ErrorMatches, `App plan "myplan" is not allowed on pool "pool1"`)
}

func (s *S) TestCreateAppUserQuotaExceeded(c *check.C) {
	app := App{Name: "america", Platform: "python", TeamOwner: s.team.Name}
	s.conn.Users().Update(
		bson.M{"email": s.user.Email},
		bson.M{"$set": bson.M{"quota.limit": 1, "quota.inuse": 1}},
	)
	s.mockService.UserQuota.OnInc = func(email string, q int) error {
		c.Assert(email, check.Equals, s.user.Email)
		return &quota.QuotaExceededError{Available: 0, Requested: 1}
	}
	err := CreateApp(&app, s.user)
	e, ok := err.(*appTypes.AppCreationError)
	c.Assert(ok, check.Equals, true)
	qe, ok := e.Err.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(qe.Available, check.Equals, uint(0))
	c.Assert(qe.Requested, check.Equals, uint(1))
}

func (s *S) TestCreateAppTeamOwner(c *check.C) {
	app := App{Name: "america", Platform: "python", TeamOwner: "tsuruteam"}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	c.Assert(app.TeamOwner, check.Equals, "tsuruteam")
}

func (s *S) TestCreateAppTeamOwnerTeamNotFound(c *check.C) {
	app := App{
		Name:      "someapp",
		Platform:  "python",
		TeamOwner: "not found",
	}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "team not found")
}

func (s *S) TestCannotCreateAppWithoutTeamOwner(c *check.C) {
	u := auth.User{Email: "perpetual@yes.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	a := App{Name: "beyond"}
	err = CreateApp(&a, &u)
	c.Check(err, check.DeepEquals, &errors.ValidationError{Message: authTypes.ErrTeamNotFound.Error()})
}

func (s *S) TestCantCreateTwoAppsWithTheSameName(c *check.C) {
	err := CreateApp(&App{Name: "appname", TeamOwner: s.team.Name}, s.user)
	c.Assert(err, check.IsNil)
	a := App{Name: "appname", Platform: "python", TeamOwner: s.team.Name}
	err = CreateApp(&a, s.user)
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
	err := CreateApp(&a, s.user)
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.NotNil)
	expected := `tsuru failed to create the app "theirapp": exit status 1`
	c.Assert(err.Error(), check.Equals, expected)
	_, err = GetByName(a.Name)
	c.Assert(err, check.NotNil)
}

func (s *S) TestCreateAppRepositoryManagerFailure(c *check.C) {
	repository.Manager().CreateRepository("otherapp", nil)
	a := App{Name: "otherapp", Platform: "python", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.NotNil)
	count, err := s.conn.Apps().Find(bson.M{"name": a.Name}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
}

func (s *S) TestBindAndUnbindUnit(c *check.C) {
	var requests []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
	}))
	defer server.Close()
	app := App{
		Name: "warpaint", Platform: "python",
		Quota:     quota.UnlimitedQuota,
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	srvc := service.Service{
		Name:       "mysql",
		Endpoint:   map[string]string{"production": server.URL},
		Password:   "abcde",
		OwnerTeams: []string{s.team.Name},
	}
	err = service.Create(srvc)
	c.Assert(err, check.IsNil)
	si1 := service.ServiceInstance{
		Name:        "mydb",
		ServiceName: "mysql",
		Apps:        []string{app.Name},
	}
	err = s.conn.ServiceInstances().Insert(si1)
	c.Assert(err, check.IsNil)
	si2 := service.ServiceInstance{
		Name:        "yourdb",
		ServiceName: "mysql",
		Apps:        []string{app.Name},
	}
	err = s.conn.ServiceInstances().Insert(si2)
	c.Assert(err, check.IsNil)
	unit := provision.Unit{ID: "some-unit", IP: "127.0.2.1"}
	err = app.BindUnit(&unit)
	c.Assert(err, check.IsNil)
	err = app.UnbindUnit(&unit)
	c.Assert(err, check.IsNil)
	c.Assert(requests, check.HasLen, 4)
	c.Assert(requests[0].Method, check.Equals, "POST")
	c.Assert(requests[0].URL.Path, check.Equals, "/resources/mydb/bind")
	c.Assert(requests[1].Method, check.Equals, "POST")
	c.Assert(requests[1].URL.Path, check.Equals, "/resources/yourdb/bind")
	c.Assert(requests[2].Method, check.Equals, "DELETE")
	c.Assert(requests[2].URL.Path, check.Equals, "/resources/mydb/bind")
	c.Assert(requests[3].Method, check.Equals, "DELETE")
	c.Assert(requests[3].URL.Path, check.Equals, "/resources/yourdb/bind")
}

func (s *S) TestBindUnitWithError(c *check.C) {
	i := 0
	var requests []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		i++
		if i > 1 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("myerr"))
		}
	}))
	defer server.Close()
	app := App{
		Name: "warpaint", Platform: "python",
		Quota:     quota.UnlimitedQuota,
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	srvc := service.Service{
		Name:       "mysql",
		Endpoint:   map[string]string{"production": server.URL},
		Password:   "abcde",
		OwnerTeams: []string{s.team.Name},
	}
	err = service.Create(srvc)
	c.Assert(err, check.IsNil)
	si1 := service.ServiceInstance{
		Name:        "mydb",
		ServiceName: "mysql",
		Apps:        []string{app.Name},
	}
	err = s.conn.ServiceInstances().Insert(si1)
	c.Assert(err, check.IsNil)
	si2 := service.ServiceInstance{
		Name:        "yourdb",
		ServiceName: "mysql",
		Apps:        []string{app.Name},
	}
	err = s.conn.ServiceInstances().Insert(si2)
	c.Assert(err, check.IsNil)
	unit := provision.Unit{ID: "some-unit", IP: "127.0.2.1"}
	err = app.BindUnit(&unit)
	c.Assert(err, check.ErrorMatches, `Failed to bind the instance "mysql/yourdb" to the unit "127.0.2.1": invalid response: myerr \(code: 500\)`)
	c.Assert(requests, check.HasLen, 3)
	c.Assert(requests[0].Method, check.Equals, "POST")
	c.Assert(requests[0].URL.Path, check.Equals, "/resources/mydb/bind")
	c.Assert(requests[1].Method, check.Equals, "POST")
	c.Assert(requests[1].URL.Path, check.Equals, "/resources/yourdb/bind")
	c.Assert(requests[2].Method, check.Equals, "DELETE")
	c.Assert(requests[2].URL.Path, check.Equals, "/resources/mydb/bind")
}

func (s *S) TestAddUnits(c *check.C) {
	app := App{
		Name: "warpaint", Platform: "python",
		Quota:     quota.UnlimitedQuota,
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddUnits(5, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := app.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 5)
	err = app.AddUnits(2, "worker", nil)
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", nil)
	c.Assert(err, check.IsNil)
	err = a.Stop(nil, "web")
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot add units to an app that has stopped or sleeping units")
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
}

func (s *S) TestAddUnitsInSleepingApp(c *check.C) {
	a := App{
		Name: "sejuani", Platform: "python",
		TeamOwner: s.team.Name,
		Quota:     quota.UnlimitedQuota,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", nil)
	c.Assert(err, check.IsNil)
	err = a.Sleep(nil, "web", &url.URL{Scheme: "http", Host: "proxy:1234"})
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot add units to an app that has stopped or sleeping units")
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
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	err = app.AddUnits(2, "web", &buf)
	c.Assert(err, check.IsNil)
	units, err := app.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 2)
	for _, unit := range units {
		c.Assert(unit.AppName, check.Equals, app.Name)
	}
	c.Assert(buf.String(), check.Equals, "added 2 units")
}

func (s *S) TestAddUnitsQuota(c *check.C) {
	app := App{
		Name: "warpaint", Platform: "python",
		TeamOwner: s.team.Name, Quota: quota.Quota{Limit: 7, InUse: 0},
	}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	var inUseNow int
	s.mockService.AppQuota.OnInc = func(appName string, quantity int) error {
		inUseNow++
		c.Assert(appName, check.Equals, app.Name)
		c.Assert(quantity, check.Equals, 1)
		return nil
	}
	otherApp := App{Name: "warpaint"}
	for i := 1; i <= 7; i++ {
		err = otherApp.AddUnits(1, "web", nil)
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
	s.mockService.AppQuota.OnInc = func(appName string, quantity int) error {
		c.Assert(appName, check.Equals, app.Name)
		c.Assert(quantity, check.Equals, 1)
		return &quota.QuotaExceededError{Available: 0, Requested: 1}
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	err = app.AddUnits(1, "web", nil)
	e, ok := err.(*quota.QuotaExceededError)
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
	s.mockService.AppQuota.OnInc = func(appName string, quantity int) error {
		c.Assert(appName, check.Equals, app.Name)
		c.Assert(quantity, check.Equals, 10)
		return nil
	}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddUnits(10, "web", nil)
	c.Assert(err, check.IsNil)
	units := s.provisioner.GetUnits(&app)
	c.Assert(units, check.HasLen, 10)
}

func (s *S) TestAddZeroUnits(c *check.C) {
	app := App{Name: "warpaint", Platform: "ruby"}
	err := app.AddUnits(0, "web", nil)
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
	err = app.AddUnits(2, "web", nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "App is not provisioned.")
}

func (s *S) TestAddUnitsIsAtomic(c *check.C) {
	app := App{
		Name: "warpaint", Platform: "golang",
		Quota: quota.UnlimitedQuota,
	}
	err := app.AddUnits(2, "web", nil)
	c.Assert(err, check.NotNil)
	_, err = GetByName(app.Name)
	c.Assert(err, check.Equals, appTypes.ErrAppNotFound)
}

func (s *S) TestRemoveUnitsWithQuota(c *check.C) {
	a := App{
		Name:      "ble",
		TeamOwner: s.team.Name,
	}
	s.mockService.AppQuota.OnSetLimit = func(appName string, quantity int) error {
		c.Assert(appName, check.Equals, a.Name)
		c.Assert(quantity, check.Equals, 6)

		return nil
	}
	setCalls := 0
	s.mockService.AppQuota.OnSet = func(appName string, quantity int) error {
		c.Assert(appName, check.Equals, a.Name)
		setCalls++
		if setCalls == 1 {
			c.Assert(quantity, check.Equals, 6)
		} else {
			c.Assert(quantity, check.Equals, 2)
		}
		return nil
	}
	s.mockService.AppQuota.OnInc = func(appName string, quantity int) error {
		c.Assert(appName, check.Equals, a.Name)
		c.Assert(quantity, check.Equals, 4)
		return nil
	}
	s.mockService.AppQuota.OnGet = func(appName string) (*quota.Quota, error) {
		c.Assert(appName, check.Equals, a.Name)
		return &quota.Quota{Limit: 6, InUse: 2}, nil
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetQuotaLimit(6)
	c.Assert(err, check.IsNil)
	err = a.SetQuotaInUse(6)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 6, "web", nil)
	err = a.RemoveUnits(4, "web", nil)
	c.Assert(err, check.IsNil)
	quota, err := servicemanager.AppQuota.Get(a.Name)
	c.Assert(err, check.IsNil)
	err = tsurutest.WaitCondition(2e9, func() bool {
		return quota.InUse == 2
	})
	c.Assert(err, check.IsNil)
	c.Assert(setCalls, check.Equals, 2)
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
	err = CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddUnits(2, "web", nil)
	c.Assert(err, check.IsNil)
	err = app.AddUnits(2, "worker", nil)
	c.Assert(err, check.IsNil)
	err = app.AddUnits(2, "web", nil)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(nil)
	err = app.RemoveUnits(2, "worker", buf)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "removing 2 units")
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
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&app, 3, "web", nil)
	for _, test := range tests {
		err := app.RemoveUnits(test.n, "web", nil)
		c.Check(err, check.ErrorMatches, test.expected)
	}
}

func (s *S) TestSetUnitStatus(c *check.C) {
	a := App{Name: "app-name", Platform: "python", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	err = a.SetUnitStatus(units[0].ID, provision.StatusError)
	c.Assert(err, check.IsNil)
	units, err = a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units[0].Status, check.Equals, provision.StatusError)
}

func (s *S) TestSetUnitStatusPartialID(c *check.C) {
	a := App{Name: "app-name", Platform: "python", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	name := units[0].ID
	err = a.SetUnitStatus(name[0:len(name)-2], provision.StatusError)
	c.Assert(err, check.IsNil)
	units, err = a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units[0].Status, check.Equals, provision.StatusError)
}

func (s *S) TestSetUnitStatusNotFound(c *check.C) {
	a := App{Name: "app-name", Platform: "django"}
	err := a.SetUnitStatus("someunit", provision.StatusError)
	c.Assert(err, check.NotNil)
	e, ok := err.(*provision.UnitNotFoundError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.ID, check.Equals, "someunit")
}

func (s *S) TestUpdateNodeStatus(c *check.C) {
	a := App{Name: "lapname", Platform: "python", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "addr1",
	})
	c.Assert(err, check.IsNil)
	units, err := s.provisioner.AddUnitsToNode(&a, 3, "web", nil, "addr1")
	c.Assert(err, check.IsNil)
	unitStates := []provision.UnitStatusData{
		{ID: units[0].ID, Status: provision.Status("started")},
		{ID: units[1].ID, Status: provision.Status("stopped")},
		{ID: units[2].ID, Status: provision.Status("error")},
		{ID: units[2].ID + "-not-found", Status: provision.Status("error")},
	}
	result, err := UpdateNodeStatus(provision.NodeStatusData{Addrs: []string{"addr1"}, Units: unitStates})
	c.Assert(err, check.IsNil)
	expected := []UpdateUnitsResult{
		{ID: units[0].ID, Found: true},
		{ID: units[1].ID, Found: true},
		{ID: units[2].ID, Found: true},
		{ID: units[2].ID + "-not-found", Found: false},
	}
	c.Assert(result, check.DeepEquals, expected)
}

func (s *S) TestUpdateNodeStatusProvError(c *check.C) {
	_, err := healer.Initialize()
	c.Assert(err, check.IsNil)
	defer func() {
		healer.HealerInstance.Shutdown(context.Background())
		healer.HealerInstance = nil
	}()

	a := App{Name: "lapname", Platform: "python", TeamOwner: s.team.Name}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "addr1",
	})
	c.Assert(err, check.IsNil)

	_, err = UpdateNodeStatus(provision.NodeStatusData{Addrs: []string{"addr1", "addr2"}})
	c.Assert(err, check.IsNil)

	s.provisioner.PrepareFailure("NodeForNodeData", stderrors.New("myerror"))
	_, err = UpdateNodeStatus(provision.NodeStatusData{Addrs: []string{"addr1", "addr2"}})
	c.Assert(err, check.ErrorMatches, "myerror")

	node, err := s.provisioner.GetNode("addr1")
	c.Assert(err, check.IsNil)
	nodeData, err := healer.HealerInstance.GetNodeStatusData(node)
	c.Assert(err, check.IsNil)
	c.Assert(nodeData.Checks, check.HasLen, 2)
}

func (s *S) TestUpdateNodeStatusProvErrorNotFoundInHealer(c *check.C) {
	_, err := healer.Initialize()
	c.Assert(err, check.IsNil)
	defer func() {
		healer.HealerInstance.Shutdown(context.Background())
		healer.HealerInstance = nil
	}()

	a := App{Name: "lapname", Platform: "python", TeamOwner: s.team.Name}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "addr1",
	})
	c.Assert(err, check.IsNil)

	s.provisioner.PrepareFailure("NodeForNodeData", stderrors.New("myerror"))
	_, err = UpdateNodeStatus(provision.NodeStatusData{Addrs: []string{"addr1", "addr2"}})
	c.Assert(err, check.ErrorMatches, "myerror")

	node, err := s.provisioner.GetNode("addr1")
	c.Assert(err, check.IsNil)
	_, err = healer.HealerInstance.GetNodeStatusData(node)
	c.Assert(err, check.ErrorMatches, "node not found")
}

func (s *S) TestUpdateNodeStatusProvErrorSingleAddr(c *check.C) {
	_, err := healer.Initialize()
	c.Assert(err, check.IsNil)
	defer func() {
		healer.HealerInstance.Shutdown(context.Background())
		healer.HealerInstance = nil
	}()

	a := App{Name: "lapname", Platform: "python", TeamOwner: s.team.Name}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "addr1",
	})
	c.Assert(err, check.IsNil)

	s.provisioner.PrepareFailure("GetNode", stderrors.New("myerror"))
	_, err = UpdateNodeStatus(provision.NodeStatusData{Addrs: []string{"addr1"}})
	c.Assert(err, check.ErrorMatches, "myerror")

	node, err := s.provisioner.GetNode("addr1")
	c.Assert(err, check.IsNil)
	nodeData, err := healer.HealerInstance.GetNodeStatusData(node)
	c.Assert(err, check.IsNil)
	c.Assert(nodeData.Checks, check.HasLen, 1)
}

func (s *S) TestUpdateNodeStatusNotFound(c *check.C) {
	a := App{Name: "lapname", Platform: "python", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	units, err := s.provisioner.AddUnitsToNode(&a, 3, "web", nil, "addr1")
	c.Assert(err, check.IsNil)
	unitStates := []provision.UnitStatusData{
		{ID: units[0].ID, Status: provision.Status("started")},
		{ID: units[1].ID, Status: provision.Status("stopped")},
		{ID: units[2].ID, Status: provision.Status("error")},
		{ID: units[2].ID + "-not-found", Status: provision.Status("error")},
	}
	result, err := UpdateNodeStatus(provision.NodeStatusData{Addrs: []string{"addr1"}, Units: unitStates})
	c.Assert(err, check.IsNil)
	expected := []UpdateUnitsResult{
		{ID: units[0].ID, Found: false},
		{ID: units[1].ID, Found: false},
		{ID: units[2].ID, Found: false},
		{ID: units[2].ID + "-not-found", Found: false},
	}
	c.Assert(result, check.DeepEquals, expected)
}

func (s *S) TestUpdateNodeStatusUnrelatedProvError(c *check.C) {
	p1 := provisiontest.NewFakeProvisioner()
	p1.Name = "fake1"
	provision.Register("fake1", func() (provision.Provisioner, error) {
		return p1, nil
	})
	defer provision.Unregister("fake1")
	s.provisioner.PrepareFailure("NodeForNodeData", stderrors.New("myerror"))

	_, err := healer.Initialize()
	c.Assert(err, check.IsNil)
	defer func() {
		healer.HealerInstance.Shutdown(context.Background())
		healer.HealerInstance = nil
	}()

	a := App{Name: "lapname", Platform: "python", TeamOwner: s.team.Name}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = p1.AddNode(provision.AddNodeOptions{
		Address: "addr1",
	})
	c.Assert(err, check.IsNil)

	_, err = UpdateNodeStatus(provision.NodeStatusData{Addrs: []string{"addr1", "addr2"}})
	c.Check(err, check.IsNil)

	node, err := p1.GetNode("addr1")
	c.Assert(err, check.IsNil)
	nodeData, err := healer.HealerInstance.GetNodeStatusData(node)
	c.Assert(err, check.IsNil)
	c.Assert(nodeData.Checks, check.HasLen, 1)
}

func (s *S) TestGrantAccess(c *check.C) {
	user, _ := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	app := App{Name: "app-name", Platform: "django", Teams: []string{"acid-rain", "zito"}}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	err = repository.Manager().CreateRepository(app.Name, nil)
	c.Assert(err, check.IsNil)
	err = app.Grant(&s.team)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Find(bson.M{"name": app.Name}).One(&app)
	c.Assert(err, check.IsNil)
	c.Assert(app.Teams, check.DeepEquals, []string{"acid-rain", "zito", s.team.Name})
	grants, err := repositorytest.Granted(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(grants, check.DeepEquals, []string{user.Email})
}

func (s *S) TestGrantAccessFailsIfTheTeamAlreadyHasAccessToTheApp(c *check.C) {
	a := App{Name: "app-name", Platform: "django", Teams: []string{s.team.Name}}
	err := a.Grant(&s.team)
	c.Assert(err, check.Equals, ErrAlreadyHaveAccess)
}

func (s *S) TestRevokeAccess(c *check.C) {
	user, _ := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	team := authTypes.Team{Name: "abcd"}
	app := App{Name: "app-name", Platform: "django", Teams: []string{s.team.Name, team.Name}}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	err = repository.Manager().CreateRepository(app.Name, nil)
	c.Assert(err, check.IsNil)
	err = repository.Manager().GrantAccess(app.Name, user.Email)
	c.Assert(err, check.IsNil)
	err = app.Revoke(&s.team)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Find(bson.M{"name": app.Name}).One(&app)
	c.Assert(err, check.IsNil)
	_, found := app.findTeam(&s.team)
	c.Assert(found, check.Equals, false)
	grants, err := repositorytest.Granted(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(grants, check.HasLen, 0)
}

func (s *S) TestRevokeAccessKeepsUsersThatBelongToTwoTeams(c *check.C) {
	team := authTypes.Team{Name: "abcd"}
	user, _ := permissiontest.CustomUserWithPermission(c, nativeScheme, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permTypes.CtxTeam, team.Name),
	})
	app := App{Name: "app-name", Platform: "django", Teams: []string{s.team.Name, team.Name}}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	err = repository.Manager().CreateRepository(app.Name, nil)
	c.Assert(err, check.IsNil)
	err = repository.Manager().GrantAccess(app.Name, user.Email)
	c.Assert(err, check.IsNil)
	err = app.Revoke(&team)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Find(bson.M{"name": app.Name}).One(&app)
	c.Assert(err, check.IsNil)
	_, found := app.findTeam(&team)
	c.Assert(found, check.Equals, false)
	grants, err := repositorytest.Granted(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(grants, check.DeepEquals, []string{user.Email})
}

func (s *S) TestRevokeAccessDoesntLeaveOrphanApps(c *check.C) {
	app := App{Name: "app-name", Platform: "django", TeamOwner: s.team.Name}
	err := CreateApp(&app, s.user)
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
	env := bind.EnvVar{Name: "PATH", Value: "/"}
	a.setEnv(env)
	c.Assert(a.Env, check.NotNil)
}

func (s *S) TestSetPublicEnvironmentVariableToApp(c *check.C) {
	a := App{Name: "app-name", Platform: "django"}
	a.setEnv(bind.EnvVar{Name: "PATH", Value: "/", Public: true})
	env := a.Env["PATH"]
	c.Assert(env.Name, check.Equals, "PATH")
	c.Assert(env.Value, check.Equals, "/")
	c.Assert(env.Public, check.Equals, true)
}

func (s *S) TestSetPrivateEnvironmentVariableToApp(c *check.C) {
	a := App{Name: "app-name", Platform: "django"}
	a.setEnv(bind.EnvVar{Name: "PATH", Value: "/", Public: false})
	env := a.Env["PATH"]
	c.Assert(env.Name, check.Equals, "PATH")
	c.Assert(env.Value, check.Equals, "/")
	c.Assert(env.Public, check.Equals, false)
}

func (s *S) TestSetMultiplePublicEnvironmentVariableToApp(c *check.C) {
	a := App{Name: "app-name", Platform: "django"}
	a.setEnv(bind.EnvVar{Name: "PATH", Value: "/", Public: true})
	a.setEnv(bind.EnvVar{Name: "DATABASE", Value: "mongodb", Public: true})
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
	a.setEnv(bind.EnvVar{Name: "PATH", Value: "/", Public: false})
	a.setEnv(bind.EnvVar{Name: "DATABASE", Value: "mongodb", Public: false})
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
		ServiceEnvs: []bind.ServiceEnvVar{
			{
				EnvVar: bind.EnvVar{
					Name:   "DATABASE_HOST",
					Value:  "localhost",
					Public: false,
				},
				InstanceName: "some service",
			},
		},
		TeamOwner: s.team.Name,
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", nil)
	c.Assert(err, check.IsNil)
	envs := []bind.EnvVar{
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
	c.Assert(err, check.IsNil)
	newApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	expected := map[string]bind.EnvVar{
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
	}
	newAppEnvs := newApp.Envs()
	delete(newAppEnvs, TsuruServicesEnvVar)
	c.Assert(newAppEnvs, check.DeepEquals, expected)
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 1)
	c.Assert(buf.String(), check.Equals, "---- Setting 2 new environment variables ----\nrestarting app")
}

func (s *S) TestSetEnvWithNoRestartFlag(c *check.C) {
	a := App{
		Name: "myapp",
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST": {
				Name:   "DATABASE_HOST",
				Value:  "localhost",
				Public: false,
			},
		},
		TeamOwner: s.team.Name,
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	envs := []bind.EnvVar{
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
	newApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	expected := map[string]bind.EnvVar{
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
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST": {
				Name:   "DATABASE_HOST",
				Value:  "localhost",
				Public: false,
			},
		},
		TeamOwner: s.team.Name,
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	envs := []bind.EnvVar{
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
	newApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	expected := map[string]bind.EnvVar{
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
	err := CreateApp(&a, s.user)
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
		envs := []bind.EnvVar{
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
		Env: map[string]bind.EnvVar{
			"DATABASE_PASSWORD": {
				Name:   "DATABASE_PASSWORD",
				Value:  "123",
				Public: false,
			},
		},
		ServiceEnvs: []bind.ServiceEnvVar{
			{
				EnvVar: bind.EnvVar{
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", nil)
	c.Assert(err, check.IsNil)
	err = a.UnsetEnvs(bind.UnsetEnvArgs{
		VariableNames: []string{"DATABASE_HOST", "DATABASE_PASSWORD"},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	newApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "localhost",
			Public: false,
		},
	}
	newAppEnvs := newApp.Envs()
	delete(newAppEnvs, TsuruServicesEnvVar)
	c.Assert(newAppEnvs, check.DeepEquals, expected)
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 1)
}

func (s *S) TestUnsetEnvWithNoRestartFlag(c *check.C) {
	a := App{
		Name: "myapp",
		Env: map[string]bind.EnvVar{
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", nil)
	c.Assert(err, check.IsNil)
	err = a.UnsetEnvs(bind.UnsetEnvArgs{
		VariableNames: []string{"DATABASE_HOST", "DATABASE_PASSWORD"},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	newApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(newApp.Env, check.DeepEquals, map[string]bind.EnvVar{})
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 0)
}
func (s *S) TestUnsetEnvNoUnits(c *check.C) {
	a := App{
		Name: "myapp",
		Env: map[string]bind.EnvVar{
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.UnsetEnvs(bind.UnsetEnvArgs{
		VariableNames: []string{"DATABASE_HOST", "DATABASE_PASSWORD"},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	newApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(newApp.Env, check.DeepEquals, map[string]bind.EnvVar{})
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 0)
}

func (s *S) TestGetEnvironmentVariableFromApp(c *check.C) {
	a := App{Name: "whole-lotta-love"}
	a.setEnv(bind.EnvVar{Name: "PATH", Value: "/"})
	v, err := a.getEnv("PATH")
	c.Assert(err, check.IsNil)
	c.Assert(v.Value, check.Equals, "/")
}

func (s *S) TestGetEnvReturnsErrorIfTheVariableIsNotDeclared(c *check.C) {
	a := App{Name: "what-is-and-what-should-never"}
	a.Env = make(map[string]bind.EnvVar)
	_, err := a.getEnv("PATH")
	c.Assert(err, check.NotNil)
}

func (s *S) TestGetEnvReturnsErrorIfTheEnvironmentMapIsNil(c *check.C) {
	a := App{Name: "what-is-and-what-should-never"}
	_, err := a.getEnv("PATH")
	c.Assert(err, check.NotNil)
}

func (s *S) TestInstanceEnvironmentReturnEnvironmentVariablesForTheServer(c *check.C) {
	envs := []bind.ServiceEnvVar{
		{EnvVar: bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, ServiceName: "srv1", InstanceName: "mysql"},
		{EnvVar: bind.EnvVar{Name: "DATABASE_USER", Value: "root"}, ServiceName: "srv1", InstanceName: "mysql"},
		{EnvVar: bind.EnvVar{Name: "DATABASE_HOST", Value: "postgresaddr"}, ServiceName: "srv2", InstanceName: "postgres"},
		{EnvVar: bind.EnvVar{Name: "HOST", Value: "10.0.2.1"}, ServiceName: "srv3", InstanceName: "redis"},
	}
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": {Name: "DATABASE_HOST", Value: "localhost"},
		"DATABASE_USER": {Name: "DATABASE_USER", Value: "root"},
	}
	a := App{Name: "hi-there", ServiceEnvs: envs}
	c.Assert(a.InstanceEnvs("srv1", "mysql"), check.DeepEquals, expected)
}

func (s *S) TestInstanceEnvironmentDoesNotPanicIfTheEnvMapIsNil(c *check.C) {
	a := App{Name: "hi-there"}
	c.Assert(a.InstanceEnvs("srv1", "mysql"), check.DeepEquals, map[string]bind.EnvVar{})
}

func (s *S) TestAddCName(c *check.C) {
	app := &App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	app, err = GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{"ktulu.mycompany.com"})
	err = app.AddCName("ktulu2.mycompany.com")
	c.Assert(err, check.IsNil)
	app, err = GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{"ktulu.mycompany.com", "ktulu2.mycompany.com"})
}

func (s *S) TestAddCNameCantBeDuplicated(c *check.C) {
	app := &App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = app.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cname already exists!")
	app2 := &App{Name: "ktulu2", TeamOwner: s.team.Name}
	err = CreateApp(app2, s.user)
	c.Assert(err, check.IsNil)
	err = app2.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cname already exists!")
}

func (s *S) TestAddCNameWithWildCard(c *check.C) {
	app := &App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddCName("*.mycompany.com")
	c.Assert(err, check.IsNil)
	app, err = GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{"*.mycompany.com"})
}

func (s *S) TestAddCNameErrsOnEmptyCName(c *check.C) {
	app := &App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddCName("")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Invalid cname")
}

func (s *S) TestAddCNameErrsOnInvalid(c *check.C) {
	app := &App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddCName("_ktulu.mycompany.com")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Invalid cname")
}

func (s *S) TestAddCNamePartialUpdate(c *check.C) {
	a := &App{Name: "master", Platform: "puppet", TeamOwner: s.team.Name}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	other := App{Name: a.Name, Routers: []appTypes.AppRouter{{Name: "fake"}}}
	err = other.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
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
	err := CreateApp(&a, s.user)
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
	err := CreateApp(&a, s.user)
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
	err := CreateApp(&a, s.user)
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
	err := CreateApp(&a, s.user)
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
	a := App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddCName("ktulu2.mycompany.com")
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.FailForIp("ktulu3.mycompany.com")
	err = a.AddCName("ktulu3.mycompany.com")
	c.Assert(err, check.ErrorMatches, "Forced failure")
	c.Assert(a.CName, check.DeepEquals, []string{"ktulu2.mycompany.com"})
	hasCName := routertest.FakeRouter.HasCNameFor(a.Name, "ktulu3.mycompany.com")
	c.Assert(hasCName, check.Equals, false)
}

func (s *S) TestAddCnameRollbackWithDatabaseFailure(c *check.C) {
	a := App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddCName("ktulu2.mycompany.com", "ktulu3.mycompany.com", "ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = a.RemoveCName("ktulu2.mycompany.com", "test.com")
	c.Assert(err, check.NotNil)
	c.Assert(a.CName, check.DeepEquals, []string{"ktulu2.mycompany.com", "ktulu3.mycompany.com", "ktulu.mycompany.com"})
	hasCName := routertest.FakeRouter.HasCNameFor(a.Name, "ktulu2.mycompany.com")
	c.Assert(hasCName, check.Equals, true)
	routertest.FakeRouter.FailForIp("ktulu2.mycompany.com")
	err = a.RemoveCName("ktulu2.mycompany.com")
	c.Assert(err, check.ErrorMatches, "Forced failure")
	c.Assert(a.CName, check.DeepEquals, []string{"ktulu2.mycompany.com", "ktulu3.mycompany.com", "ktulu.mycompany.com"})
}

func (s *S) TestRemoveCNameRemovesFromDatabase(c *check.C) {
	a := &App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = a.RemoveCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(a.CName, check.DeepEquals, []string{})
}

func (s *S) TestRemoveCNameWhichNoExists(c *check.C) {
	a := &App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = a.RemoveCName("ktulu.mycompany.com")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cname ktulu.mycompany.com not exists in app")
}

func (s *S) TestRemoveMoreThanOneCName(c *check.C) {
	a := &App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddCName("ktulu.mycompany.com")
	c.Assert(err, check.IsNil)
	err = a.AddCName("ktulu2.mycompany.com")
	c.Assert(err, check.IsNil)
	err = a.RemoveCName("ktulu.mycompany.com", "ktulu2.mycompany.com")
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(a.CName, check.DeepEquals, []string{})
}

func (s *S) TestRemoveCNameRemovesFromRouter(c *check.C) {
	a := App{Name: "ktulu", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
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
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bind.ServiceEnvVar{
			{EnvVar: bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "srv1"},
			{EnvVar: bind.EnvVar{Name: "DATABASE_PORT", Value: "3306"}, InstanceName: "myinstance", ServiceName: "srv1"},
			{EnvVar: bind.EnvVar{Name: "DATABASE_USER", Value: "root"}, InstanceName: "myinstance", ServiceName: "srv1"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	serviceEnv := allEnvs[TsuruServicesEnvVar]
	c.Assert(serviceEnv.Name, check.Equals, TsuruServicesEnvVar)
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
	delete(allEnvs, TsuruServicesEnvVar)
	delete(allEnvs, "TSURU_APPDIR")
	delete(allEnvs, "TSURU_APPNAME")
	delete(allEnvs, "TSURU_APP_TOKEN")
	c.Assert(allEnvs, check.DeepEquals, map[string]bind.EnvVar{
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
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bind.ServiceEnvVar{
			{EnvVar: bind.EnvVar{Name: "ZMQ_PEER", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "srv1"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	// inserts duplicated
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bind.ServiceEnvVar{
			{EnvVar: bind.EnvVar{Name: "ZMQ_PEER", Value: "8.8.8.8"}, InstanceName: "myinstance", ServiceName: "srv1"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	serviceEnv := allEnvs[TsuruServicesEnvVar]
	c.Assert(serviceEnv.Name, check.Equals, TsuruServicesEnvVar)
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
	c.Assert(allEnvs["ZMQ_PEER"], check.DeepEquals, bind.EnvVar{
		Name:   "ZMQ_PEER",
		Value:  "8.8.8.8",
		Public: false,
	})
}

func (s *S) TestAddInstanceWithUnits(c *check.C) {
	a := &App{Name: "dark", Quota: quota.Quota{Limit: 10}, TeamOwner: s.team.Name}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", nil)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bind.ServiceEnvVar{
			{EnvVar: bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "myservice"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	serviceEnv := allEnvs[TsuruServicesEnvVar]
	c.Assert(serviceEnv.Name, check.Equals, TsuruServicesEnvVar)
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
	c.Assert(allEnvs["DATABASE_HOST"], check.DeepEquals, bind.EnvVar{
		Name:   "DATABASE_HOST",
		Value:  "localhost",
		Public: false,
	})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 1)
}

func (s *S) TestAddInstanceWithUnitsNoRestart(c *check.C) {
	a := &App{Name: "dark", Quota: quota.Quota{Limit: 10}, TeamOwner: s.team.Name}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", nil)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bind.ServiceEnvVar{
			{EnvVar: bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost"}, InstanceName: "myinstance", ServiceName: "myservice"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	serviceEnv := allEnvs[TsuruServicesEnvVar]
	c.Assert(serviceEnv.Name, check.Equals, TsuruServicesEnvVar)
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
	c.Assert(allEnvs["DATABASE_HOST"], check.DeepEquals, bind.EnvVar{
		Name:   "DATABASE_HOST",
		Value:  "localhost",
		Public: false,
	})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 0)
}

func (s *S) TestAddInstanceMultipleServices(c *check.C) {
	a := &App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bind.ServiceEnvVar{
			{EnvVar: bind.EnvVar{Name: "DATABASE_HOST", Value: "host1"}, InstanceName: "instance1", ServiceName: "mysql"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bind.ServiceEnvVar{
			{EnvVar: bind.EnvVar{Name: "DATABASE_HOST", Value: "host2"}, InstanceName: "instance2", ServiceName: "mysql"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bind.ServiceEnvVar{
			{EnvVar: bind.EnvVar{Name: "DATABASE_HOST", Value: "host3"}, InstanceName: "instance3", ServiceName: "mongodb"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	serviceEnv := allEnvs[TsuruServicesEnvVar]
	c.Assert(serviceEnv.Name, check.Equals, TsuruServicesEnvVar)
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
	c.Assert(allEnvs["DATABASE_HOST"], check.DeepEquals, bind.EnvVar{
		Name:   "DATABASE_HOST",
		Value:  "host3",
		Public: false,
	})
}

func (s *S) TestAddInstanceAndRemoveInstanceMultipleServices(c *check.C) {
	a := &App{Name: "fuchsia", TeamOwner: s.team.Name}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bind.ServiceEnvVar{
			{EnvVar: bind.EnvVar{Name: "DATABASE_HOST", Value: "host1"}, InstanceName: "instance1", ServiceName: "mysql"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bind.ServiceEnvVar{
			{EnvVar: bind.EnvVar{Name: "DATABASE_HOST", Value: "host2"}, InstanceName: "instance2", ServiceName: "mysql"},
		},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	c.Assert(allEnvs["DATABASE_HOST"], check.DeepEquals, bind.EnvVar{
		Name:   "DATABASE_HOST",
		Value:  "host2",
		Public: false,
	})
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(allEnvs[TsuruServicesEnvVar].Value), &serviceEnvVal)
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
	c.Assert(allEnvs["DATABASE_HOST"], check.DeepEquals, bind.EnvVar{
		Name:   "DATABASE_HOST",
		Value:  "host1",
		Public: false,
	})
	err = json.Unmarshal([]byte(allEnvs[TsuruServicesEnvVar].Value), &serviceEnvVal)
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
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bind.ServiceEnvVar{
			{EnvVar: bind.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, InstanceName: "mydb", ServiceName: "mysql"},
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
	c.Assert(allEnvs["DATABASE_HOST"], check.DeepEquals, bind.EnvVar{})
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(allEnvs[TsuruServicesEnvVar].Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 0)
}

func (s *S) TestRemoveInstanceShifts(c *check.C) {
	a := &App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	toAdd := []bind.ServiceEnvVar{
		{EnvVar: bind.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, InstanceName: "mydb", ServiceName: "mysql"},
		{EnvVar: bind.EnvVar{Name: "DATABASE_NAME", Value: "yourdb"}, InstanceName: "yourdb", ServiceName: "mysql"},
		{EnvVar: bind.EnvVar{Name: "DATABASE_NAME", Value: "hisdb"}, InstanceName: "hisdb", ServiceName: "mysql"},
		{EnvVar: bind.EnvVar{Name: "DATABASE_NAME", Value: "herdb"}, InstanceName: "herdb", ServiceName: "mysql"},
		{EnvVar: bind.EnvVar{Name: "DATABASE_NAME", Value: "ourdb"}, InstanceName: "ourdb", ServiceName: "mysql"},
	}
	for _, env := range toAdd {
		err = a.AddInstance(bind.AddInstanceArgs{
			Envs:          []bind.ServiceEnvVar{env},
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
	err = json.Unmarshal([]byte(allEnvs[TsuruServicesEnvVar].Value), &serviceEnvVal)
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
	c.Assert(allEnvs["DATABASE_NAME"], check.DeepEquals, bind.EnvVar{
		Name:  "DATABASE_NAME",
		Value: "ourdb",
	})
}

func (s *S) TestRemoveInstanceNotFound(c *check.C) {
	a := &App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs:          []bind.ServiceEnvVar{{EnvVar: bind.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, InstanceName: "mydb", ServiceName: "mysql"}},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = a.RemoveInstance(bind.RemoveInstanceArgs{
		ServiceName:   "mysql",
		InstanceName:  "yourdb",
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(allEnvs[TsuruServicesEnvVar].Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{
		"mysql": []interface{}{
			map[string]interface{}{"instance_name": "mydb", "envs": map[string]interface{}{
				"DATABASE_NAME": "mydb",
			}},
		},
	})
	c.Assert(allEnvs["DATABASE_NAME"], check.DeepEquals, bind.EnvVar{
		Name:  "DATABASE_NAME",
		Value: "mydb",
	})
}

func (s *S) TestRemoveInstanceServiceNotFound(c *check.C) {
	a := &App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs:          []bind.ServiceEnvVar{{EnvVar: bind.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, InstanceName: "mydb", ServiceName: "mysql"}},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = a.RemoveInstance(bind.RemoveInstanceArgs{
		ServiceName:   "mongodb",
		InstanceName:  "mydb",
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(allEnvs[TsuruServicesEnvVar].Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{
		"mysql": []interface{}{
			map[string]interface{}{"instance_name": "mydb", "envs": map[string]interface{}{
				"DATABASE_NAME": "mydb",
			}},
		},
	})
	c.Assert(allEnvs["DATABASE_NAME"], check.DeepEquals, bind.EnvVar{
		Name:  "DATABASE_NAME",
		Value: "mydb",
	})
}

func (s *S) TestRemoveInstanceWithUnits(c *check.C) {
	a := &App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", nil)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs:          []bind.ServiceEnvVar{{EnvVar: bind.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, InstanceName: "mydb", ServiceName: "mysql"}},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = a.RemoveInstance(bind.RemoveInstanceArgs{
		ServiceName:   "mysql",
		InstanceName:  "mydb",
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(allEnvs[TsuruServicesEnvVar].Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{})
	c.Assert(allEnvs["DATABASE_NAME"], check.DeepEquals, bind.EnvVar{})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 1)
}

func (s *S) TestRemoveInstanceWithUnitsNoRestart(c *check.C) {
	a := &App{Name: "dark", TeamOwner: s.team.Name}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", nil)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs:          []bind.ServiceEnvVar{{EnvVar: bind.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, InstanceName: "mydb", ServiceName: "mysql"}},
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	err = a.RemoveInstance(bind.RemoveInstanceArgs{
		ServiceName:   "mysql",
		InstanceName:  "mydb",
		ShouldRestart: false,
	})
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	var serviceEnvVal map[string]interface{}
	err = json.Unmarshal([]byte(allEnvs[TsuruServicesEnvVar].Value), &serviceEnvVal)
	c.Assert(err, check.IsNil)
	c.Assert(serviceEnvVal, check.DeepEquals, map[string]interface{}{})
	c.Assert(allEnvs["DATABASE_NAME"], check.DeepEquals, bind.EnvVar{})
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
	plan1 := appTypes.Plan{Name: "plan1", Memory: 1, CpuShare: 1}
	plan2 := appTypes.Plan{Name: "plan2", Memory: 1, CpuShare: 1}
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
		{InternalAppName, s.team.Name, "pool1", "fake", "default-plan", errMsg},
		{"myapp", "invalidteam", "pool1", "fake", "default-plan", "team not found"},
		{"myapp", s.team.Name, "pool1", "faketls", "default-plan", "router \"faketls\" is not available for pool \"pool1\". Available routers are: \"fake, fake-hc, fake-tls\""},
		{"myapp", "noaccessteam", "pool1", "fake", "default-plan", "App team owner \"noaccessteam\" has no access to pool \"pool1\""},
		{"myApp", s.team.Name, "pool1", "fake", "default-plan", errMsg},
		{"myapp", s.team.Name, "pool1", "fake", "plan1", "App plan \"plan1\" is not allowed on pool \"pool1\""},
		{"myapp", s.team.Name, "pool1", "fake", "plan2", ""},
	}
	for _, d := range data {
		a := App{Name: d.name, Plan: appTypes.Plan{Name: d.plan}, TeamOwner: d.teamOwner, Pool: d.pool, Routers: []appTypes.AppRouter{{Name: d.router}}}
		if valid := a.validateNew(); valid != nil && valid.Error() != d.expected {
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	var b bytes.Buffer
	err = a.Restart("", &b)
	c.Assert(err, check.IsNil)
	c.Assert(b.String(), check.Matches, `(?s).*---- Restarting the app "someapp" ----.*`)
	restarts := s.provisioner.Restarts(&a, "")
	c.Assert(restarts, check.Equals, 1)
}

func (s *S) TestStop(c *check.C) {
	a := App{Name: "app", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	err = a.Stop(&buf, "")
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Find(bson.M{"name": a.GetName()}).One(&a)
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	for _, u := range units {
		c.Assert(u.Status, check.Equals, provision.StatusStopped)
	}
}

func (s *S) TestSleep(c *check.C) {
	a := App{
		Name:      "someapp",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Routers:   []appTypes.AppRouter{{Name: "fake"}},
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.AddBackend(&a)
	var b bytes.Buffer
	err = a.Start(&b, "")
	c.Assert(err, check.IsNil)
	proxyURL, err := url.Parse("http://example.com")
	c.Assert(err, check.IsNil)
	err = a.Sleep(&b, "", proxyURL)
	c.Assert(err, check.IsNil)
	sleeps := s.provisioner.Sleeps(&a, "")
	c.Assert(sleeps, check.Equals, 1)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, proxyURL.String()), check.Equals, true)
	routes, err := routertest.FakeRouter.Routes(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 1)
}

func (s *S) TestLog(c *check.C) {
	a := App{Name: "new-app", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer func() {
		s.logConn.Logs(a.Name).DropCollection()
	}()
	err = a.Log("last log msg", "tsuru", "outermachine")
	c.Assert(err, check.IsNil)
	var logs []Applog
	err = s.logConn.Logs(a.Name).Find(nil).All(&logs)
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 1)
	c.Assert(logs[0].Message, check.Equals, "last log msg")
	c.Assert(logs[0].Source, check.Equals, "tsuru")
	c.Assert(logs[0].AppName, check.Equals, a.Name)
	c.Assert(logs[0].Unit, check.Equals, "outermachine")
}

func (s *S) TestLogShouldAddOneRecordByLine(c *check.C) {
	a := App{Name: "new-app", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	defer func() {
		s.logConn.Logs(a.Name).DropCollection()
	}()
	err = a.Log("last log msg\nfirst log", "source", "machine")
	c.Assert(err, check.IsNil)
	var logs []Applog
	err = s.logConn.Logs(a.Name).Find(nil).Sort("$natural").All(&logs)
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 2)
	c.Assert(logs[0].Message, check.Equals, "last log msg")
	c.Assert(logs[1].Message, check.Equals, "first log")
}

func (s *S) TestLogShouldNotLogBlankLines(c *check.C) {
	a := App{Name: "ich", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.Log("some message", "tsuru", "machine")
	c.Assert(err, check.IsNil)
	err = a.Log("", "", "")
	c.Assert(err, check.IsNil)
	count, err := s.logConn.Logs(a.Name).Find(nil).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 1)
}

func (s *S) TestLogWithListeners(c *check.C) {
	var logs struct {
		l []Applog
		sync.Mutex
	}
	a := App{
		Name:      "new-app",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	l, err := NewLogListener(&a, Applog{})
	c.Assert(err, check.IsNil)
	defer l.Close()
	go func() {
		for log := range l.c {
			logs.Lock()
			logs.l = append(logs.l, log)
			logs.Unlock()
		}
	}()
	err = a.Log("last log msg", "tsuru", "machine")
	c.Assert(err, check.IsNil)
	defer s.logConn.Logs(a.Name).DropCollection()
	done := make(chan bool, 1)
	q := make(chan bool)
	go func(quit chan bool) {
		for range time.Tick(1e3) {
			select {
			case <-quit:
				return
			default:
			}
			logs.Lock()
			if len(logs.l) == 1 {
				logs.Unlock()
				done <- true
				return
			}
			logs.Unlock()
		}
	}(q)
	select {
	case <-done:
	case <-time.After(2e9):
		defer close(q)
		c.Fatal("Timed out.")
	}
	logs.Lock()
	c.Assert(logs.l, check.HasLen, 1)
	log := logs.l[0]
	logs.Unlock()
	c.Assert(log.Message, check.Equals, "last log msg")
	c.Assert(log.Source, check.Equals, "tsuru")
	c.Assert(log.Unit, check.Equals, "machine")
}

func (s *S) TestLastLogs(c *check.C) {
	app := App{
		Name:      "app3",
		Platform:  "vougan",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	for i := 0; i < 15; i++ {
		app.Log(strconv.Itoa(i), "tsuru", "rdaneel")
		time.Sleep(1e6) // let the time flow
	}
	app.Log("app3 log from circus", "circus", "rdaneel")
	logs, err := app.LastLogs(10, Applog{Source: "tsuru"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 10)
	for i := 5; i < 15; i++ {
		c.Check(logs[i-5].Message, check.Equals, strconv.Itoa(i))
		c.Check(logs[i-5].Source, check.Equals, "tsuru")
	}
}

func (s *S) TestLastLogsUnitFilter(c *check.C) {
	app := App{
		Name:      "app3",
		Platform:  "vougan",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	for i := 0; i < 15; i++ {
		app.Log(strconv.Itoa(i), "tsuru", "rdaneel")
		time.Sleep(1e6) // let the time flow
	}
	app.Log("app3 log from circus", "circus", "rdaneel")
	app.Log("app3 log from tsuru", "tsuru", "seldon")
	logs, err := app.LastLogs(10, Applog{Source: "tsuru", Unit: "rdaneel"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 10)
	for i := 5; i < 15; i++ {
		c.Check(logs[i-5].Message, check.Equals, strconv.Itoa(i))
		c.Check(logs[i-5].Source, check.Equals, "tsuru")
	}
}

func (s *S) TestLastLogsEmpty(c *check.C) {
	app := App{
		Name:      "app33",
		Platform:  "vougan",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	logs, err := app.LastLogs(10, Applog{Source: "tsuru"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.DeepEquals, []Applog{})
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
	_, err = app.LastLogs(10, Applog{})
	c.Assert(err, check.ErrorMatches, "my doc msg")
}

func (s *S) TestGetTeams(c *check.C) {
	app := App{Name: "app", Teams: []string{s.team.Name}}
	teams := app.GetTeams()
	c.Assert(teams, check.HasLen, 1)
	c.Assert(teams[0].Name, check.Equals, s.team.Name)
}

func (s *S) TestGetUnits(c *check.C) {
	app := App{Name: "app", TeamOwner: s.team.Name}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&app, 2, "web", nil)
	bindUnits, err := app.GetUnits()
	c.Assert(err, check.IsNil)
	c.Assert(bindUnits, check.HasLen, 2)
	units, err := app.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units[0].IP, check.Equals, bindUnits[0].GetIp())
	c.Assert(units[1].IP, check.Equals, bindUnits[1].GetIp())
}

func (s *S) TestAppMarshalJSON(c *check.C) {
	repository.Manager().CreateRepository("name", nil)
	opts := pool.AddPoolOptions{Name: "test", Default: false}
	err := pool.AddPool(opts)
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
		Plan:        appTypes.Plan{Name: "myplan", Memory: 64, Swap: 128, CpuShare: 100},
		TeamOwner:   "myteam",
		Routers:     []appTypes.AppRouter{{Name: "fake", Opts: map[string]string{"opt1": "val1"}}},
		Tags:        []string{"tag a", "tag b"},
	}
	err = routertest.FakeRouter.AddBackend(&app)
	c.Assert(err, check.IsNil)
	expected := map[string]interface{}{
		"name":        "name",
		"platform":    "Framework",
		"repository":  "git@" + repositorytest.ServerHost + ":name.git",
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
			"swap":     float64(128),
			"cpushare": float64(100),
			"router":   "fake",
		},
		"router":     "fake",
		"routeropts": map[string]interface{}{"opt1": "val1"},
		"routers": []interface{}{
			map[string]interface{}{
				"name":    "fake",
				"address": "name.fakerouter.com",
				"type":    "fake",
				"opts":    map[string]interface{}{"opt1": "val1"},
			},
		},
		"tags": []interface{}{"tag a", "tag b"},
	}
	data, err := app.MarshalJSON()
	c.Assert(err, check.IsNil)
	result := make(map[string]interface{})
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
}

func (s *S) TestAppMarshalJSONWithoutRepository(c *check.C) {
	app := App{
		Name:        "name",
		Platform:    "Framework",
		Teams:       []string{"team1"},
		CName:       []string{"name.mycompany.com"},
		Owner:       "appOwner",
		Deploys:     7,
		Pool:        "pool1",
		Description: "description",
		Plan:        appTypes.Plan{Name: "myplan", Memory: 64, Swap: 128, CpuShare: 100},
		TeamOwner:   "myteam",
		Routers:     []appTypes.AppRouter{{Name: "fake", Opts: map[string]string{}}},
		Tags:        []string{},
	}
	err := routertest.FakeRouter.AddBackend(&app)
	c.Assert(err, check.IsNil)
	expected := map[string]interface{}{
		"name":        "name",
		"platform":    "Framework",
		"repository":  "",
		"teams":       []interface{}{"team1"},
		"units":       []interface{}{},
		"ip":          "name.fakerouter.com",
		"cname":       []interface{}{"name.mycompany.com"},
		"owner":       "appOwner",
		"deploys":     float64(7),
		"pool":        "pool1",
		"description": "description",
		"teamowner":   "myteam",
		"lock":        s.zeroLock,
		"plan": map[string]interface{}{
			"name":     "myplan",
			"memory":   float64(64),
			"swap":     float64(128),
			"cpushare": float64(100),
			"router":   "fake",
		},
		"router":     "fake",
		"routeropts": map[string]interface{}{},
		"routers": []interface{}{
			map[string]interface{}{
				"name":    "fake",
				"address": "name.fakerouter.com",
				"type":    "fake",
				"opts":    map[string]interface{}{},
			},
		},
		"tags": []interface{}{},
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
	}
	err := routertest.FakeRouter.AddBackend(&app)
	c.Assert(err, check.IsNil)
	expected := map[string]interface{}{
		"name":        "name",
		"platform":    "",
		"repository":  "",
		"teams":       nil,
		"units":       []interface{}{},
		"error":       "unable to list app units: my err",
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
			"swap":     float64(0),
			"cpushare": float64(0),
			"router":   "fake",
		},
		"router":     "fake",
		"routeropts": map[string]interface{}{},
		"routers": []interface{}{
			map[string]interface{}{
				"name":    "fake",
				"address": "name.fakerouter.com",
				"type":    "fake",
				"opts":    map[string]interface{}{},
			},
		},
		"tags": nil,
	}
	data, err := app.MarshalJSON()
	c.Assert(err, check.IsNil)
	result := make(map[string]interface{})
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
}

func (s *S) TestAppMarshalJSONPlatformLocked(c *check.C) {
	repository.Manager().CreateRepository("name", nil)
	opts := pool.AddPoolOptions{Name: "test", Default: false}
	err := pool.AddPool(opts)
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
		Plan:            appTypes.Plan{Name: "myplan", Memory: 64, Swap: 128, CpuShare: 100},
		TeamOwner:       "myteam",
		Routers:         []appTypes.AppRouter{{Name: "fake", Opts: map[string]string{"opt1": "val1"}}},
		Tags:            []string{"tag a", "tag b"},
	}
	err = routertest.FakeRouter.AddBackend(&app)
	c.Assert(err, check.IsNil)
	expected := map[string]interface{}{
		"name":        "name",
		"platform":    "Framework:v1",
		"repository":  "git@" + repositorytest.ServerHost + ":name.git",
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
			"swap":     float64(128),
			"cpushare": float64(100),
			"router":   "fake",
		},
		"router":     "fake",
		"routeropts": map[string]interface{}{"opt1": "val1"},
		"routers": []interface{}{
			map[string]interface{}{
				"name":    "fake",
				"address": "name.fakerouter.com",
				"type":    "fake",
				"opts":    map[string]interface{}{"opt1": "val1"},
			},
		},
		"tags": []interface{}{"tag a", "tag b"},
	}
	data, err := app.MarshalJSON()
	c.Assert(err, check.IsNil)
	result := make(map[string]interface{})
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
}

func (s *S) TestRun(c *check.C) {
	s.provisioner.PrepareOutput([]byte("a lot of files"))
	s.provisioner.PrepareOutput([]byte("a lot of files"))
	app := App{Name: "myapp", TeamOwner: s.team.Name}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&app, 2, "web", nil)
	var buf bytes.Buffer
	args := provision.RunArgs{Once: false, Isolated: false}
	err = app.Run("ls -lh", &buf, args)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "a lot of filesa lot of files")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls -lh"
	units, err := app.GetUnits()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 2)
	allExecs := s.provisioner.AllExecs()
	c.Assert(allExecs, check.HasLen, 2)
	c.Assert(allExecs[units[0].GetID()], check.HasLen, 1)
	c.Assert(allExecs[units[0].GetID()][0].Cmds, check.DeepEquals, []string{"/bin/sh", "-c", expected})
	c.Assert(allExecs[units[1].GetID()], check.HasLen, 1)
	c.Assert(allExecs[units[1].GetID()][0].Cmds, check.DeepEquals, []string{"/bin/sh", "-c", expected})
	var logs []Applog
	timeout := time.After(5 * time.Second)
	for {
		logs, err = app.LastLogs(10, Applog{})
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
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&app, 2, "web", nil)
	var buf bytes.Buffer
	args := provision.RunArgs{Once: true, Isolated: false}
	err = app.Run("ls -lh", &buf, args)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "a lot of files")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls -lh"
	units, err := app.GetUnits()
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
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&app, 1, "web", nil)
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
	err := CreateApp(&app, s.user)
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
	err := CreateApp(&app, s.user)
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
		Env: map[string]bind.EnvVar{
			"http_proxy": {
				Name:   "http_proxy",
				Value:  "http://theirproxy.com:3128/",
				Public: true,
			},
		},
	}
	expected := map[string]bind.EnvVar{
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

func (s *S) TestEnvsWithServiceEnvConflict(c *check.C) {
	app := App{
		Name: "time",
		Env: map[string]bind.EnvVar{
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
		ServiceEnvs: []bind.ServiceEnvVar{
			{
				EnvVar: bind.EnvVar{
					Name:  "DB_HOST",
					Value: "host1",
				},
				ServiceName:  "srv1",
				InstanceName: "inst1",
			},
			{
				EnvVar: bind.EnvVar{
					Name:  "DB_HOST",
					Value: "host2",
				},
				ServiceName:  "srv1",
				InstanceName: "inst2",
			},
		},
	}
	expected := map[string]bind.EnvVar{
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
	serviceEnvsRaw := env[TsuruServicesEnvVar]
	delete(env, TsuruServicesEnvVar)
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(&a2, s.user)
	c.Assert(err, check.IsNil)
	apps, err := List(nil)
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(&a2, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(&a3, s.user)
	c.Assert(err, check.IsNil)
	apps, err := List(&Filter{NameMatches: "app\\d{1}"})
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(&a2, s.user)
	c.Assert(err, check.IsNil)
	apps, err := List(&Filter{Platform: "ruby"})
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
		apps, err := List(&Filter{Platform: t.platform})
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
	apps, err := List(&Filter{TeamOwner: "foo"})
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
	apps, err := List(&Filter{UserOwner: "foo"})
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(&a2, s.user)
	c.Assert(err, check.IsNil)
	apps, err := List(&Filter{Locked: true})
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
	apps, err := List(nil)
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(&a2, s.user)
	c.Assert(err, check.IsNil)
	apps, err := List(nil)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 2)
	delete(apps[0].Env, "TSURU_APP_TOKEN")
	delete(apps[1].Env, "TSURU_APP_TOKEN")
	sort.Slice(apps, func(i, j int) bool {
		return apps[i].Name < apps[j].Name
	})
	c.Assert(apps, check.DeepEquals, []App{
		{
			Name:      "app1",
			CName:     []string{},
			Teams:     []string{"tsuruteam"},
			TeamOwner: "tsuruteam",
			Owner:     "whydidifall@thewho.com",
			Env: map[string]bind.EnvVar{
				"TSURU_APPNAME": {Name: "TSURU_APPNAME", Value: "app1"},
				"TSURU_APPDIR":  {Name: "TSURU_APPDIR", Value: "/home/application/current"},
			},
			ServiceEnvs: []bind.ServiceEnvVar{},
			Plan: appTypes.Plan{
				Name:     "default-plan",
				Memory:   1024,
				Swap:     1024,
				CpuShare: 100,
				Default:  true,
			},
			Pool:       "pool1",
			RouterOpts: map[string]string{},
			Tags:       []string{},
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
			Env: map[string]bind.EnvVar{
				"TSURU_APPNAME": {Name: "TSURU_APPNAME", Value: "app2"},
				"TSURU_APPDIR":  {Name: "TSURU_APPDIR", Value: "/home/application/current"},
			},
			ServiceEnvs: []bind.ServiceEnvVar{},
			Plan: appTypes.Plan{
				Name:     "default-plan",
				Memory:   1024,
				Swap:     1024,
				CpuShare: 100,
				Default:  true,
			},
			Pool:       "pool1",
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
		apps, err = List(nil)
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
	err = routertest.FakeRouter.AddBackend(&a)
	c.Assert(err, check.IsNil)
	apps, err := List(nil)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps, check.DeepEquals, []App{
		{
			Name:        "app1",
			CName:       []string{},
			Teams:       []string{"tsuruteam"},
			TeamOwner:   "tsuruteam",
			Env:         map[string]bind.EnvVar{},
			ServiceEnvs: []bind.ServiceEnvVar{},
			Router:      "fake",
			RouterOpts:  map[string]string{},
			Tags:        []string{},
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
		apps, err = List(nil)
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(&a2, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(&a3, s.user)
	c.Assert(err, check.IsNil)
	apps, err := List(&Filter{NameMatches: `app\d{1}`})
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(&a2, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(&a3, s.user)
	c.Assert(err, check.IsNil)
	apps, err := List(&Filter{Name: "app1", NameMatches: `app\d{1}`})
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = CreateApp(&a2, s.user)
	c.Assert(err, check.IsNil)
	apps, err := List(&Filter{Platform: "ruby"})
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
	apps, err := List(&Filter{UserOwner: "foo"})
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
	apps, err := List(&Filter{TeamOwner: "foo"})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
}

func (s *S) TestListFilteringByPool(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test2", Default: false}
	err := pool.AddPool(opts)
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
	apps, err := List(&Filter{Pool: s.Pool})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].GetName(), check.Equals, a2.Name)
	c.Assert(apps[0].GetPool(), check.Equals, a2.Pool)
}

func (s *S) TestListFilteringByPools(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test2", Default: false}
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "test3", Default: false}
	err = pool.AddPool(opts)
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
	apps, err := List(&Filter{Pools: []string{s.Pool, "test2"}})
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
		err := CreateApp(&a, s.user)
		c.Assert(err, check.IsNil)
		err = a.AddUnits(1, "", nil)
		c.Assert(err, check.IsNil)
		apps = append(apps, &a)
	}
	var buf bytes.Buffer
	err := apps[1].Stop(&buf, "")
	c.Assert(err, check.IsNil)
	proxyURL, _ := url.Parse("http://somewhere.com")
	err = apps[2].Sleep(&buf, "", proxyURL)
	c.Assert(err, check.IsNil)
	resultApps, err := List(&Filter{Statuses: []string{"stopped", "asleep"}})
	c.Assert(err, check.IsNil)
	c.Assert(resultApps, check.HasLen, 2)
	names := []string{resultApps[0].Name, resultApps[1].Name}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"ta2", "ta3"})
}

func (s *S) TestListFilteringByTag(c *check.C) {
	app1 := App{Name: "app1", TeamOwner: s.team.Name, Tags: []string{"tag 1"}}
	err := CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := App{Name: "app2", TeamOwner: s.team.Name, Tags: []string{"tag 1", "tag 2", "tag 3"}}
	err = CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	app3 := App{Name: "app3", TeamOwner: s.team.Name, Tags: []string{"tag 4"}}
	err = CreateApp(&app3, s.user)
	c.Assert(err, check.IsNil)
	resultApps, err := List(&Filter{Tags: []string{" tag 1  ", "tag 1"}})
	c.Assert(err, check.IsNil)
	c.Assert(resultApps, check.HasLen, 2)
	c.Assert(resultApps[0].Name, check.Equals, app1.Name)
	c.Assert(resultApps[1].Name, check.Equals, app2.Name)
	resultApps, err = List(&Filter{Tags: []string{"tag 3 ", "tag 1", ""}})
	c.Assert(err, check.IsNil)
	c.Assert(resultApps, check.HasLen, 1)
	c.Assert(resultApps[0].Name, check.Equals, app2.Name)
	resultApps, err = List(&Filter{Tags: []string{"tag 1 ", "   ", " tag 4"}})
	c.Assert(err, check.IsNil)
	c.Assert(resultApps, check.HasLen, 0)
}

func (s *S) TestListReturnsEmptyAppArrayWhenUserHasNoAccessToAnyApp(c *check.C) {
	apps, err := List(nil)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.DeepEquals, []App{})
}

func (s *S) TestListReturnsAllAppsWhenUsedWithNoFilters(c *check.C) {
	a := App{Name: "testApp", Teams: []string{"notAdmin", "noSuperUser"}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	apps, err := List(nil)
	c.Assert(err, check.IsNil)
	c.Assert(len(apps), Greater, 0)
	c.Assert(apps[0].GetName(), check.Equals, "testApp")
	c.Assert(apps[0].GetTeamsName(), check.DeepEquals, []string{"notAdmin", "noSuperUser"})
}

func (s *S) TestListFilteringExtraWithOr(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test2", Default: false}
	err := pool.AddPool(opts)
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
	apps, err := List(f)
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
	a := App{Name: "app1", Quota: quota.UnlimitedQuota}
	c.Assert(a.GetQuota(), check.DeepEquals, quota.UnlimitedQuota)
}

func (s *S) TestSetQuotaInUse(c *check.C) {
	app := App{Name: "someapp", Quota: quota.Quota{Limit: 5, InUse: 5}}
	s.mockService.AppQuota.OnSet = func(appName string, inUse int) error {
		c.Assert(appName, check.Equals, app.Name)
		c.Assert(inUse, check.Equals, 3)
		return nil
	}
	err := app.SetQuotaInUse(3)
	c.Assert(err, check.IsNil)
}

func (s *S) TestSetQuotaInUseNotFound(c *check.C) {
	app := App{Name: "someapp", Quota: quota.Quota{Limit: 5, InUse: 5}}
	s.mockService.AppQuota.OnSet = func(appName string, inUse int) error {
		c.Assert(appName, check.Equals, app.Name)
		c.Assert(inUse, check.Equals, 3)
		return appTypes.ErrAppNotFound
	}
	err := app.SetQuotaInUse(3)
	c.Assert(err, check.Equals, appTypes.ErrAppNotFound)
}

func (s *S) TestSetQuotaInUseUnlimited(c *check.C) {
	app := App{Name: "someapp", Quota: quota.UnlimitedQuota, TeamOwner: s.team.Name}
	s.mockService.AppQuota.OnSet = func(appName string, inUse int) error {
		c.Assert(appName, check.Equals, app.Name)
		c.Assert(inUse, check.Equals, 3)
		return nil
	}
	err := app.SetQuotaInUse(3)
	c.Assert(err, check.IsNil)

}

func (s *S) TestSetQuotaInUseQuotaExceeded(c *check.C) {
	app := App{Name: "someapp", Quota: quota.Quota{Limit: 5, InUse: 3}}
	s.mockService.AppQuota.OnSet = func(appName string, inUse int) error {
		c.Assert(appName, check.Equals, app.Name)
		c.Assert(inUse, check.Equals, 6)
		return &quota.QuotaExceededError{Available: 5, Requested: 6}
	}
	err := app.SetQuotaInUse(6)
	c.Assert(err, check.NotNil)
	e, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Available, check.Equals, uint(5))
	c.Assert(e.Requested, check.Equals, uint(6))
}

func (s *S) TestSetQuotaInUseIsInvalid(c *check.C) {
	app := App{Name: "someapp", Quota: quota.Quota{Limit: 5, InUse: 3}}
	s.mockService.AppQuota.OnSet = func(appName string, inUse int) error {
		c.Assert(appName, check.Equals, app.Name)
		c.Assert(inUse, check.Equals, -1)
		return quota.ErrLessThanZero
	}
	err := app.SetQuotaInUse(-1)
	c.Assert(err, check.NotNil)
	c.Check(err, check.Equals, quota.ErrLessThanZero)
}

func (s *S) TestGetCname(c *check.C) {
	a := App{CName: []string{"cname1", "cname2"}}
	c.Assert(a.GetCname(), check.DeepEquals, a.CName)
}

func (s *S) TestGetLock(c *check.C) {
	a := App{
		Lock: appTypes.AppLock{
			Locked:      true,
			Owner:       "someone",
			Reason:      "/app/my-app/deploy",
			AcquireDate: time.Date(2048, time.November, 10, 10, 0, 0, 0, time.UTC),
		},
	}
	c.Assert(a.GetLock().GetLocked(), check.Equals, a.Lock.Locked)
	c.Assert(a.GetLock().GetOwner(), check.Equals, a.Lock.Owner)
	c.Assert(a.GetLock().GetReason(), check.Equals, a.Lock.Reason)
	c.Assert(a.GetLock().GetAcquireDate(), check.Equals, a.Lock.AcquireDate)
}

func (s *S) TestGetPlatform(c *check.C) {
	a := App{Platform: "django"}
	c.Assert(a.GetPlatform(), check.Equals, a.Platform)
}

func (s *S) TestGetDeploys(c *check.C) {
	a := App{Deploys: 3}
	c.Assert(a.GetDeploys(), check.Equals, a.Deploys)
}

func (s *S) TestGetMemory(c *check.C) {
	a := App{Plan: appTypes.Plan{Memory: 10}}
	c.Assert(a.GetMemory(), check.Equals, a.Plan.Memory)
}

func (s *S) TestGetSwap(c *check.C) {
	a := App{Plan: appTypes.Plan{Swap: 20}}
	c.Assert(a.GetSwap(), check.Equals, a.Plan.Swap)
}

func (s *S) TestAppUnits(c *check.C) {
	a := App{Name: "anycolor", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
}

func (s *S) TestAppAvailable(c *check.C) {
	a := App{
		Name:      "anycolor",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	c.Assert(a.available(), check.Equals, true)
	s.provisioner.Stop(&a, "")
	c.Assert(a.available(), check.Equals, false)
}

func (s *S) TestSwap(c *check.C) {
	app1 := &App{Name: "app1", CName: []string{"cname"}, TeamOwner: s.team.Name}
	err := CreateApp(app1, s.user)
	c.Assert(err, check.IsNil)
	s.mockService.Cache.OnCreate = func(entry cache.CacheEntry) error {
		if entry.Value != "app1.fakerouter.com" && entry.Value != "app2.fakerouter.com" {
			c.Errorf("unexpected cache entry: %v", entry)
		}
		return nil
	}
	oldAddrs1, err := app1.GetAddresses()
	c.Assert(err, check.IsNil)
	app2 := &App{Name: "app2", TeamOwner: s.team.Name}
	err = CreateApp(app2, s.user)
	c.Assert(err, check.IsNil)
	oldAddrs2, err := app2.GetAddresses()
	c.Assert(err, check.IsNil)
	err = Swap(app1, app2, false)
	c.Assert(err, check.IsNil)
	newAddrs1, err := app1.GetAddresses()
	c.Assert(err, check.IsNil)
	newAddrs2, err := app2.GetAddresses()
	c.Assert(err, check.IsNil)
	c.Assert(app1.CName, check.IsNil)
	c.Assert(app2.CName, check.DeepEquals, []string{"cname"})
	c.Assert(newAddrs1, check.DeepEquals, oldAddrs2)
	c.Assert(newAddrs2, check.DeepEquals, oldAddrs1)
}

func (s *S) TestSwapCnameOnly(c *check.C) {
	app1 := &App{Name: "app1", CName: []string{"app1.cname", "app1.cname2"}, TeamOwner: s.team.Name}
	err := CreateApp(app1, s.user)
	c.Assert(err, check.IsNil)
	oldAddrs1, err := app1.GetAddresses()
	c.Assert(err, check.IsNil)
	app2 := &App{Name: "app2", CName: []string{"app2.cname"}, TeamOwner: s.team.Name}
	err = CreateApp(app2, s.user)
	c.Assert(err, check.IsNil)
	oldAddrs2, err := app2.GetAddresses()
	c.Assert(err, check.IsNil)
	err = Swap(app1, app2, true)
	c.Assert(err, check.IsNil)
	newAddrs1, err := app1.GetAddresses()
	c.Assert(err, check.IsNil)
	newAddrs2, err := app2.GetAddresses()
	c.Assert(err, check.IsNil)
	c.Assert(app1.CName, check.DeepEquals, []string{"app2.cname"})
	c.Assert(app2.CName, check.DeepEquals, []string{"app1.cname", "app1.cname2"})
	c.Assert(newAddrs1, check.DeepEquals, oldAddrs1)
	c.Assert(newAddrs2, check.DeepEquals, oldAddrs2)
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	var b bytes.Buffer
	err = a.Start(&b, "")
	c.Assert(err, check.IsNil)
	starts := s.provisioner.Starts(&a, "")
	c.Assert(starts, check.Equals, 1)
}

func (s *S) TestStartAsleepApp(c *check.C) {
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake"}}}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	var b bytes.Buffer
	err = a.Sleep(&b, "web", &url.URL{Scheme: "http", Host: "proxy:1234"})
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	for _, u := range units {
		c.Assert(u.Status, check.Not(check.Equals), provision.StatusStarted)
	}
	err = a.Start(&b, "web")
	c.Assert(err, check.IsNil)
	routes, err := routertest.FakeRouter.Routes(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 1)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, "http://proxy:1234"), check.Equals, false)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[0].Address.String()), check.Equals, true)
}

func (s *S) TestRestartAsleepApp(c *check.C) {
	a := App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake"}}, TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	var b bytes.Buffer
	err = a.Sleep(&b, "web", &url.URL{Scheme: "http", Host: "proxy:1234"})
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	for _, u := range units {
		c.Assert(u.Status, check.Not(check.Equals), provision.StatusStarted)
	}
	err = a.Restart("web", &b)
	c.Assert(err, check.IsNil)
	routes, err := routertest.FakeRouter.Routes(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 1)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, "http://proxy:1234"), check.Equals, false)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[0].Address.String()), check.Equals, true)
}

func (s *S) TestAppSetUpdatePlatform(c *check.C) {
	a := App{
		Name:      "someapp",
		Platform:  "django",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	a.SetUpdatePlatform(true)
	app, err := GetByName("someapp")
	c.Assert(err, check.IsNil)
	c.Assert(app.UpdatePlatform, check.Equals, true)
}

func (s *S) TestAppAcquireApplicationLock(c *check.C) {
	a := App{
		Name:      "someapp",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	locked, err := AcquireApplicationLock(a.Name, "foo", "/something")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
	app, err := GetByName("someapp")
	c.Assert(err, check.IsNil)
	c.Assert(app.Lock.Locked, check.Equals, true)
	c.Assert(app.Lock.Owner, check.Equals, "foo")
	c.Assert(app.Lock.Reason, check.Equals, "/something")
	c.Assert(app.Lock.AcquireDate, check.NotNil)
}

func (s *S) TestAppAcquireApplicationLockNonExistentApp(c *check.C) {
	locked, err := AcquireApplicationLock("myApp", "foo", "/something")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, false)
}

func (s *S) TestAppAcquireApplicationLockAlreadyLocked(c *check.C) {
	a := App{
		Name: "someapp",
		Lock: appTypes.AppLock{
			Locked:      true,
			Reason:      "/app/my-app/deploy",
			Owner:       "someone",
			AcquireDate: time.Date(2048, time.November, 10, 10, 0, 0, 0, time.UTC),
		},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	locked, err := AcquireApplicationLock(a.Name, "foo", "/something")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, false)
	app, err := GetByName("someapp")
	c.Assert(err, check.IsNil)
	c.Assert(app.Lock.Locked, check.Equals, true)
	c.Assert(app.Lock.Owner, check.Equals, "someone")
	c.Assert(app.Lock.Reason, check.Equals, "/app/my-app/deploy")
	c.Assert(app.Lock.AcquireDate, check.NotNil)
}

func (s *S) TestAppAcquireApplicationLockWait(c *check.C) {
	a := App{Name: "test-lock-app", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	locked, err := AcquireApplicationLock(a.Name, "foo", "/something")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
	wg := sync.WaitGroup{}
	wg.Add(1)
	defer wg.Wait()
	go func() {
		defer wg.Done()
		time.Sleep(time.Second)
		ReleaseApplicationLock(a.Name)
	}()
	locked, err = AcquireApplicationLockWait(a.Name, "zzz", "/other", 10*time.Second)
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
	app, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Lock.Locked, check.Equals, true)
	c.Assert(app.Lock.Owner, check.Equals, "zzz")
	c.Assert(app.Lock.Reason, check.Equals, "/other")
	c.Assert(app.Lock.AcquireDate, check.NotNil)
}

func (s *S) TestAppAcquireApplicationLockWaitWithoutRelease(c *check.C) {
	a := App{Name: "test-lock-app", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	locked, err := AcquireApplicationLock(a.Name, "foo", "/something")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
	locked, err = AcquireApplicationLockWait(a.Name, "zzz", "/other", 500*time.Millisecond)
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, false)
	app, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Lock.Locked, check.Equals, true)
	c.Assert(app.Lock.Owner, check.Equals, "foo")
	c.Assert(app.Lock.Reason, check.Equals, "/something")
	c.Assert(app.Lock.AcquireDate, check.NotNil)
}

func (s *S) TestAppAcquireApplicationLockWaitMany(c *check.C) {
	a1 := App{Name: "test-lock-app1", TeamOwner: s.team.Name}
	err := CreateApp(&a1, s.user)
	c.Assert(err, check.IsNil)
	a2 := App{Name: "test-lock-app2", TeamOwner: s.team.Name}
	err = CreateApp(&a2, s.user)
	c.Assert(err, check.IsNil)
	err = AcquireApplicationLockWaitMany([]string{a1.Name, a2.Name}, "foo", "/something", 0)
	c.Assert(err, check.IsNil)
	wg := sync.WaitGroup{}
	wg.Add(1)
	defer wg.Wait()
	go func() {
		defer wg.Done()
		time.Sleep(time.Second)
		ReleaseApplicationLockMany([]string{a1.Name, a2.Name})
	}()
	err = AcquireApplicationLockWaitMany([]string{a1.Name, a2.Name}, "zzz", "/other", 10*time.Second)
	c.Assert(err, check.IsNil)
	app1, err := GetByName(a1.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app1.Lock.Locked, check.Equals, true)
	c.Assert(app1.Lock.Owner, check.Equals, "zzz")
	c.Assert(app1.Lock.Reason, check.Equals, "/other")
	c.Assert(app1.Lock.AcquireDate, check.NotNil)
	app2, err := GetByName(a2.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app2.Lock.Locked, check.Equals, true)
	c.Assert(app2.Lock.Owner, check.Equals, "zzz")
	c.Assert(app2.Lock.Reason, check.Equals, "/other")
	c.Assert(app2.Lock.AcquireDate, check.NotNil)
}

func (s *S) TestAppAcquireApplicationLockWaitManyPartialFailure(c *check.C) {
	a1 := App{Name: "test-lock-app1", TeamOwner: s.team.Name}
	err := CreateApp(&a1, s.user)
	c.Assert(err, check.IsNil)
	a2 := App{Name: "test-lock-app2", TeamOwner: s.team.Name}
	err = CreateApp(&a2, s.user)
	c.Assert(err, check.IsNil)
	locked, err := AcquireApplicationLock(a2.Name, "x", "y")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
	err = AcquireApplicationLockWaitMany([]string{a1.Name, a2.Name}, "zzz", "/other", 0)
	c.Assert(err, check.DeepEquals, appTypes.ErrAppNotLocked{
		App: a2.Name,
	})
	app1, err := GetByName(a1.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app1.Lock.Locked, check.Equals, false)
	app2, err := GetByName(a2.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app2.Lock.Locked, check.Equals, true)
	c.Assert(app2.Lock.Owner, check.Equals, "x")
	c.Assert(app2.Lock.Reason, check.Equals, "y")
	c.Assert(app2.Lock.AcquireDate, check.NotNil)
}

func (s *S) TestAppLockStringUnlocked(c *check.C) {
	lock := appTypes.AppLock{Locked: false}
	c.Assert(lock.String(), check.Equals, "Not locked")
}

func (s *S) TestAppLockStringLocked(c *check.C) {
	lock := appTypes.AppLock{
		Locked:      true,
		Reason:      "/app/my-app/deploy",
		Owner:       "someone",
		AcquireDate: time.Date(2048, time.November, 10, 10, 0, 0, 0, time.UTC),
	}
	c.Assert(lock.String(), check.Matches, "App locked by someone, running /app/my-app/deploy. Acquired in 2048-11-10T.*")
}

func (s *S) TestAppLockMarshalJSON(c *check.C) {
	lock := appTypes.AppLock{
		Locked:      true,
		Reason:      "/app/my-app/deploy",
		Owner:       "someone",
		AcquireDate: time.Date(2048, time.November, 10, 10, 0, 0, 0, time.UTC),
	}
	data, err := lock.MarshalJSON()
	c.Assert(err, check.IsNil)
	var a appTypes.AppLock
	err = json.Unmarshal(data, &a)
	c.Assert(err, check.IsNil)
	c.Assert(a, check.DeepEquals, lock)
}

func (s *S) TestAppLockGetLocked(c *check.C) {
	lock := appTypes.AppLock{Locked: true}
	c.Assert(lock.GetLocked(), check.Equals, lock.Locked)
}

func (s *S) TestAppLockGetReason(c *check.C) {
	lock := appTypes.AppLock{Reason: "/app/my-app/deploy"}
	c.Assert(lock.GetReason(), check.Equals, lock.Reason)
}

func (s *S) TestAppLockGetOwner(c *check.C) {
	lock := appTypes.AppLock{Owner: "someone"}
	c.Assert(lock.GetOwner(), check.Equals, lock.Owner)
}

func (s *S) TestAppLockGetAcquireDate(c *check.C) {
	lock := appTypes.AppLock{AcquireDate: time.Date(2048, time.November, 10, 10, 0, 0, 0, time.UTC)}
	c.Assert(lock.GetAcquireDate(), check.Equals, lock.AcquireDate)
}

func (s *S) TestAppRegisterUnit(c *check.C) {
	a := App{Name: "app-name", Platform: "python", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	var ips []string
	for _, u := range units {
		ips = append(ips, u.IP)
	}
	customData := map[string]interface{}{"x": "y"}
	err = a.RegisterUnit(units[0].ID, customData)
	c.Assert(err, check.IsNil)
	units, err = a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units[0].IP, check.Equals, ips[0]+"-updated")
	c.Assert(units[1].IP, check.Equals, ips[1])
	c.Assert(units[2].IP, check.Equals, ips[2])
	c.Assert(s.provisioner.CustomData(&a), check.DeepEquals, customData)
}

func (s *S) TestAppRegisterUnitDoesBind(c *check.C) {
	var requests []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
	}))
	defer server.Close()
	app := App{
		Name: "warpaint", Platform: "python",
		Quota:     quota.UnlimitedQuota,
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	srvc := service.Service{
		Name:       "mysql",
		Endpoint:   map[string]string{"production": server.URL},
		Password:   "abcde",
		OwnerTeams: []string{s.team.Name},
	}
	err = service.Create(srvc)
	c.Assert(err, check.IsNil)
	si1 := service.ServiceInstance{
		Name:        "mydb",
		ServiceName: "mysql",
		Apps:        []string{app.Name},
	}
	err = s.conn.ServiceInstances().Insert(si1)
	c.Assert(err, check.IsNil)
	si2 := service.ServiceInstance{
		Name:        "yourdb",
		ServiceName: "mysql",
		Apps:        []string{app.Name},
	}
	err = s.conn.ServiceInstances().Insert(si2)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&app, 1, "web", nil)
	units, err := app.Units()
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{"x": "y"}
	err = app.RegisterUnit(units[0].GetID(), customData)
	c.Assert(err, check.IsNil)
	c.Assert(requests, check.HasLen, 2)
	c.Assert(requests[0].Method, check.Equals, "POST")
	c.Assert(requests[0].URL.Path, check.Equals, "/resources/mydb/bind")
	c.Assert(requests[1].Method, check.Equals, "POST")
	c.Assert(requests[1].URL.Path, check.Equals, "/resources/yourdb/bind")
}

func (s *S) TestAppRegisterUnitInvalidUnit(c *check.C) {
	a := App{Name: "app-name", Platform: "python", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.RegisterUnit("oddity", nil)
	c.Assert(err, check.NotNil)
	e, ok := err.(*provision.UnitNotFoundError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.ID, check.Equals, "oddity")
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
}

func (s *S) TestValidateAppService(c *check.C) {
	app := App{Name: "fyrone-flats", Platform: "python", TeamOwner: s.team.Name, Pool: s.Pool}
	err := CreateApp(&app, s.user)
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
	c.Assert(err.Error(), check.Equals, "service \"invalidService\" is not available for pool \"pool1\". Available services are: \"healthcheck\"")
}

func (s *S) TestValidateBlacklistedAppService(c *check.C) {
	app := App{Name: "urgot", Platform: "python", TeamOwner: s.team.Name, Pool: s.Pool}
	err := CreateApp(&app, s.user)
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
	poolConstraint := pool.PoolConstraint{PoolExpr: s.Pool, Field: pool.ConstraintTypeService, Values: []string{serv.Name}, Blacklist: true}
	err = pool.SetPoolConstraint(&poolConstraint)
	c.Assert(err, check.IsNil)
	err = app.ValidateService(serv.Name)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, pool.ErrPoolHasNoService)
	opts := pool.AddPoolOptions{Name: "poolz"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	app2 := App{Name: "nidalee", Platform: "python", TeamOwner: s.team.Name, Pool: "poolz"}
	err = CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	err = app2.ValidateService(serv.Name)
	c.Assert(err, check.IsNil)
}

func (s *S) TestAppCreateValidateTeamOwnerSetAnTeamWhichNotExists(c *check.C) {
	a := App{Name: "test", Platform: "python", TeamOwner: "not-exists"}
	err := CreateApp(&a, s.user)
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.DeepEquals, &errors.ValidationError{
		Message: "router \"fake-tls\" is not available for pool \"pool1\". Available routers are: \"fake, fake-hc\"",
	})
}

func (s *S) TestAppSetPoolByTeamOwner(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test"}
	err := pool.AddPool(opts)
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
	err := pool.AddPool(opts)
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
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("test", []string{"tsuruteam"})
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "pool2"}
	err = pool.AddPool(opts)
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
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("test", []string{"test"})
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "pool2"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("pool2", []string{"test"})
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "pool3", Public: true}
	err = pool.AddPool(opts)
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
	err := pool.AddPool(opts)
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
	err := pool.AddPool(opts)
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
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "nonpublic"}
	err = pool.AddPool(opts)
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
	app, _ := GetByName(a.Name)
	c.Assert("nonpublic", check.Equals, app.Pool)
}

func (s *S) TestShellToUnit(c *check.C) {
	a := App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.PrepareOutput([]byte("output"))
	s.provisioner.AddUnits(&a, 1, "web", nil)
	units, err := s.provisioner.Units(&a)
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
	c.Assert(allExecs[unit.GetID()][0].Cmds, check.DeepEquals, []string{"/bin/sh", "-c", "[ -f /home/application/apprc ] && source /home/application/apprc; [ -d /home/application/current ] && cd /home/application/current; bash -l"})
}

func (s *S) TestShellNoUnits(c *check.C) {
	a := App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
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
	c.Assert(allExecs["isolated"][0].Cmds, check.DeepEquals, []string{"/bin/sh", "-c", "[ -f /home/application/apprc ] && source /home/application/apprc; [ -d /home/application/current ] && cd /home/application/current; bash -l"})
}

func (s *S) TestSetCertificateForApp(c *check.C) {
	cname := "app.io"
	routertest.TLSRouter.SetBackendAddr("my-test-app", cname)
	cert, err := ioutil.ReadFile("testdata/certificate.crt")
	c.Assert(err, check.IsNil)
	key, err := ioutil.ReadFile("testdata/private.key")
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate(cname, string(cert), string(key))
	c.Assert(err, check.IsNil)
	c.Assert(routertest.TLSRouter.Certs[cname], check.Equals, string(cert))
	c.Assert(routertest.TLSRouter.Keys[cname], check.Equals, string(key))
}

func (s *S) TestSetCertificateForAppCName(c *check.C) {
	cname := "app.io"
	cert, err := ioutil.ReadFile("testdata/certificate.crt")
	c.Assert(err, check.IsNil)
	key, err := ioutil.ReadFile("testdata/private.key")
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}, CName: []string{cname}}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate(cname, string(cert), string(key))
	c.Assert(err, check.IsNil)
	c.Assert(routertest.TLSRouter.Certs[cname], check.Equals, string(cert))
	c.Assert(routertest.TLSRouter.Keys[cname], check.Equals, string(key))
}

func (s *S) TestSetCertificateNonTLSRouter(c *check.C) {
	cert, err := ioutil.ReadFile("testdata/certificate.crt")
	c.Assert(err, check.IsNil)
	key, err := ioutil.ReadFile("testdata/private.key")
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, CName: []string{"app.io"}}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate("app.io", string(cert), string(key))
	c.Assert(err, check.ErrorMatches, "no router with tls support")
}

func (s *S) TestSetCertificateInvalidCName(c *check.C) {
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate("example.com", "cert", "key")
	c.Assert(err, check.ErrorMatches, "invalid name")
	c.Assert(routertest.TLSRouter.Certs["example.com"], check.Equals, "")
	c.Assert(routertest.TLSRouter.Keys["example.com"], check.Equals, "")
}

func (s *S) TestSetCertificateInvalidCertificateForCName(c *check.C) {
	cname := "example.io"
	cert, err := ioutil.ReadFile("testdata/certificate.crt")
	c.Assert(err, check.IsNil)
	key, err := ioutil.ReadFile("testdata/private.key")
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}, CName: []string{cname}}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate(cname, string(cert), string(key))
	c.Assert(err, check.ErrorMatches, "*certificate is valid for app.io, not example.io*")
	c.Assert(routertest.TLSRouter.Certs[cname], check.Equals, "")
	c.Assert(routertest.TLSRouter.Keys[cname], check.Equals, "")
}

func (s *S) TestRemoveCertificate(c *check.C) {
	cname := "app.io"
	routertest.TLSRouter.SetBackendAddr("my-test-app", cname)
	cert, err := ioutil.ReadFile("testdata/certificate.crt")
	c.Assert(err, check.IsNil)
	key, err := ioutil.ReadFile("testdata/private.key")
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}}
	err = CreateApp(&a, s.user)
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
	cert, err := ioutil.ReadFile("testdata/certificate.crt")
	c.Assert(err, check.IsNil)
	key, err := ioutil.ReadFile("testdata/private.key")
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}, CName: []string{cname}}
	err = CreateApp(&a, s.user)
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
	cert, err := ioutil.ReadFile("testdata/certificate.crt")
	c.Assert(err, check.IsNil)
	key, err := ioutil.ReadFile("testdata/private.key")
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake-tls"}}, CName: []string{cname}}
	err = CreateApp(&a, s.user)
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	certs, err := a.GetCertificates()
	c.Assert(err, check.ErrorMatches, "no router with tls support")
	c.Assert(certs, check.IsNil)
}

func (s *S) TestAppMetricEnvs(c *check.C) {
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: nodecontainer.BsDefaultName,
		Config: docker.Config{
			Image: "img1",
			Env: []string{
				"OTHER_ENV=asd",
				"METRICS_BACKEND=LOGSTASH",
				"METRICS_LOGSTASH_URI=localhost:2222",
			},
		},
	})
	c.Assert(err, check.IsNil)
	a := App{Name: "app-name", Platform: "python"}
	envs, err := a.MetricEnvs()
	c.Assert(err, check.IsNil)
	expected := map[string]string{
		"METRICS_LOGSTASH_URI": "localhost:2222",
		"METRICS_BACKEND":      "LOGSTASH",
	}
	c.Assert(envs, check.DeepEquals, expected)
}

func (s *S) TestUpdateAppWithInvalidName(c *check.C) {
	app := App{Name: "app with invalid name", Plan: s.defaultPlan, Platform: "python", TeamOwner: s.team.Name, Pool: s.Pool}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)

	updateData := App{Name: app.Name, Description: "bleble"}
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Description, check.Equals, "bleble")
}

func (s *S) TestUpdateDescription(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla"}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "example", Description: "bleble"}
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Description, check.Equals, "bleble")
}

func (s *S) TestUpdateAppPlatform(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "example", Platform: "heimerdinger"}
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Platform, check.Equals, updateData.Platform)
	c.Assert(dbApp.UpdatePlatform, check.Equals, true)
}

func (s *S) TestUpdateAppPlatformWithVersion(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "example", Platform: "python:v3"}
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Platform, check.Equals, "python")
	c.Assert(dbApp.PlatformVersion, check.Equals, "v3")
	c.Assert(dbApp.UpdatePlatform, check.Equals, true)
}

func (s *S) TestUpdateTeamOwner(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla"}
	err := CreateApp(&app, s.user)
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
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.TeamOwner, check.Equals, teamName)
}

func (s *S) TestUpdateTeamOwnerNotExists(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla"}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "example", TeamOwner: "newowner"}
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "team not found")
	dbApp, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.TeamOwner, check.Equals, s.team.Name)
}

func (s *S) TestUpdatePool(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test"}
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("test", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "test2"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("test2", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	app := App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
	err = CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "test", Pool: "test2"}
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Pool, check.Equals, "test2")
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
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "test2", Provisioner: "fake2", Public: true}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	app := App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
	err = CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddUnits(1, "", nil)
	c.Assert(err, check.IsNil)
	prov, err := app.getProvisioner()
	c.Assert(err, check.IsNil)
	c.Assert(prov.GetName(), check.Equals, "fake1")
	c.Assert(p1.GetUnits(&app), check.HasLen, 1)
	c.Assert(p2.GetUnits(&app), check.HasLen, 0)
	c.Assert(p1.Provisioned(&app), check.Equals, true)
	c.Assert(p2.Provisioned(&app), check.Equals, false)
	updateData := App{Name: "test", Pool: "test2"}
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(app.Name)
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
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("test", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	app := App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
	err = CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "test", Pool: "test2"}
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.Equals, pool.ErrPoolNotFound)
	dbApp, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Pool, check.Equals, "test")
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
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "test2", Provisioner: "fake2", Public: true}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)

	app := App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
	err = CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	v1 := volume.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volume.VolumePlan{Name: "nfs"}}
	err = v1.Create()
	c.Assert(err, check.IsNil)
	err = v1.BindApp(app.Name, "/mnt", false)
	c.Assert(err, check.IsNil)
	err = app.AddUnits(1, "", nil)
	c.Assert(err, check.IsNil)
	prov, err := app.getProvisioner()
	c.Assert(err, check.IsNil)
	c.Assert(prov.GetName(), check.Equals, "fake1")
	c.Assert(p1.GetUnits(&app), check.HasLen, 1)
	c.Assert(p2.GetUnits(&app), check.HasLen, 0)
	c.Assert(p1.Provisioned(&app), check.Equals, true)
	c.Assert(p2.Provisioned(&app), check.Equals, false)
	updateData := App{Name: "test", Pool: "test2"}
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.ErrorMatches, "can't change the provisioner of an app with binded volumes")
}

func (s *S) TestUpdatePoolWithBindedVolumeSameProvisioner(c *check.C) {
	p1 := provisiontest.NewFakeProvisioner()
	p1.Name = "fake1"
	provision.Register("fake1", func() (provision.Provisioner, error) {
		return p1, nil
	})
	opts := pool.AddPoolOptions{Name: "test", Provisioner: "fake1", Public: true}
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "test2", Provisioner: "fake1", Public: true}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)

	app := App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
	err = CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	config.Set("volume-plans:nfs:fake:plugin", "nfs")
	defer config.Unset("volume-plans")
	v1 := volume.Volume{Name: "v1", Pool: s.Pool, TeamOwner: s.team.Name, Plan: volume.VolumePlan{Name: "nfs"}}
	err = v1.Create()
	c.Assert(err, check.IsNil)
	err = v1.BindApp(app.Name, "/mnt", false)
	c.Assert(err, check.IsNil)
	err = app.AddUnits(1, "", nil)
	c.Assert(err, check.IsNil)
	prov, err := app.getProvisioner()
	c.Assert(err, check.IsNil)
	c.Assert(prov.GetName(), check.Equals, "fake1")
	c.Assert(p1.GetUnits(&app), check.HasLen, 1)
	c.Assert(p1.Provisioned(&app), check.Equals, true)
	updateData := App{Name: "test", Pool: "test2"}
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
}

func (s *S) TestUpdatePlan(c *check.C) {
	plan := appTypes.Plan{Name: "something", CpuShare: 100, Memory: 268435456}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		c.Assert(name, check.Equals, plan.Name)
		return &plan, nil
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{s.defaultPlan, plan}, nil
	}
	a := App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Memory: 536870912, CpuShare: 50}, TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	c.Assert(routertest.FakeRouter.HasBackend(a.Name), check.Equals, true)
	c.Assert(routertest.HCRouter.HasBackend(a.Name), check.Equals, false)
	updateData := App{Name: "my-test-app", Plan: appTypes.Plan{Name: "something"}}
	err = a.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Plan, check.DeepEquals, plan)
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 1)
}

func (s *S) TestUpdatePlanWithConstraint(c *check.C) {
	plan := appTypes.Plan{Name: "something", CpuShare: 100, Memory: 268435456}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		c.Assert(name, check.Equals, plan.Name)
		return &plan, nil
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{s.defaultPlan, plan}, nil
	}
	err := pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr:  "pool1",
		Field:     pool.ConstraintTypePlan,
		Values:    []string{plan.Name},
		Blacklist: true,
	})
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Memory: 536870912, CpuShare: 50}, TeamOwner: s.team.Name}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "my-test-app", Plan: appTypes.Plan{Name: "something"}}
	err = a.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.ErrorMatches, `App plan "something" is not allowed on pool "pool1"`)
}

func (s *S) TestUpdatePlanNoRouteChange(c *check.C) {
	plan := appTypes.Plan{Name: "something", CpuShare: 100, Memory: 268435456}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		c.Assert(name, check.Equals, plan.Name)
		return &plan, nil
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{s.defaultPlan, plan}, nil
	}
	a := App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Memory: 536870912, CpuShare: 50}, TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	c.Assert(routertest.FakeRouter.HasBackend(a.Name), check.Equals, true)
	updateData := App{Name: "my-test-app", Plan: appTypes.Plan{Name: "something"}}
	err = a.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Plan, check.DeepEquals, plan)
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 1)
	c.Assert(routertest.FakeRouter.HasBackend(dbApp.Name), check.Equals, true)
	routes, err := routertest.FakeRouter.Routes(dbApp.Name)
	c.Assert(err, check.IsNil)
	routesStr := make([]string, len(routes))
	for i, route := range routes {
		routesStr[i] = route.String()
	}
	units, err := dbApp.Units()
	c.Assert(err, check.IsNil)
	expected := make([]string, len(units))
	for i, unit := range units {
		expected[i] = unit.Address.String()
	}
	sort.Strings(routesStr)
	sort.Strings(expected)
	c.Assert(routesStr, check.DeepEquals, expected)
}

func (s *S) TestUpdatePlanNotFound(c *check.C) {
	var app App
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		return nil, appTypes.ErrPlanNotFound
	}
	updateData := App{Name: "my-test-app", Plan: appTypes.Plan{Name: "some-unknown-plan"}}
	err := app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.Equals, appTypes.ErrPlanNotFound)
}

func (s *S) TestUpdatePlanRestartFailure(c *check.C) {
	plan := appTypes.Plan{Name: "something", CpuShare: 100, Memory: 268435456}
	oldPlan := appTypes.Plan{Name: "old", CpuShare: 50, Memory: 536870912}
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
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	c.Assert(routertest.FakeRouter.HasBackend(a.Name), check.Equals, true)
	c.Assert(routertest.HCRouter.HasBackend(a.Name), check.Equals, false)
	s.provisioner.PrepareFailure("Restart", fmt.Errorf("cannot restart app, I'm sorry"))
	updateData := App{Name: "my-test-app", Routers: []appTypes.AppRouter{{Name: "fake-hc"}}, Plan: appTypes.Plan{Name: "something"}}
	err = a.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.NotNil)
	dbApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Plan.Name, check.Equals, "old")
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 0)
	c.Assert(routertest.FakeRouter.HasBackend(dbApp.Name), check.Equals, true)
	c.Assert(routertest.HCRouter.HasBackend(dbApp.Name), check.Equals, false)
	routes, err := routertest.FakeRouter.Routes(dbApp.Name)
	c.Assert(err, check.IsNil)
	routesStr := make([]string, len(routes))
	for i, route := range routes {
		routesStr[i] = route.String()
	}
	units, err := dbApp.Units()
	c.Assert(err, check.IsNil)
	expected := make([]string, len(units))
	for i, unit := range units {
		expected[i] = unit.Address.String()
	}
	sort.Strings(routesStr)
	sort.Strings(expected)
	c.Assert(routesStr, check.DeepEquals, expected)
}

func (s *S) TestUpdateIgnoresEmptyAndDuplicatedTags(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla", Tags: []string{"tag1"}}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Tags: []string{"tag2 ", "  tag3  ", "", " tag3", "  "}}
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Tags, check.DeepEquals, []string{"tag2", "tag3"})
}

func (s *S) TestUpdatePlatform(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla"}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{UpdatePlatform: true}
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.UpdatePlatform, check.Equals, true)
}

func (s *S) TestUpdateWithEmptyTagsRemovesAllTags(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla", Tags: []string{"tag1"}}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Description: "ble", Tags: []string{}}
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Tags, check.DeepEquals, []string{})
}

func (s *S) TestUpdateWithoutTagsKeepsOriginalTags(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla", Tags: []string{"tag1", "tag2"}}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Description: "ble", Tags: nil}
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Tags, check.DeepEquals, []string{"tag1", "tag2"})
}

func (s *S) TestUpdateDescriptionPoolPlan(c *check.C) {
	opts := pool.AddPoolOptions{Name: "test"}
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("test", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "test2"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("test2", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	plan := appTypes.Plan{Name: "something", CpuShare: 100, Memory: 268435456}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{s.defaultPlan, plan}, nil
	}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		c.Assert(name, check.Equals, plan.Name)
		return &plan, nil
	}
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Routers: []appTypes.AppRouter{{Name: "fake"}}, Plan: appTypes.Plan{Memory: 536870912, CpuShare: 50}, Description: "blablabla", Pool: "test"}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	updateData := App{Name: "my-test-app", Plan: appTypes.Plan{Name: "something"}, Description: "bleble", Pool: "test2"}
	err = a.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Plan, check.DeepEquals, plan)
	c.Assert(dbApp.Description, check.Equals, "bleble")
	c.Assert(dbApp.Pool, check.Equals, "test2")
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 1)
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
	err := RenameTeam("t2", "t9000")
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
	locked, err := AcquireApplicationLock("test2", "me", "because yes")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
	err = RenameTeam("t2", "t9000")
	c.Assert(err, check.ErrorMatches, `unable to acquire lock for app "test2"`)
	var dbApps []App
	err = s.conn.Apps().Find(nil).Sort("name").All(&dbApps)
	c.Assert(err, check.IsNil)
	c.Assert(dbApps, check.HasLen, 2)
	c.Assert(dbApps[0].TeamOwner, check.Equals, "t1")
	c.Assert(dbApps[1].TeamOwner, check.Equals, "t2")
	c.Assert(dbApps[0].Teams, check.DeepEquals, []string{"t2", "t3", "t1"})
	c.Assert(dbApps[1].Teams, check.DeepEquals, []string{"t3", "t1"})
}

func (s *S) TestRenameTeamUnchanagedLockedApp(c *check.C) {
	apps := []App{
		{Name: "test1", TeamOwner: "t1", Routers: []appTypes.AppRouter{{Name: "fake"}}, Teams: []string{"t2", "t3", "t1"}},
		{Name: "test2", TeamOwner: "t2", Routers: []appTypes.AppRouter{{Name: "fake"}}, Teams: []string{"t3", "t1"}},
		{Name: "test3", TeamOwner: "t3", Routers: []appTypes.AppRouter{{Name: "fake"}}, Teams: []string{"t3", "t1"}},
	}
	for _, a := range apps {
		err := s.conn.Apps().Insert(a)
		c.Assert(err, check.IsNil)
	}
	locked, err := AcquireApplicationLock("test3", "me", "because yes")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
	err = RenameTeam("t2", "t9000")
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
	config.Set("routers:fake-opts:type", "fake-opts")
	defer config.Unset("routers:fake-opts:type")
	app := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddRouter(appTypes.AppRouter{
		Name: "fake-opts",
		Opts: map[string]string{
			"a": "b",
		},
	})
	c.Assert(err, check.IsNil)
	err = app.UpdateRouter(appTypes.AppRouter{Name: "fake-opts", Opts: map[string]string{
		"c": "d",
	}})
	c.Assert(err, check.IsNil)
	routers := app.GetRouters()
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake"},
		{Name: "fake-opts", Opts: map[string]string{"c": "d"}},
	})
	c.Assert(routertest.OptsRouter.Opts["myapp"], check.DeepEquals, map[string]string{
		"c": "d",
	})
}

func (s *S) TestUpdateRouterNotSupported(c *check.C) {
	app := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddRouter(appTypes.AppRouter{
		Name: "fake-tls",
		Opts: map[string]string{
			"a": "b",
		},
	})
	c.Assert(err, check.IsNil)
	err = app.UpdateRouter(appTypes.AppRouter{Name: "fake-tls", Opts: map[string]string{
		"c": "d",
	}})
	c.Assert(err, check.ErrorMatches, "updating is not supported by router \"fake-tls\"")
}

func (s *S) TestUpdateRouterNotFound(c *check.C) {
	app := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(&app, s.user)
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
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddRouter(appTypes.AppRouter{
		Name: "fake-tls",
	})
	c.Assert(err, check.IsNil)
	routers, err := app.GetRoutersWithAddr()
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Address: "myapp.fakerouter.com", Type: "fake"},
		{Name: "fake-tls", Address: "myapp.faketlsrouter.com", Type: "fake-tls"},
	})
	addrs, err := app.GetAddresses()
	c.Assert(err, check.IsNil)
	c.Assert(addrs, check.DeepEquals, []string{"myapp.fakerouter.com", "myapp.faketlsrouter.com"})
}

func (s *S) TestAppRemoveRouter(c *check.C) {
	app := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(&app, s.user)
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
		{Name: "fake-tls", Address: "myapp.faketlsrouter.com", Type: "fake-tls"},
	})
	addrs, err := app.GetAddresses()
	c.Assert(err, check.IsNil)
	c.Assert(addrs, check.DeepEquals, []string{"myapp.faketlsrouter.com"})
}

func (s *S) TestGetRoutersWithAddr(c *check.C) {
	app := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(&app, s.user)
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
		{Name: "fake", Address: "myapp.fakerouter.com", Type: "fake"},
		{Name: "fake-tls", Address: "myapp.faketlsrouter.com", Type: "fake-tls"},
	})
}

func (s *S) TestGetRoutersWithAddrError(c *check.C) {
	app := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddRouter(appTypes.AppRouter{
		Name: "fake-tls",
	})
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.FailForIp("fakemyapp")
	s.mockService.Cache.OnCreate = func(entry cache.CacheEntry) error {
		if entry.Value != "myapp.faketlsrouter.com" {
			c.Errorf("unexpected cache entry: %v", entry)
		}
		return nil
	}
	routers, err := app.GetRoutersWithAddr()
	c.Assert(err, check.ErrorMatches, `(?s)Forced failure.*`)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Address: "", Type: ""},
		{Name: "fake-tls", Address: "myapp.faketlsrouter.com", Type: "fake-tls"},
	})
}

func (s *S) TestGetRoutersWithAddrWithStatus(c *check.C) {
	config.Set("routers:mystatus:type", "fake-status")
	defer config.Unset("routers:mystatus")
	routertest.StatusRouter.Status = router.BackendStatusNotReady
	routertest.StatusRouter.StatusDetail = "burn"
	defer routertest.StatusRouter.Reset()
	app := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddRouter(appTypes.AppRouter{
		Name: "mystatus",
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
		{Name: "fake", Address: "myapp.fakerouter.com", Type: "fake"},
		{Name: "mystatus", Address: "myapp.fakerouter.com", Type: "fake-status", Status: "not ready", StatusDetail: "burn"},
	})
}

func (s *S) TestGetRoutersIgnoresDuplicatedEntry(c *check.C) {
	app := App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := CreateApp(&app, s.user)
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
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	app := App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
	err = CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	err = app.AddUnits(1, "web", nil)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "test", Description: "updated description"}
	err = app.Update(updateData, nil)
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
	err := CreateApp(&app, s.user)
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
	err = pool.AddPool(optsPool2)
	c.Assert(err, check.IsNil)
	pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr: optsPool2.Name,
		Field:    pool.ConstraintTypeService,
		Values: []string{
			svc.Name,
		},
		Blacklist: true,
	})
	err = app.Update(App{Pool: optsPool2.Name}, nil)
	c.Assert(err, check.NotNil)
}

func (s *S) TestGetUUID(c *check.C) {
	app := App{Name: "test", TeamOwner: s.team.Name, Pool: s.Pool}
	err := CreateApp(&app, s.user)
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
