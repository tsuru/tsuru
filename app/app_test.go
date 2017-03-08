// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"encoding/json"
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

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/safe"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/tsurutest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
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
	c.Assert(err, check.Equals, ErrAppNotFound)
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
	err = Delete(app, nil)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasBackend(app.Name), check.Equals, false)
	_, err = GetByName(app.Name)
	c.Assert(err, check.Equals, ErrAppNotFound)
	c.Assert(s.provisioner.Provisioned(&a), check.Equals, false)
	err = auth.ReserveApp(s.user)
	c.Assert(err, check.IsNil)
	count, err := s.logConn.Logs(app.Name).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
	_, err = repository.Manager().GetRepository(a.Name)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "repository not found")
	_, err = router.Retrieve(a.Name)
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
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
	_, err = event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppUpdateEnvSet,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	evt2, err := event.New(&event.Opts{
		Target:   event.Target{Type: "app", Value: "other"},
		Kind:     permission.PermAppUpdateEnvSet,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	Delete(app, nil)
	evts, err := event.List(&event.Filter{})
	c.Assert(err, check.IsNil)
	c.Assert(evts, eventtest.EvtEquals, evt2)
	evts, err = event.List(&event.Filter{IncludeRemoved: true})
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 2)
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
	Delete(a, nil)
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
	err = Delete(&a, nil)
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
	err = Delete(&a, nil)
	c.Assert(err, check.IsNil)
	c.Assert(s.provisioner.Provisioned(&a), check.Equals, false)
}

func (s *S) TestCreateApp(c *check.C) {
	a := App{
		Name:      "appname",
		Platform:  "python",
		TeamOwner: s.team.Name,
		Tags:      []string{"test a", "test b"},
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
	c.Assert(retrievedApp.Tags, check.DeepEquals, a.Tags)
	env := retrievedApp.InstanceEnv("")
	c.Assert(env["TSURU_APPNAME"].Value, check.Equals, a.Name)
	c.Assert(env["TSURU_APPNAME"].Public, check.Equals, false)
	err = auth.ReserveApp(s.user)
	_, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	_, err = repository.Manager().GetRepository(a.Name)
	c.Assert(err, check.IsNil)
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

func (s *S) TestCreateAppWithoutDefaultPlan(c *check.C) {
	s.conn.Plans().RemoveAll(nil)
	defer s.conn.Plans().Insert(s.defaultPlan)
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
	c.Assert(retrievedApp.Plan, check.DeepEquals, Plan{
		Name:     "autogenerated",
		Memory:   0,
		Swap:     0,
		CpuShare: 100,
	})
	_, err = repository.Manager().GetRepository(a.Name)
	c.Assert(err, check.IsNil)
}

func (s *S) TestCreateAppWithExplicitPlan(c *check.C) {
	myPlan := Plan{
		Name:     "myplan",
		Memory:   4194304,
		Swap:     2,
		CpuShare: 3,
	}
	err := myPlan.Save()
	c.Assert(err, check.IsNil)
	defer PlanRemove(myPlan.Name)
	a := App{
		Name:      "appname",
		Platform:  "python",
		Plan:      Plan{Name: "myplan"},
		TeamOwner: s.team.Name,
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": 1}})
	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	retrievedApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(retrievedApp.Plan, check.DeepEquals, myPlan)
	_, err = repository.Manager().GetRepository(a.Name)
	c.Assert(err, check.IsNil)
}

func (s *S) TestCreateAppUserQuotaExceeded(c *check.C) {
	app := App{Name: "america", Platform: "python", TeamOwner: s.team.Name}
	s.conn.Users().Update(
		bson.M{"email": s.user.Email},
		bson.M{"$set": bson.M{"quota.limit": 0}},
	)
	err := CreateApp(&app, s.user)
	e, ok := err.(*AppCreationError)
	c.Assert(ok, check.Equals, true)
	_, ok = e.Err.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
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
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	a := App{Name: "beyond"}
	err = CreateApp(&a, &u)
	c.Check(err, check.DeepEquals, &errors.ValidationError{Message: auth.ErrTeamNotFound.Error()})
}

func (s *S) TestCantCreateTwoAppsWithTheSameName(c *check.C) {
	err := CreateApp(&App{Name: "appname", TeamOwner: s.team.Name}, s.user)
	c.Assert(err, check.IsNil)
	a := App{Name: "appname", Platform: "python", TeamOwner: s.team.Name}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.NotNil)
	e, ok := err.(*AppCreationError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.app, check.Equals, "appname")
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
	msg := "Invalid app name, your app should have at most 63 " +
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
		Quota:     quota.Unlimited,
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	srvc := service.Service{
		Name:     "mysql",
		Endpoint: map[string]string{"production": server.URL},
	}
	err = srvc.Create()
	c.Assert(err, check.IsNil)
	defer srvc.Delete()
	si1 := service.ServiceInstance{
		Name:        "mydb",
		ServiceName: "mysql",
		Apps:        []string{app.Name},
	}
	err = si1.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si1.Name})
	si2 := service.ServiceInstance{
		Name:        "yourdb",
		ServiceName: "mysql",
		Apps:        []string{app.Name},
	}
	err = si2.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si2.Name})
	unit := provision.Unit{ID: "some-unit", Ip: "127.0.2.1"}
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
		Quota:     quota.Unlimited,
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	srvc := service.Service{
		Name:     "mysql",
		Endpoint: map[string]string{"production": server.URL},
	}
	err = srvc.Create()
	c.Assert(err, check.IsNil)
	defer srvc.Delete()
	si1 := service.ServiceInstance{
		Name:        "mydb",
		ServiceName: "mysql",
		Apps:        []string{app.Name},
	}
	err = si1.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si1.Name})
	si2 := service.ServiceInstance{
		Name:        "yourdb",
		ServiceName: "mysql",
		Apps:        []string{app.Name},
	}
	err = si2.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si2.Name})
	unit := provision.Unit{ID: "some-unit", Ip: "127.0.2.1"}
	err = app.BindUnit(&unit)
	c.Assert(err, check.ErrorMatches, "Failed to bind the instance \"mysql/yourdb\" to the unit \"127.0.2.1\": invalid response: myerr")
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
		Quota:     quota.Unlimited,
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

func (s *S) TestAddUnitsWithWriter(c *check.C) {
	app := App{
		Name: "warpaint", Platform: "python",
		Quota:     quota.Unlimited,
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
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	err = ChangeQuota(&app, 7)
	c.Assert(err, check.IsNil)
	otherApp := App{Name: "warpaint"}
	err = otherApp.AddUnits(5, "web", nil)
	c.Assert(err, check.IsNil)
	units := s.provisioner.GetUnits(&app)
	c.Assert(units, check.HasLen, 5)
	err = otherApp.AddUnits(2, "web", nil)
	c.Assert(err, check.IsNil)
	units = s.provisioner.GetUnits(&app)
	c.Assert(units, check.HasLen, 7)
	err = reserveUnits(&app, 1)
	_, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestAddUnitsQuotaExceeded(c *check.C) {
	app := App{Name: "warpaint", Platform: "ruby", TeamOwner: s.team.Name, Router: "fake"}
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
	}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	err = ChangeQuota(&app, 10)
	c.Assert(err, check.IsNil)
	err = app.AddUnits(11, "web", nil)
	e, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Available, check.Equals, uint(10))
	c.Assert(e.Requested, check.Equals, uint(11))
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
		Quota:     quota.Unlimited,
		TeamOwner: s.team.Name,
		Router:    "fake",
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
		Quota: quota.Unlimited,
	}
	err := app.AddUnits(2, "web", nil)
	c.Assert(err, check.NotNil)
	_, err = GetByName(app.Name)
	c.Assert(err, check.Equals, ErrAppNotFound)
}

func (s *S) TestRemoveUnitsWithQuota(c *check.C) {
	a := App{
		Name:      "ble",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = ChangeQuota(&a, 6)
	c.Assert(err, check.IsNil)
	err = a.SetQuotaInUse(6)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 5, "web", nil)
	err = a.RemoveUnits(4, "web", nil)
	c.Assert(err, check.IsNil)
	err = tsurutest.WaitCondition(2e9, func() bool {
		app, appErr := GetByName(a.Name)
		if appErr != nil {
			c.Log(appErr)
			return false
		}
		return app.Quota.InUse == 1
	})
	c.Assert(err, check.IsNil)
}

func (s *S) TestRemoveUnits(c *check.C) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	app := App{
		Name:      "chemistry",
		Platform:  "python",
		Quota:     quota.Unlimited,
		TeamOwner: s.team.Name,
	}
	instance := service.ServiceInstance{
		Name:        "my-inst",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{app.Name},
	}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-inst"})
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
		gotApp, inErr := GetByName(app.Name)
		if inErr != nil {
			c.Log(inErr)
			return false
		}
		units, inErr := app.Units()
		c.Assert(inErr, check.IsNil)
		return len(units) == 4 && gotApp.Quota.InUse == 4
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
	_, err = UpdateNodeStatus(provision.NodeStatusData{Addrs: []string{"addr1"}, Units: unitStates})
	c.Assert(err, check.Equals, provision.ErrNodeNotFound)
}

func (s *S) TestGrantAccess(c *check.C) {
	user := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
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
	user := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	})
	team := auth.Team{Name: "abcd"}
	err := s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	app := App{Name: "app-name", Platform: "django", Teams: []string{s.team.Name, team.Name}}
	err = s.conn.Apps().Insert(app)
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
	team := auth.Team{Name: "abcd"}
	err := s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	user := customUserWithPermission(c, "myuser", permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permission.CtxTeam, team.Name),
	})
	app := App{Name: "app-name", Platform: "django", Teams: []string{s.team.Name, team.Name}}
	err = s.conn.Apps().Insert(app)
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

func (s *S) TestSetEnvRespectsThePublicOnlyFlagKeepPrivateVariablesWhenServiceSets(c *check.C) {
	a := App{
		Name: "myapp",
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST": {
				Name:         "DATABASE_HOST",
				Value:        "localhost",
				Public:       false,
				InstanceName: "some service",
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
			Public: false,
		},
		{
			Name:   "DATABASE_PASSWORD",
			Value:  "123",
			Public: true,
		},
	}
	var buf bytes.Buffer
	err = a.setEnvsToApp(
		bind.SetEnvApp{
			Envs:          envs,
			PublicOnly:    true,
			ShouldRestart: true,
		}, &buf)
	c.Assert(err, check.IsNil)
	newApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:         "DATABASE_HOST",
			Value:        "localhost",
			Public:       false,
			InstanceName: "some service",
		},
		"DATABASE_PASSWORD": {
			Name:   "DATABASE_PASSWORD",
			Value:  "123",
			Public: true,
		},
	}
	c.Assert(newApp.Env, check.DeepEquals, expected)
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 1)
	c.Assert(buf.String(), check.Equals, "---- Setting 2 new environment variables ----\nrestarting app")
}

func (s *S) TestSetEnvRespectsThePublicOnlyFlagOverwrittenAllVariablesWhenItsFalse(c *check.C) {
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
	err = a.setEnvsToApp(
		bind.SetEnvApp{
			Envs:          envs,
			PublicOnly:    false,
			ShouldRestart: true,
		}, nil)
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
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 1)
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
	err = a.setEnvsToApp(
		bind.SetEnvApp{
			Envs:          envs,
			PublicOnly:    false,
			ShouldRestart: false,
		}, nil)
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
	err = a.setEnvsToApp(
		bind.SetEnvApp{
			Envs:          envs,
			PublicOnly:    false,
			ShouldRestart: false,
		}, nil)
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

func (s *S) TestUnsetEnvRespectsThePublicOnlyFlagKeepPrivateVariablesWhenItsTrue(c *check.C) {
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
		Quota: quota.Quota{
			Limit: 10,
		},
		TeamOwner: s.team.Name,
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", nil)
	c.Assert(err, check.IsNil)
	err = a.UnsetEnvs(
		bind.UnsetEnvApp{
			VariableNames: []string{"DATABASE_HOST", "DATABASE_PASSWORD"},
			PublicOnly:    true,
			ShouldRestart: true,
		}, nil)
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
	c.Assert(newApp.Env, check.DeepEquals, expected)
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 1)
}

func (s *S) TestUnsetEnvRespectsThePublicOnlyFlagUnsettingAllVariablesWhenItsFalse(c *check.C) {
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
		Quota: quota.Quota{
			Limit: 10,
		},
		TeamOwner: s.team.Name,
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", nil)
	c.Assert(err, check.IsNil)
	err = a.UnsetEnvs(
		bind.UnsetEnvApp{
			VariableNames: []string{"DATABASE_HOST", "DATABASE_PASSWORD"},
			PublicOnly:    false,
			ShouldRestart: true,
		}, nil)
	c.Assert(err, check.IsNil)
	newApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(newApp.Env, check.DeepEquals, map[string]bind.EnvVar{})
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
		Quota: quota.Quota{
			Limit: 10,
		},
		TeamOwner: s.team.Name,
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", nil)
	c.Assert(err, check.IsNil)
	err = a.UnsetEnvs(
		bind.UnsetEnvApp{
			VariableNames: []string{"DATABASE_HOST", "DATABASE_PASSWORD"},
			PublicOnly:    false,
			ShouldRestart: false,
		}, nil)
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
	err = a.UnsetEnvs(
		bind.UnsetEnvApp{
			VariableNames: []string{"DATABASE_HOST", "DATABASE_PASSWORD"},
			PublicOnly:    false,
			ShouldRestart: true,
		}, nil)
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
	envs := map[string]bind.EnvVar{
		"DATABASE_HOST": {Name: "DATABASE_HOST", Value: "localhost", Public: false, InstanceName: "mysql"},
		"DATABASE_USER": {Name: "DATABASE_USER", Value: "root", Public: true, InstanceName: "mysql"},
		"HOST":          {Name: "HOST", Value: "10.0.2.1", Public: false, InstanceName: "redis"},
	}
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": {Name: "DATABASE_HOST", Value: "localhost", Public: false, InstanceName: "mysql"},
		"DATABASE_USER": {Name: "DATABASE_USER", Value: "root", Public: true, InstanceName: "mysql"},
	}
	a := App{Name: "hi-there", Env: envs}
	c.Assert(a.InstanceEnv("mysql"), check.DeepEquals, expected)
}

func (s *S) TestInstanceEnvironmentDoesNotPanicIfTheEnvMapIsNil(c *check.C) {
	a := App{Name: "hi-there"}
	c.Assert(a.InstanceEnv("mysql"), check.DeepEquals, map[string]bind.EnvVar{})
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
	other := App{Name: a.Name, Router: "fake"}
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
	instance := bind.ServiceInstance{
		Name: "myinstance",
		Envs: map[string]string{
			"DATABASE_HOST": "localhost",
			"DATABASE_PORT": "3306",
			"DATABASE_USER": "root",
		},
	}
	err = a.AddInstance(
		bind.InstanceApp{
			ServiceName:   "myservice",
			Instance:      instance,
			ShouldRestart: true,
		}, nil)
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	expected := map[string][]bind.ServiceInstance{"myservice": {instance}}
	env, ok := a.Env[TsuruServicesEnvVar]
	c.Assert(ok, check.Equals, true)
	c.Assert(env.Public, check.Equals, false)
	c.Assert(env.Name, check.Equals, TsuruServicesEnvVar)
	var got map[string][]bind.ServiceInstance
	err = json.Unmarshal([]byte(env.Value), &got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, expected)
	delete(a.Env, TsuruServicesEnvVar)
	c.Assert(a.Env, check.DeepEquals, map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:         "DATABASE_HOST",
			Value:        "localhost",
			Public:       false,
			InstanceName: "myinstance",
		},
		"DATABASE_PORT": {
			Name:         "DATABASE_PORT",
			Value:        "3306",
			Public:       false,
			InstanceName: "myinstance",
		},
		"DATABASE_USER": {
			Name:         "DATABASE_USER",
			Value:        "root",
			Public:       false,
			InstanceName: "myinstance",
		},
	})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 0)
}

func (s *S) TestAddInstanceDuplicated(c *check.C) {
	a := &App{Name: "sith", TeamOwner: s.team.Name}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	instance := bind.ServiceInstance{
		Name: "myinstance",
		Envs: map[string]string{
			"ZMQ_PEER": "localhost",
		},
	}
	err = a.AddInstance(
		bind.InstanceApp{
			ServiceName:   "myservice",
			Instance:      instance,
			ShouldRestart: true,
		}, nil)
	c.Assert(err, check.IsNil)
	// inserts duplicated
	instance = bind.ServiceInstance{
		Name: "myinstance",
		Envs: map[string]string{
			"ZMQ_PEER": "8.8.8.8",
		},
	}
	err = a.AddInstance(
		bind.InstanceApp{
			ServiceName:   "myservice",
			Instance:      instance,
			ShouldRestart: true,
		}, nil)
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	expected := map[string][]bind.ServiceInstance{"myservice": {instance}}
	env, ok := a.Env[TsuruServicesEnvVar]
	c.Assert(ok, check.Equals, true)
	c.Assert(env.Public, check.Equals, false)
	c.Assert(env.Name, check.Equals, TsuruServicesEnvVar)
	var got map[string][]bind.ServiceInstance
	err = json.Unmarshal([]byte(env.Value), &got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, expected)
	c.Assert(a.Env["ZMQ_PEER"], check.DeepEquals, bind.EnvVar{
		Name:         "ZMQ_PEER",
		Value:        "8.8.8.8",
		Public:       false,
		InstanceName: "myinstance",
	})
}

func (s *S) TestAddInstanceWithUnits(c *check.C) {
	a := &App{Name: "dark", Quota: quota.Quota{Limit: 10}, TeamOwner: s.team.Name}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", nil)
	c.Assert(err, check.IsNil)
	instance := bind.ServiceInstance{
		Name: "myinstance",
		Envs: map[string]string{
			"DATABASE_HOST": "localhost",
		},
	}
	err = a.AddInstance(
		bind.InstanceApp{
			ServiceName:   "myservice",
			Instance:      instance,
			ShouldRestart: true,
		}, nil)
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	expected := map[string][]bind.ServiceInstance{"myservice": {instance}}
	env, ok := a.Env[TsuruServicesEnvVar]
	c.Assert(ok, check.Equals, true)
	c.Assert(env.Public, check.Equals, false)
	c.Assert(env.Name, check.Equals, TsuruServicesEnvVar)
	var got map[string][]bind.ServiceInstance
	err = json.Unmarshal([]byte(env.Value), &got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, expected)
	delete(a.Env, TsuruServicesEnvVar)
	c.Assert(a.Env, check.DeepEquals, map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:         "DATABASE_HOST",
			Value:        "localhost",
			Public:       false,
			InstanceName: "myinstance",
		},
	})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 1)
}

func (s *S) TestAddInstanceWithUnitsNoRestart(c *check.C) {
	a := &App{Name: "dark", Quota: quota.Quota{Limit: 10}, TeamOwner: s.team.Name}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", nil)
	c.Assert(err, check.IsNil)
	instance := bind.ServiceInstance{
		Name: "myinstance",
		Envs: map[string]string{
			"DATABASE_HOST": "localhost",
		},
	}
	err = a.AddInstance(
		bind.InstanceApp{
			ServiceName:   "myservice",
			Instance:      instance,
			ShouldRestart: false,
		}, nil)
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	expected := map[string][]bind.ServiceInstance{"myservice": {instance}}
	env, ok := a.Env[TsuruServicesEnvVar]
	c.Assert(ok, check.Equals, true)
	c.Assert(env.Public, check.Equals, false)
	c.Assert(env.Name, check.Equals, TsuruServicesEnvVar)
	var got map[string][]bind.ServiceInstance
	err = json.Unmarshal([]byte(env.Value), &got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, expected)
	delete(a.Env, TsuruServicesEnvVar)
	c.Assert(a.Env, check.DeepEquals, map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:         "DATABASE_HOST",
			Value:        "localhost",
			Public:       false,
			InstanceName: "myinstance",
		},
	})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 0)
}

func (s *S) TestAddInstanceMultipleServices(c *check.C) {
	a := &App{
		Name: "dark",
		Env: map[string]bind.EnvVar{
			TsuruServicesEnvVar: {
				Name:   TsuruServicesEnvVar,
				Public: false,
				Value:  `{"mysql": [{"instance_name": "mydb", "envs": {"DATABASE_NAME": "mydb"}}]}`,
			},
		},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	instance1 := bind.ServiceInstance{
		Name: "myinstance",
		Envs: map[string]string{"DATABASE_NAME": "myinstance"},
	}
	err = a.AddInstance(
		bind.InstanceApp{
			ServiceName:   "mysql",
			Instance:      instance1,
			ShouldRestart: true,
		}, nil)
	c.Assert(err, check.IsNil)
	instance2 := bind.ServiceInstance{
		Name: "yourinstance",
		Envs: map[string]string{"DATABASE_NAME": "supermongo"},
	}
	err = a.AddInstance(
		bind.InstanceApp{
			ServiceName:   "mongodb",
			Instance:      instance2,
			ShouldRestart: true,
		}, nil)
	c.Assert(err, check.IsNil)
	expected := map[string][]bind.ServiceInstance{
		"mysql":   {bind.ServiceInstance{Name: "mydb", Envs: map[string]string{"DATABASE_NAME": "mydb"}}, instance1},
		"mongodb": {instance2},
	}
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(a.parsedTsuruServices(), check.DeepEquals, expected)
	delete(a.Env, TsuruServicesEnvVar)
	c.Assert(a.Env, check.DeepEquals, map[string]bind.EnvVar{
		"DATABASE_NAME": {
			Name:         "DATABASE_NAME",
			Value:        "supermongo",
			Public:       false,
			InstanceName: "yourinstance",
		},
	})
}

func (s *S) TestAddInstanceAndRemoveInstanceMultipleServices(c *check.C) {
	a := &App{
		Name:      "fuchsia",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	instance1 := bind.ServiceInstance{
		Name: "myinstance",
		Envs: map[string]string{"DATABASE_NAME": "myinstance"},
	}
	err = a.AddInstance(
		bind.InstanceApp{
			ServiceName:   "mysql",
			Instance:      instance1,
			ShouldRestart: true,
		}, nil)
	c.Assert(err, check.IsNil)
	instance2 := bind.ServiceInstance{
		Name: "yourinstance",
		Envs: map[string]string{"DATABASE_NAME": "supermongo"},
	}
	err = a.AddInstance(
		bind.InstanceApp{
			ServiceName:   "mongodb",
			Instance:      instance2,
			ShouldRestart: true,
		}, nil)
	c.Assert(err, check.IsNil)
	err = a.RemoveInstance(
		bind.InstanceApp{
			ServiceName:   "mysql",
			Instance:      instance1,
			ShouldRestart: true,
		}, nil)
	c.Assert(err, check.IsNil)
	expected := map[string][]bind.ServiceInstance{
		"mysql":   {},
		"mongodb": {instance2},
	}
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(a.parsedTsuruServices(), check.DeepEquals, expected)
	delete(a.Env, TsuruServicesEnvVar)
	c.Assert(a.Env, check.DeepEquals, map[string]bind.EnvVar{
		"DATABASE_NAME": {
			Name:         "DATABASE_NAME",
			Value:        "supermongo",
			Public:       false,
			InstanceName: "yourinstance",
		},
	})
}

func (s *S) TestRemoveInstance(c *check.C) {
	a := &App{
		Name: "dark",
		Env: map[string]bind.EnvVar{
			TsuruServicesEnvVar: {
				Name:   TsuruServicesEnvVar,
				Public: false,
				Value:  `{"mysql": [{"instance_name": "mydb", "envs": {"DATABASE_NAME": "mydb"}}]}`,
			},
			"DATABASE_NAME": {
				Name:         "DATABASE_NAME",
				Public:       false,
				Value:        "mydb",
				InstanceName: "mydb",
			},
		},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	instance := bind.ServiceInstance{Name: "mydb", Envs: map[string]string{"DATABASE_NAME": "mydb"}}
	err = a.RemoveInstance(
		bind.InstanceApp{
			ServiceName:   "mysql",
			Instance:      instance,
			ShouldRestart: true,
		}, nil)
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	env, ok := a.Env[TsuruServicesEnvVar]
	c.Assert(ok, check.Equals, true)
	c.Assert(env.Value, check.Equals, `{"mysql":[]}`)
	c.Assert(env.Public, check.Equals, false)
	c.Assert(env.Name, check.Equals, TsuruServicesEnvVar)
	delete(a.Env, TsuruServicesEnvVar)
	c.Assert(a.Env, check.DeepEquals, map[string]bind.EnvVar{})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 0)
}

func (s *S) TestRemoveInstanceShifts(c *check.C) {
	value := `{"mysql": [{"instance_name": "mydb", "envs": {"DATABASE_NAME": "mydb"}},
{"instance_name": "yourdb", "envs": {"DATABASE_NAME": "yourdb"}},
{"instance_name": "hisdb", "envs": {"DATABASE_NAME": "hisdb"}},
{"instance_name": "herdb", "envs": {"DATABASE_NAME": "herdb"}},
{"instance_name": "ourdb", "envs": {"DATABASE_NAME": "ourdb"}}
]}`
	a := &App{
		Name: "dark",
		Env: map[string]bind.EnvVar{
			TsuruServicesEnvVar: {
				Name:   TsuruServicesEnvVar,
				Public: false,
				Value:  value,
			},
		},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	instance := bind.ServiceInstance{Name: "hisdb"}
	err = a.RemoveInstance(
		bind.InstanceApp{
			ServiceName:   "mysql",
			Instance:      instance,
			ShouldRestart: true,
		}, nil)
	c.Assert(err, check.IsNil)
	expected := map[string][]bind.ServiceInstance{
		"mysql": {
			bind.ServiceInstance{Name: "mydb", Envs: map[string]string{"DATABASE_NAME": "mydb"}},
			bind.ServiceInstance{Name: "yourdb", Envs: map[string]string{"DATABASE_NAME": "yourdb"}},
			bind.ServiceInstance{Name: "herdb", Envs: map[string]string{"DATABASE_NAME": "herdb"}},
			bind.ServiceInstance{Name: "ourdb", Envs: map[string]string{"DATABASE_NAME": "ourdb"}},
		},
	}
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	env, ok := a.Env[TsuruServicesEnvVar]
	c.Assert(ok, check.Equals, true)
	c.Assert(env.Public, check.Equals, false)
	c.Assert(env.Name, check.Equals, TsuruServicesEnvVar)
	var got map[string][]bind.ServiceInstance
	err = json.Unmarshal([]byte(env.Value), &got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, expected)
}

func (s *S) TestRemoveInstanceNotFound(c *check.C) {
	a := &App{
		Name: "dark",
		Env: map[string]bind.EnvVar{
			TsuruServicesEnvVar: {
				Name:   TsuruServicesEnvVar,
				Public: false,
				Value:  `{"mysql": [{"instance_name": "mydb", "envs": {"DATABASE_NAME": "mydb"}}]}`,
			},
		},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	instance := bind.ServiceInstance{Name: "yourdb"}
	err = a.RemoveInstance(
		bind.InstanceApp{
			ServiceName:   "mysql",
			Instance:      instance,
			ShouldRestart: true,
		}, nil)
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	services := a.parsedTsuruServices()
	c.Assert(services, check.DeepEquals, map[string][]bind.ServiceInstance{
		"mysql": {
			{
				Name: "mydb",
				Envs: map[string]string{"DATABASE_NAME": "mydb"},
			},
		},
	})
}

func (s *S) TestRemoveInstanceServiceNotFound(c *check.C) {
	a := &App{
		Name: "dark",
		Env: map[string]bind.EnvVar{
			TsuruServicesEnvVar: {
				Name:   TsuruServicesEnvVar,
				Public: false,
				Value:  `{"mysql": [{"instance_name": "mydb", "envs": {"DATABASE_NAME": "mydb"}}]}`,
			},
		},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	instance := bind.ServiceInstance{Name: "mydb"}
	err = a.RemoveInstance(
		bind.InstanceApp{
			ServiceName:   "mongodb",
			Instance:      instance,
			ShouldRestart: true,
		}, nil)
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	services := a.parsedTsuruServices()
	c.Assert(services, check.DeepEquals, map[string][]bind.ServiceInstance{
		"mysql": {
			{
				Name: "mydb",
				Envs: map[string]string{"DATABASE_NAME": "mydb"},
			},
		},
	})
}

func (s *S) TestRemoveInstanceWithUnits(c *check.C) {
	a := &App{
		Name: "dark",
		Env: map[string]bind.EnvVar{
			TsuruServicesEnvVar: {
				Name:   TsuruServicesEnvVar,
				Public: false,
				Value:  `{"mysql": [{"instance_name": "mydb", "envs": {"DATABASE_NAME": "mydb"}}]}`,
			},
			"DATABASE_NAME": {
				Name:         "DATABASE_NAME",
				Public:       false,
				Value:        "mydb",
				InstanceName: "mydb",
			},
		},
		Quota:     quota.Quota{Limit: 10},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", nil)
	c.Assert(err, check.IsNil)
	instance := bind.ServiceInstance{Name: "mydb", Envs: map[string]string{"DATABASE_NAME": "mydb"}}
	err = a.RemoveInstance(
		bind.InstanceApp{
			ServiceName:   "mysql",
			Instance:      instance,
			ShouldRestart: true,
		}, nil)
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	env, ok := a.Env[TsuruServicesEnvVar]
	c.Assert(ok, check.Equals, true)
	c.Assert(env.Value, check.Equals, `{"mysql":[]}`)
	c.Assert(env.Public, check.Equals, false)
	c.Assert(env.Name, check.Equals, TsuruServicesEnvVar)
	delete(a.Env, TsuruServicesEnvVar)
	c.Assert(a.Env, check.DeepEquals, map[string]bind.EnvVar{})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 1)
}

func (s *S) TestRemoveInstanceWithUnitsNoRestart(c *check.C) {
	a := &App{
		Name: "dark",
		Env: map[string]bind.EnvVar{
			TsuruServicesEnvVar: {
				Name:   TsuruServicesEnvVar,
				Public: false,
				Value:  `{"mysql": [{"instance_name": "mydb", "envs": {"DATABASE_NAME": "mydb"}}]}`,
			},
			"DATABASE_NAME": {
				Name:         "DATABASE_NAME",
				Public:       false,
				Value:        "mydb",
				InstanceName: "mydb",
			},
		},
		Quota:     quota.Quota{Limit: 10},
		TeamOwner: s.team.Name,
	}
	err := CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "web", nil)
	c.Assert(err, check.IsNil)
	instance := bind.ServiceInstance{Name: "mydb", Envs: map[string]string{"DATABASE_NAME": "mydb"}}
	err = a.RemoveInstance(
		bind.InstanceApp{
			ServiceName:   "mysql",
			Instance:      instance,
			ShouldRestart: false,
		}, nil)
	c.Assert(err, check.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, check.IsNil)
	env, ok := a.Env[TsuruServicesEnvVar]
	c.Assert(ok, check.Equals, true)
	c.Assert(env.Value, check.Equals, `{"mysql":[]}`)
	c.Assert(env.Public, check.Equals, false)
	c.Assert(env.Name, check.Equals, TsuruServicesEnvVar)
	delete(a.Env, TsuruServicesEnvVar)
	c.Assert(a.Env, check.DeepEquals, map[string]bind.EnvVar{})
	c.Assert(s.provisioner.Restarts(a, ""), check.Equals, 0)
}

func (s *S) TestIsValid(c *check.C) {
	err := auth.CreateTeam("noaccessteam", s.user)
	c.Assert(err, check.IsNil)
	err = provision.SetPoolConstraint(&provision.PoolConstraint{
		PoolExpr:  "pool1",
		Field:     "team",
		Values:    []string{"noaccessteam"},
		Blacklist: true,
	})
	c.Assert(err, check.IsNil)
	errMsg := "Invalid app name, your app should have at most 63 characters, containing only lower case letters, numbers or dashes, starting with a letter."
	var data = []struct {
		name      string
		teamOwner string
		pool      string
		router    string
		expected  string
	}{
		{"myappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyapp", s.team.Name, "pool1", "fake", errMsg},
		{"myappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyap", s.team.Name, "pool1", "fake", errMsg},
		{"myappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmya", s.team.Name, "pool1", "fake", ""},
		{"myApp", s.team.Name, "pool1", "fake", errMsg},
		{"my app", s.team.Name, "pool1", "fake", errMsg},
		{"123myapp", s.team.Name, "pool1", "fake", errMsg},
		{"myapp", s.team.Name, "pool1", "fake", ""},
		{"_theirapp", s.team.Name, "pool1", "fake", errMsg},
		{"my-app", s.team.Name, "pool1", "fake", ""},
		{"-myapp", s.team.Name, "pool1", "fake", errMsg},
		{"my_app", s.team.Name, "pool1", "fake", errMsg},
		{"b", s.team.Name, "pool1", "fake", ""},
		{InternalAppName, s.team.Name, "pool1", "fake", errMsg},
		{"myapp", "invalidteam", "pool1", "fake", "team not found"},
		{"myapp", s.team.Name, "pool1", "faketls", "router \"faketls\" is not available for pool \"pool1\""},
		{"myapp", "noaccessteam", "pool1", "fake", "App team owner \"noaccessteam\" has no access to pool \"pool1\""},
	}
	for _, d := range data {
		a := App{Name: d.name, TeamOwner: d.teamOwner, Pool: d.pool, Router: d.router}
		if valid := a.validate(); valid != nil && valid.Error() != d.expected {
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
		Router:    "fake",
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
		Router:    "fake",
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.AddBackend(a.Name)
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
		s.conn.Apps().Remove(bson.M{"name": a.Name})
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
		s.conn.Apps().Remove(bson.M{"name": a.Name})
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
	c.Assert(units[0].Ip, check.Equals, bindUnits[0].GetIp())
	c.Assert(units[1].Ip, check.Equals, bindUnits[1].GetIp())
}

func (s *S) TestAppMarshalJSON(c *check.C) {
	repository.Manager().CreateRepository("name", nil)
	opts := provision.AddPoolOptions{Name: "test", Default: false}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	app := App{
		Name:        "name",
		Platform:    "Framework",
		Teams:       []string{"team1"},
		Ip:          "10.10.10.1",
		CName:       []string{"name.mycompany.com"},
		Owner:       "appOwner",
		Deploys:     7,
		Pool:        "test",
		Description: "description",
		Plan:        Plan{Name: "myplan", Memory: 64, Swap: 128, CpuShare: 100},
		TeamOwner:   "myteam",
		Router:      "fake",
	}
	expected := map[string]interface{}{
		"name":        "name",
		"platform":    "Framework",
		"repository":  "git@" + repositorytest.ServerHost + ":name.git",
		"teams":       []interface{}{"team1"},
		"units":       nil,
		"ip":          "10.10.10.1",
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
		"router": "fake",
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
		Ip:          "10.10.10.1",
		CName:       []string{"name.mycompany.com"},
		Owner:       "appOwner",
		Deploys:     7,
		Pool:        "pool1",
		Description: "description",
		Plan:        Plan{Name: "myplan", Memory: 64, Swap: 128, CpuShare: 100},
		TeamOwner:   "myteam",
		Router:      "fake",
	}
	expected := map[string]interface{}{
		"name":        "name",
		"platform":    "Framework",
		"repository":  "",
		"teams":       []interface{}{"team1"},
		"units":       nil,
		"ip":          "10.10.10.1",
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
		"router": "fake",
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
	app := App{Name: "myapp", TeamOwner: s.team.Name}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&app, 1, "web", nil)
	var buf bytes.Buffer
	args := provision.RunArgs{Once: false, Isolated: false}
	err = app.Run("ls -lh", &buf, args)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "a lot of files")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls -lh"
	cmds := s.provisioner.GetCmds(expected, &app)
	c.Assert(cmds, check.HasLen, 1)
	var logs []Applog
	timeout := time.After(5 * time.Second)
	for {
		logs, err = app.LastLogs(10, Applog{})
		c.Assert(err, check.IsNil)
		if len(logs) > 1 {
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
	c.Assert(logs, check.HasLen, 2)
	c.Assert(logs[1].Message, check.Equals, "a lot of files")
	c.Assert(logs[1].Source, check.Equals, "app-run")
}

func (s *S) TestRunOnce(c *check.C) {
	s.provisioner.PrepareOutput([]byte("a lot of files"))
	app := App{
		Name:      "myapp",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&app, 1, "web", nil)
	var buf bytes.Buffer
	args := provision.RunArgs{Once: true, Isolated: false}
	err = app.Run("ls -lh", &buf, args)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "a lot of files")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls -lh"
	cmds := s.provisioner.GetCmds(expected, &app)
	c.Assert(cmds, check.HasLen, 1)
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
	cmds := s.provisioner.GetCmds(expected, &app)
	c.Assert(cmds, check.HasLen, 1)
}

func (s *S) TestRunWithoutEnv(c *check.C) {
	s.provisioner.PrepareOutput([]byte("a lot of files"))
	app := App{
		Name:      "myapp",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&app, 1, "web", nil)
	var buf bytes.Buffer
	args := provision.RunArgs{Once: false, Isolated: false}
	err = app.run("ls -lh", &buf, args)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "a lot of files")
	cmds := s.provisioner.GetCmds("ls -lh", &app)
	c.Assert(cmds, check.HasLen, 1)
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
	env := app.Envs()
	c.Assert(env, check.DeepEquals, app.Env)
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
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
	}()
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
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
		s.conn.Apps().Remove(bson.M{"name": a3.Name})
	}()
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
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
	}()
	apps, err := List(&Filter{Platform: "ruby"})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
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
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
	}()
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
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
	}()
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
		Lock: AppLock{
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
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
	}()
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
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
	}()
	apps, err := List(nil)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 2)
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
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
		s.conn.Apps().Remove(bson.M{"name": a3.Name})
	}()
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
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
		s.conn.Apps().Remove(bson.M{"name": a3.Name})
	}()
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
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
	}()
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
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
	}()
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
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
	}()
	apps, err := List(&Filter{TeamOwner: "foo"})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
}

func (s *S) TestListFilteringByPool(c *check.C) {
	opts := provision.AddPoolOptions{Name: "test2", Default: false}
	err := provision.AddPool(opts)
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
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
	}()
	apps, err := List(&Filter{Pool: s.Pool})
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].GetName(), check.Equals, a2.Name)
	c.Assert(apps[0].GetPool(), check.Equals, a2.Pool)
}

func (s *S) TestListFilteringByPools(c *check.C) {
	opts := provision.AddPoolOptions{Name: "test2", Default: false}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	opts = provision.AddPoolOptions{Name: "test3", Default: false}
	err = provision.AddPool(opts)
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
			Name:  name,
			Teams: []string{s.team.Name},
			Quota: quota.Quota{
				Limit: 10,
			},
			TeamOwner: s.team.Name,
			Router:    "fake",
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
	proxyUrl, _ := url.Parse("http://somewhere.com")
	err = apps[2].Sleep(&buf, "", proxyUrl)
	c.Assert(err, check.IsNil)
	resultApps, err := List(&Filter{Statuses: []string{"stopped", "asleep"}})
	c.Assert(err, check.IsNil)
	c.Assert(resultApps, check.HasLen, 2)
	names := []string{resultApps[0].Name, resultApps[1].Name}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"ta2", "ta3"})
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
	opts := provision.AddPoolOptions{Name: "test2", Default: false}
	err := provision.AddPool(opts)
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
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
		s.conn.Apps().Remove(bson.M{"name": a3.Name})
	}()
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

func (s *S) TestGetIP(c *check.C) {
	a := App{Ip: "10.10.10.10"}
	c.Assert(a.GetIp(), check.Equals, a.Ip)
}

func (s *S) TestGetQuota(c *check.C) {
	a := App{Quota: quota.Unlimited}
	c.Assert(a.GetQuota(), check.DeepEquals, quota.Unlimited)
}

func (s *S) TestSetQuotaInUse(c *check.C) {
	app := App{Name: "someapp", Quota: quota.Quota{Limit: 5, InUse: 5}}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, check.IsNil)
	err = app.SetQuotaInUse(3)
	c.Assert(err, check.IsNil)
	a, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(a.Quota, check.DeepEquals, quota.Quota{Limit: 5, InUse: 3})
}

func (s *S) TestSetQuotaInUseNotFound(c *check.C) {
	app := App{Name: "someapp", Quota: quota.Quota{Limit: 5, InUse: 5}}
	err := app.SetQuotaInUse(3)
	c.Assert(err, check.Equals, ErrAppNotFound)
}

func (s *S) TestSetQuotaInUseUnlimited(c *check.C) {
	app := App{Name: "someapp", Quota: quota.Unlimited, TeamOwner: s.team.Name}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	err = app.SetQuotaInUse(3)
	c.Assert(err, check.IsNil)
	a, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(a.Quota, check.DeepEquals, quota.Quota{Limit: -1, InUse: 3})

}

func (s *S) TestSetQuotaInUseInvalid(c *check.C) {
	app := App{Name: "someapp", Quota: quota.Quota{Limit: 5, InUse: 3}}
	err := app.SetQuotaInUse(6)
	c.Assert(err, check.NotNil)
	e, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Available, check.Equals, uint(5))
	c.Assert(e.Requested, check.Equals, uint(6))
	err = app.SetQuotaInUse(-1)
	c.Assert(err, check.NotNil)
	c.Check(err.Error(), check.Equals, "invalid value, cannot be lesser than 0")
}

func (s *S) TestGetCname(c *check.C) {
	a := App{CName: []string{"cname1", "cname2"}}
	c.Assert(a.GetCname(), check.DeepEquals, a.CName)
}

func (s *S) TestGetLock(c *check.C) {
	a := App{
		Lock: AppLock{
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
	a := App{Plan: Plan{Memory: 10}}
	c.Assert(a.GetMemory(), check.Equals, a.Plan.Memory)
}

func (s *S) TestGetSwap(c *check.C) {
	a := App{Plan: Plan{Swap: 20}}
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
	app1.Ip, err = s.provisioner.Addr(app1)
	c.Assert(err, check.IsNil)
	oldIp1 := app1.Ip
	app2 := &App{Name: "app2", TeamOwner: s.team.Name}
	err = CreateApp(app2, s.user)
	c.Assert(err, check.IsNil)
	app2.Ip, err = s.provisioner.Addr(app2)
	c.Assert(err, check.IsNil)
	oldIp2 := app2.Ip
	err = Swap(app1, app2, false)
	c.Assert(err, check.IsNil)
	c.Assert(app1.CName, check.IsNil)
	c.Assert(app2.CName, check.DeepEquals, []string{"cname"})
	c.Assert(app1.Ip, check.Equals, oldIp2)
	c.Assert(app2.Ip, check.Equals, oldIp1)
}

func (s *S) TestSwapCnameOnly(c *check.C) {
	app1 := &App{Name: "app1", CName: []string{"app1.cname", "app1.cname2"}, TeamOwner: s.team.Name}
	err := CreateApp(app1, s.user)
	c.Assert(err, check.IsNil)
	app1.Ip, err = s.provisioner.Addr(app1)
	c.Assert(err, check.IsNil)
	oldIp1 := app1.Ip
	app2 := &App{Name: "app2", CName: []string{"app2.cname"}, TeamOwner: s.team.Name}
	err = CreateApp(app2, s.user)
	c.Assert(err, check.IsNil)
	app2.Ip, err = s.provisioner.Addr(app2)
	c.Assert(err, check.IsNil)
	oldIp2 := app2.Ip
	err = Swap(app1, app2, true)
	c.Assert(err, check.IsNil)
	c.Assert(app1.CName, check.DeepEquals, []string{"app2.cname"})
	c.Assert(app2.CName, check.DeepEquals, []string{"app1.cname", "app1.cname2"})
	c.Assert(app1.Ip, check.Equals, oldIp1)
	c.Assert(app2.Ip, check.Equals, oldIp2)
}

func (s *S) TestStart(c *check.C) {
	s.provisioner.PrepareOutput([]byte("not yaml")) // loadConf
	a := App{
		Name:      "someapp",
		Platform:  "django",
		Teams:     []string{s.team.Name},
		TeamOwner: s.team.Name,
		Router:    "fake",
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
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Router: "fake"}
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
	a := App{Name: "my-test-app", Router: "fake", TeamOwner: s.team.Name}
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
		Lock: AppLock{
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

func (s *S) TestAppLockStringUnlocked(c *check.C) {
	lock := AppLock{Locked: false}
	c.Assert(lock.String(), check.Equals, "Not locked")
}

func (s *S) TestAppLockStringLocked(c *check.C) {
	lock := AppLock{
		Locked:      true,
		Reason:      "/app/my-app/deploy",
		Owner:       "someone",
		AcquireDate: time.Date(2048, time.November, 10, 10, 0, 0, 0, time.UTC),
	}
	c.Assert(lock.String(), check.Matches, "App locked by someone, running /app/my-app/deploy. Acquired in 2048-11-10T.*")
}

func (s *S) TestAppLockMarshalJSON(c *check.C) {
	lock := AppLock{
		Locked:      true,
		Reason:      "/app/my-app/deploy",
		Owner:       "someone",
		AcquireDate: time.Date(2048, time.November, 10, 10, 0, 0, 0, time.UTC),
	}
	data, err := lock.MarshalJSON()
	c.Assert(err, check.IsNil)
	var a AppLock
	err = json.Unmarshal(data, &a)
	c.Assert(err, check.IsNil)
	c.Assert(a, check.DeepEquals, lock)
}

func (s *S) TestAppLockGetLocked(c *check.C) {
	lock := AppLock{Locked: true}
	c.Assert(lock.GetLocked(), check.Equals, lock.Locked)
}

func (s *S) TestAppLockGetReason(c *check.C) {
	lock := AppLock{Reason: "/app/my-app/deploy"}
	c.Assert(lock.GetReason(), check.Equals, lock.Reason)
}

func (s *S) TestAppLockGetOwner(c *check.C) {
	lock := AppLock{Owner: "someone"}
	c.Assert(lock.GetOwner(), check.Equals, lock.Owner)
}

func (s *S) TestAppLockGetAcquireDate(c *check.C) {
	lock := AppLock{AcquireDate: time.Date(2048, time.November, 10, 10, 0, 0, 0, time.UTC)}
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
		ips = append(ips, u.Ip)
	}
	customData := map[string]interface{}{"x": "y"}
	err = a.RegisterUnit(units[0].ID, customData)
	c.Assert(err, check.IsNil)
	units, err = a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units[0].Ip, check.Equals, ips[0]+"-updated")
	c.Assert(units[1].Ip, check.Equals, ips[1])
	c.Assert(units[2].Ip, check.Equals, ips[2])
	c.Assert(s.provisioner.CustomData(&a), check.DeepEquals, customData)
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
	team := auth.Team{Name: "test"}
	err := s.conn.Teams().Insert(team)
	defer s.conn.Teams().Remove(bson.M{"_id": team.Name})
	c.Assert(err, check.IsNil)
	a := App{Name: "test", Platform: "python", TeamOwner: team.Name}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
}

func (s *S) TestAppCreateValidateTeamOwnerSetAnTeamWhichNotExists(c *check.C) {
	a := App{Name: "test", Platform: "python", TeamOwner: "not-exists"}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.DeepEquals, &errors.ValidationError{Message: auth.ErrTeamNotFound.Error()})
}

func (s *S) TestAppCreateValidateRouterNotAvailableForPool(c *check.C) {
	provision.SetPoolConstraint(&provision.PoolConstraint{
		PoolExpr:  "pool1",
		Field:     "router",
		Values:    []string{"fake-tls"},
		Blacklist: true,
	})
	a := App{Name: "test", Platform: "python", TeamOwner: s.team.Name, Router: "fake-tls"}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.DeepEquals, &errors.ValidationError{
		Message: "router \"fake-tls\" is not available for pool \"pool1\"",
	})
}

func (s *S) TestAppSetPoolByTeamOwner(c *check.C) {
	opts := provision.AddPoolOptions{Name: "test"}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool("test", []string{"tsuruteam"})
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
	opts := provision.AddPoolOptions{Name: "test", Public: true}
	err := provision.AddPool(opts)
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
	opts := provision.AddPoolOptions{Name: "test"}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool("test", []string{"tsuruteam"})
	c.Assert(err, check.IsNil)
	opts = provision.AddPoolOptions{Name: "pool2"}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool("pool2", []string{"tsuruteam"})
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
	opts := provision.AddPoolOptions{Name: "test"}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool("test", []string{"test"})
	c.Assert(err, check.IsNil)
	opts = provision.AddPoolOptions{Name: "pool2"}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool("pool2", []string{"test"})
	c.Assert(err, check.IsNil)
	app := App{
		Name:      "test",
		TeamOwner: "test",
	}
	err = app.SetPool()
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "you have access to \"test\",\"pool2\" pools. Please choose one in app creation")
}

func (s *S) TestAppSetPoolNoDefault(c *check.C) {
	err := provision.RemovePool("pool1")
	c.Assert(err, check.IsNil)
	opts := provision.AddPoolOptions{Name: "pool1"}
	defer provision.AddPool(opts)
	app := App{
		Name: "test",
	}
	err = app.SetPool()
	c.Assert(err, check.NotNil)
	c.Assert(app.Pool, check.Equals, "")
}

func (s *S) TestAppSetPoolUserDontHaveAccessToPool(c *check.C) {
	opts := provision.AddPoolOptions{Name: "test"}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool("test", []string{"nopool"})
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
	opts := provision.AddPoolOptions{Name: "test", Public: true}
	err := provision.AddPool(opts)
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
	opts := provision.AddPoolOptions{Name: "test", Public: true}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	opts = provision.AddPoolOptions{Name: "nonpublic"}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool("nonpublic", []string{"tsuruteam"})
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

func (s *S) TestShellToAnApp(c *check.C) {
	a := App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	units, err := s.provisioner.Units(&a)
	c.Assert(err, check.IsNil)
	unit := units[0]
	buf := safe.NewBuffer([]byte("echo teste"))
	opts := provision.ShellOptions{
		Conn:   &provisiontest.FakeConn{Buf: buf},
		Width:  200,
		Height: 40,
		Unit:   unit.ID,
		Term:   "xterm",
	}
	err = a.Shell(opts)
	c.Assert(err, check.IsNil)
	expected := []provision.ShellOptions{opts}
	expected[0].App = &a
	c.Assert(s.provisioner.Shells(unit.ID), check.DeepEquals, expected)
}

func (s *S) TestSetCertificateForApp(c *check.C) {
	cname := "app.io"
	cert, err := ioutil.ReadFile("testdata/certificate.crt")
	c.Assert(err, check.IsNil)
	key, err := ioutil.ReadFile("testdata/private.key")
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Router: "fake-tls"}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	a.Ip = "app.io"
	err = s.conn.Apps().Update(bson.M{"name": a.Name}, bson.M{"$set": bson.M{"ip": a.Ip}})
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
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Router: "fake-tls", CName: []string{cname}}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate(cname, string(cert), string(key))
	c.Assert(err, check.IsNil)
	c.Assert(routertest.TLSRouter.Certs[cname], check.Equals, string(cert))
	c.Assert(routertest.TLSRouter.Keys[cname], check.Equals, string(key))
}

func (s *S) TestSetCertificateNonTLSRouter(c *check.C) {
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, CName: []string{"app.io"}}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate("app.io", "cert", "key")
	c.Assert(err, check.ErrorMatches, "router does not support tls")
}

func (s *S) TestSetCertificateInvalidCName(c *check.C) {
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Router: "fake-tls"}
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
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Router: "fake-tls", CName: []string{cname}}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate(cname, string(cert), string(key))
	c.Assert(err, check.ErrorMatches, "*certificate is valid for app.io, not example.io*")
	c.Assert(routertest.TLSRouter.Certs[cname], check.Equals, "")
	c.Assert(routertest.TLSRouter.Keys[cname], check.Equals, "")
}

func (s *S) TestRemoveCertificate(c *check.C) {
	cname := "app.io"
	cert, err := ioutil.ReadFile("testdata/certificate.crt")
	c.Assert(err, check.IsNil)
	key, err := ioutil.ReadFile("testdata/private.key")
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Router: "fake-tls"}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	a.Ip = "app.io"
	err = s.conn.Apps().Update(bson.M{"name": a.Name}, bson.M{"$set": bson.M{"ip": a.Ip}})
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
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Router: "fake-tls", CName: []string{cname}}
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
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Router: "fake-tls", CName: []string{cname}}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate(cname, string(cert), string(key))
	c.Assert(err, check.IsNil)
	expectedCerts := map[string]string{
		"app.io":                     string(cert),
		"my-test-app.fakerouter.com": "",
	}
	certs, err := a.GetCertificates()
	c.Assert(err, check.IsNil)
	c.Assert(certs, check.DeepEquals, expectedCerts)
}

func (s *S) TestGetCertificatesNonTLSRouter(c *check.C) {
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, CName: []string{"app.io"}}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	certs, err := a.GetCertificates()
	c.Assert(err, check.ErrorMatches, "router does not support tls")
	c.Assert(certs, check.IsNil)
}

func (s *S) TestAppMetricEnvs(c *check.C) {
	a := App{Name: "app-name", Platform: "python"}
	envs, err := a.MetricEnvs()
	c.Assert(err, check.IsNil)
	prov, err := a.getProvisioner()
	c.Assert(err, check.IsNil)
	metricProv, ok := prov.(provision.MetricsProvisioner)
	c.Assert(ok, check.Equals, true)
	expected := metricProv.MetricEnvs(&a)
	c.Assert(envs, check.DeepEquals, expected)
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

func (s *S) TestUpdateTeamOwner(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla"}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	team := &auth.Team{Name: "newowner"}
	err = s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": team.Name})
	updateData := App{Name: "example", TeamOwner: "newowner"}
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.TeamOwner, check.Equals, "newowner")
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
	opts := provision.AddPoolOptions{Name: "test"}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool("test", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	opts = provision.AddPoolOptions{Name: "test2"}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool("test2", []string{s.team.Name})
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

func (s *S) TestUpdatePoolNotExists(c *check.C) {
	opts := provision.AddPoolOptions{Name: "test"}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool("test", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	app := App{Name: "test", TeamOwner: s.team.Name, Pool: "test"}
	err = CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "test", Pool: "test2"}
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.Equals, provision.ErrPoolNotFound)
	dbApp, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Pool, check.Equals, "test")
}

func (s *S) TestUpdatePlan(c *check.C) {
	plan := Plan{Name: "something", CpuShare: 100, Memory: 268435456}
	err := s.conn.Plans().Insert(plan)
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", Router: "fake", Plan: Plan{Memory: 536870912, CpuShare: 50}, TeamOwner: s.team.Name}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	c.Assert(routertest.FakeRouter.HasBackend(a.Name), check.Equals, true)
	c.Assert(routertest.HCRouter.HasBackend(a.Name), check.Equals, false)
	updateData := App{Name: "my-test-app", Plan: Plan{Name: "something"}}
	err = a.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Plan, check.DeepEquals, plan)
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 1)
}

func (s *S) TestUpdatePlanNoRouteChange(c *check.C) {
	plan := Plan{Name: "something", CpuShare: 100, Memory: 268435456}
	err := s.conn.Plans().Insert(plan)
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", Router: "fake", Plan: Plan{Memory: 536870912, CpuShare: 50}, TeamOwner: s.team.Name}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	c.Assert(routertest.FakeRouter.HasBackend(a.Name), check.Equals, true)
	updateData := App{Name: "my-test-app", Plan: Plan{Name: "something"}}
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
	updateData := App{Name: "my-test-app", Plan: Plan{Name: "some-unknown-plan"}}
	err := app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.Equals, ErrPlanNotFound)
}

func (s *S) TestUpdateRouterBackendRemovalFailure(c *check.C) {
	plan := Plan{Name: "something", CpuShare: 100, Memory: 268435456}
	err := s.conn.Plans().Insert(plan)
	c.Assert(err, check.IsNil)
	plan = Plan{Name: "wrong", CpuShare: 50, Memory: 536870912}
	err = s.conn.Plans().Insert(plan)
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", Router: "fake", Plan: Plan{Name: "wrong"}, TeamOwner: s.team.Name}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	c.Assert(routertest.FakeRouter.HasBackend(a.Name), check.Equals, true)
	c.Assert(routertest.HCRouter.HasBackend(a.Name), check.Equals, false)
	routertest.FakeRouter.FailForIp("my-test-app")
	updateData := App{Name: "my-test-app", Router: "fake-hc", Plan: Plan{Name: "something"}}
	err = a.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Plan.Name, check.Equals, "something")
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 1)
	// Yeah, a test-ensured inconsistency.
	c.Assert(routertest.FakeRouter.HasBackend(dbApp.Name), check.Equals, true)
	c.Assert(routertest.HCRouter.HasBackend(dbApp.Name), check.Equals, true)
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

func (s *S) TestUpdatePlanRestartFailure(c *check.C) {
	plan := Plan{Name: "something", CpuShare: 100, Memory: 268435456}
	err := s.conn.Plans().Insert(plan)
	c.Assert(err, check.IsNil)
	plan = Plan{Name: "old", CpuShare: 50, Memory: 536870912}
	err = s.conn.Plans().Insert(plan)
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", Router: "fake", Plan: Plan{Name: "old"}, TeamOwner: s.team.Name}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	c.Assert(routertest.FakeRouter.HasBackend(a.Name), check.Equals, true)
	c.Assert(routertest.HCRouter.HasBackend(a.Name), check.Equals, false)
	a.Ip = "old-address"
	err = s.conn.Apps().Update(bson.M{"name": a.Name}, bson.M{"$set": bson.M{"ip": a.Ip}})
	c.Assert(err, check.IsNil)
	s.provisioner.PrepareFailure("Restart", fmt.Errorf("cannot restart app, I'm sorry"))
	updateData := App{Name: "my-test-app", Router: "fake-hc", Plan: Plan{Name: "something"}}
	err = a.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.NotNil)
	dbApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Plan.Name, check.Equals, "old")
	c.Assert(dbApp.Ip, check.Equals, "old-address")
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

func (s *S) TestUpdateTags(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla", Tags: []string{"tag1"}}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	newTags := []string{"tag2", "tag3"}
	updateData := App{Tags: newTags}
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Tags, check.DeepEquals, newTags)
}

func (s *S) TestUpdateWithoutNewTags(c *check.C) {
	app := App{Name: "example", Platform: "python", TeamOwner: s.team.Name, Description: "blabla", Tags: []string{"tag1"}}
	err := CreateApp(&app, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Description: "ble"}
	err = app.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Tags, check.DeepEquals, []string{"tag1"})
}

func (s *S) TestUpdateDescriptionPoolPlanAndRouter(c *check.C) {
	opts := provision.AddPoolOptions{Name: "test"}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool("test", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	opts = provision.AddPoolOptions{Name: "test2"}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool("test2", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	plan := Plan{Name: "something", CpuShare: 100, Memory: 268435456}
	err = s.conn.Plans().Insert(plan)
	c.Assert(err, check.IsNil)
	a := App{Name: "my-test-app", TeamOwner: s.team.Name, Router: "fake", Plan: Plan{Memory: 536870912, CpuShare: 50}, Description: "blablabla", Pool: "test"}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	c.Assert(routertest.FakeRouter.HasBackend(a.Name), check.Equals, true)
	c.Assert(routertest.HCRouter.HasBackend(a.Name), check.Equals, false)
	updateData := App{Name: "my-test-app", Router: "fake-hc", Plan: Plan{Name: "something"}, Description: "bleble", Pool: "test2"}
	err = a.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Plan, check.DeepEquals, plan)
	c.Assert(dbApp.Description, check.Equals, "bleble")
	c.Assert(dbApp.Pool, check.Equals, "test2")
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 1)
	c.Assert(routertest.FakeRouter.HasBackend(dbApp.Name), check.Equals, false)
	c.Assert(routertest.HCRouter.HasBackend(dbApp.Name), check.Equals, true)
	routes, err := routertest.HCRouter.Routes(dbApp.Name)
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

func (s *S) TestUpdateRouter(c *check.C) {
	a := App{Name: "my-test-app", Router: "fake", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "my-test-app", Router: "fake-hc"}
	err = a.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.IsNil)
	dbApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Router, check.Equals, "fake-hc")
	c.Assert(s.provisioner.Restarts(dbApp, ""), check.Equals, 1)
	c.Assert(routertest.FakeRouter.HasBackend(dbApp.Name), check.Equals, false)
	c.Assert(routertest.HCRouter.HasBackend(dbApp.Name), check.Equals, true)
	routes, err := routertest.HCRouter.Routes(dbApp.Name)
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

func (s *S) TestUpdateRouterNotFound(c *check.C) {
	a := App{Name: "my-test-app", Router: "fake", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "my-test-app", Router: "invalid-router"}
	err = a.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.DeepEquals, &router.ErrRouterNotFound{Name: "invalid-router"})
	dbApp, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Router, check.Equals, "fake")
}

func (s *S) TestAppUpdateRouterNotAvailableForPool(c *check.C) {
	provision.SetPoolConstraint(&provision.PoolConstraint{
		PoolExpr:  "pool1",
		Field:     "router",
		Values:    []string{"fake-tls"},
		Blacklist: true,
	})
	a := App{Name: "test", Router: "fake", TeamOwner: s.team.Name}
	err := CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	updateData := App{Name: "test", Router: "fake-tls"}
	err = a.Update(updateData, new(bytes.Buffer))
	c.Assert(err, check.DeepEquals, &errors.ValidationError{
		Message: "router \"fake-tls\" is not available for pool \"pool1\"",
	})
}
