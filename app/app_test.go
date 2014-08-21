// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"encoding/json"
	stderr "errors"
	"fmt"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/testing"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func (s *S) TestGetAppByName(c *gocheck.C) {
	newApp := App{Name: "myApp", Platform: "Django"}
	err := s.conn.Apps().Insert(newApp)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": newApp.Name})
	newApp.Env = map[string]bind.EnvVar{}
	err = s.conn.Apps().Update(bson.M{"name": newApp.Name}, &newApp)
	c.Assert(err, gocheck.IsNil)
	myApp, err := GetByName("myApp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(myApp.Name, gocheck.Equals, newApp.Name)
}

func (s *S) TestGetAppByNameNotFound(c *gocheck.C) {
	app, err := GetByName("wat")
	c.Assert(err, gocheck.Equals, ErrAppNotFound)
	c.Assert(app, gocheck.IsNil)
}

func (s *S) TestDelete(c *gocheck.C) {
	s.conn.Users().Update(
		bson.M{"email": s.user.Email},
		bson.M{"$set": bson.M{"quota.limit": 1, "quota.inuse": 1}},
	)
	defer s.conn.Users().Update(
		bson.M{"email": s.user.Email},
		bson.M{"$set": bson.M{"quota": quota.Unlimited}},
	)
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	a := App{
		Name:     "ritual",
		Platform: "ruby",
		Owner:    s.user.Email,
	}
	err := s.conn.Apps().Insert(&a)
	c.Assert(err, gocheck.IsNil)
	app, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	err = app.Log("msg", "src", "unit")
	c.Assert(err, gocheck.IsNil)
	err = Delete(app)
	time.Sleep(200 * time.Millisecond)
	c.Assert(err, gocheck.IsNil)
	_, err = GetByName(app.Name)
	c.Assert(err, gocheck.NotNil)
	c.Assert(s.provisioner.Provisioned(&a), gocheck.Equals, false)
	err = auth.ReserveApp(s.user)
	c.Assert(err, gocheck.IsNil)
	count, err := s.conn.Logs(app.Name).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 0)
}

func (s *S) TestDestroy(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	a := App{
		Name:     "ritual",
		Platform: "python",
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, gocheck.IsNil)
	app, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	token := app.Env["TSURU_APP_TOKEN"].Value
	err = Delete(app)
	time.Sleep(200 * time.Millisecond)
	c.Assert(err, gocheck.IsNil)
	_, err = GetByName(app.Name)
	c.Assert(err, gocheck.NotNil)
	c.Assert(s.provisioner.Provisioned(&a), gocheck.Equals, false)
	_, err = nativeScheme.Auth(token)
	c.Assert(err, gocheck.Equals, auth.ErrInvalidToken)
}

func (s *S) TestDestroyWithoutUnits(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{Name: "x4", Platform: "python"}
	err := CreateApp(&app, s.user)
	c.Assert(err, gocheck.IsNil)
	defer s.provisioner.Destroy(&app)
	a, err := GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	err = Delete(a)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestCreateApp(c *gocheck.C) {
	ts := testing.StartGandalfTestServer(&testHandler{})
	defer ts.Close()
	a := App{
		Name:     "appname",
		Platform: "python",
		Memory:   15,
		Swap:     64,
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": 1}})
	defer s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": -1}})
	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err := CreateApp(&a, s.user)
	c.Assert(err, gocheck.IsNil)
	defer Delete(&a)
	retrievedApp, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(retrievedApp.Name, gocheck.Equals, a.Name)
	c.Assert(retrievedApp.Platform, gocheck.Equals, a.Platform)
	c.Assert(retrievedApp.Teams, gocheck.DeepEquals, []string{s.team.Name})
	c.Assert(retrievedApp.Owner, gocheck.Equals, s.user.Email)
	c.Assert(retrievedApp.Memory, gocheck.Equals, a.Memory)
	env := retrievedApp.InstanceEnv("")
	c.Assert(env["TSURU_APPNAME"].Value, gocheck.Equals, a.Name)
	c.Assert(env["TSURU_APPNAME"].Public, gocheck.Equals, false)
	c.Assert(env["TSURU_HOST"].Value, gocheck.Equals, expectedHost)
	c.Assert(env["TSURU_HOST"].Public, gocheck.Equals, false)
	err = auth.ReserveApp(s.user)
	_, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestCreateAppDefaultMemory(c *gocheck.C) {
	config.Set("docker:memory", 10)
	ts := testing.StartGandalfTestServer(&testHandler{})
	defer ts.Close()
	a := App{
		Name:     "appname",
		Platform: "python",
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": 1}})
	defer s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": -1}})
	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err := CreateApp(&a, s.user)
	c.Assert(err, gocheck.IsNil)
	defer Delete(&a)
	retrievedApp, err := GetByName(a.Name)
	c.Assert(retrievedApp.Memory, gocheck.Equals, 10)
}

func (s *S) TestCreateAppDefaultSwap(c *gocheck.C) {
	config.Set("docker:swap", 32)
	defer config.Unset("docker:swap")
	config.Set("docker:memory", 10)
	defer config.Unset("docker:memory")
	ts := testing.StartGandalfTestServer(&testHandler{})
	defer ts.Close()
	a := App{
		Name:     "appname",
		Platform: "python",
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": 1}})
	defer s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": -1}})
	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err := CreateApp(&a, s.user)
	c.Assert(err, gocheck.IsNil)
	defer Delete(&a)
	retrievedApp, err := GetByName(a.Name)
	c.Assert(retrievedApp.Swap, gocheck.Equals, 42)
}

func (s *S) TestCreateAppWithoutDefault(c *gocheck.C) {
	config.Unset("docker:memory")
	config.Unset("docker:swap")
	ts := testing.StartGandalfTestServer(&testHandler{})
	defer ts.Close()
	a := App{
		Name:     "appname",
		Platform: "python",
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": 1}})
	defer s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": -1}})
	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err := CreateApp(&a, s.user)
	c.Assert(err, gocheck.IsNil)
	defer Delete(&a)
	retrievedApp, err := GetByName(a.Name)
	c.Assert(retrievedApp.Memory, gocheck.Equals, 0)
	c.Assert(retrievedApp.Swap, gocheck.Equals, 0)
}

func (s *S) TestCreateAppMemoryFromPlatform(c *gocheck.C) {
	config.Set("docker:memory", 128)
	defer config.Unset("docker:memory")
	ts := testing.StartGandalfTestServer(&testHandler{})
	defer ts.Close()
	platform := Platform{
		Name:   "python-limited",
		Config: PlatformConfig{Memory: 1073741824},
	}
	s.conn.Platforms().Insert(platform)
	defer s.conn.Platforms().Remove(bson.M{"_id": platform.Name})
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	app := App{Name: "myapp", Platform: "python-limited"}
	err := CreateApp(&app, s.user)
	c.Assert(err, gocheck.IsNil)
	defer Delete(&app)
	retrievedApp, err := GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(retrievedApp.Memory, gocheck.Equals, int(platform.Config.Memory))
	c.Assert(retrievedApp.Swap, gocheck.Equals, 0)
}

func (s *S) TestCreateAppSwapFromPlatform(c *gocheck.C) {
	config.Set("docker:swap", 128)
	defer config.Unset("docker:swap")
	ts := testing.StartGandalfTestServer(&testHandler{})
	defer ts.Close()
	platform := Platform{
		Name:   "python-limited",
		Config: PlatformConfig{VirtualMemory: 1073741824},
	}
	s.conn.Platforms().Insert(platform)
	defer s.conn.Platforms().Remove(bson.M{"_id": platform.Name})
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	app := App{Name: "myapp", Platform: "python-limited"}
	err := CreateApp(&app, s.user)
	c.Assert(err, gocheck.IsNil)
	defer Delete(&app)
	retrievedApp, err := GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(retrievedApp.Memory, gocheck.Equals, 0)
	c.Assert(retrievedApp.Swap, gocheck.Equals, int(platform.Config.VirtualMemory))
}

func (s *S) TestCreateAppUserQuotaExceeded(c *gocheck.C) {
	app := App{Name: "america", Platform: "python"}
	s.conn.Users().Update(
		bson.M{"email": s.user.Email},
		bson.M{"$set": bson.M{"quota.limit": 0}},
	)
	defer s.conn.Users().Update(
		bson.M{"email": s.user.Email},
		bson.M{"$set": bson.M{"quota.limit": -1}},
	)
	err := CreateApp(&app, s.user)
	e, ok := err.(*AppCreationError)
	c.Assert(ok, gocheck.Equals, true)
	_, ok = e.Err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestCreateAppTeamOwner(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{Name: "america", Platform: "python", TeamOwner: "tsuruteam"}
	err := CreateApp(&app, s.user)
	c.Check(err, gocheck.IsNil)
	defer Delete(&app)
}

func (s *S) TestCreateAppTeamOwnerOneTeam(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{Name: "america", Platform: "python"}
	err := CreateApp(&app, s.user)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.TeamOwner, gocheck.Equals, "tsuruteam")
	defer Delete(&app)
}

func (s *S) TestCreateAppTeamOwnerMoreTeamShouldReturnError(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{Name: "america", Platform: "python"}
	team := auth.Team{Name: "tsurutwo", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(team)
	c.Check(err, gocheck.IsNil)
	defer s.conn.Teams().RemoveId(team.Name)
	err = CreateApp(&app, s.user)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.FitsTypeOf, ManyTeamsError{})
}

func (s *S) TestCreateAppTeamOwnerTeamNotFound(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{
		Name:      "someapp",
		Platform:  "python",
		TeamOwner: "not found",
	}
	err := CreateApp(&app, s.user)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, ".*team not found.")
}

func (s *S) TestCannotCreateAppWithUnknownPlatform(c *gocheck.C) {
	a := App{Name: "paradisum", Platform: "unknown"}
	err := CreateApp(&a, s.user)
	_, ok := err.(InvalidPlatformError)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestCannotCreateAppWithoutTeams(c *gocheck.C) {
	u := auth.User{Email: "perpetual@yes.com"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	a := App{Name: "beyond"}
	err = CreateApp(&a, &u)
	c.Check(err, gocheck.NotNil)
	_, ok := err.(NoTeamsError)
	c.Check(ok, gocheck.Equals, true)
}

func (s *S) TestCantCreateTwoAppsWithTheSameName(c *gocheck.C) {
	err := s.conn.Apps().Insert(bson.M{"name": "appname"})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": "appname"})
	a := App{Name: "appname", Platform: "python"}
	err = CreateApp(&a, s.user)
	defer Delete(&a) // clean mess if test fail
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*AppCreationError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.app, gocheck.Equals, "appname")
	c.Assert(e.Err, gocheck.NotNil)
	c.Assert(e.Err.Error(), gocheck.Equals, "there is already an app with this name")
}

func (s *S) TestCantCreateAppWithInvalidName(c *gocheck.C) {
	a := App{
		Name:     "1123app",
		Platform: "python",
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.ValidationError)
	c.Assert(ok, gocheck.Equals, true)
	msg := "Invalid app name, your app should have at most 63 " +
		"characters, containing only lower case letters, numbers or dashes, " +
		"starting with a letter."
	c.Assert(e.Message, gocheck.Equals, msg)
}

func (s *S) TestCantCreateAppWithSamePlatformName(c *gocheck.C) {
	a := App{Name: "python", Platform: "python"}
	err := CreateApp(&a, s.user)
	e, ok := err.(*errors.ValidationError)
	c.Assert(ok, gocheck.Equals, true)
	msg := "Invalid app name: platform name and app name can not be the same"
	c.Assert(e.Message, gocheck.Equals, msg)
}

func (s *S) TestCantCreateAppWhenEqualToSomePlatformName(c *gocheck.C) {
	a := App{Name: "django", Platform: "python"}
	platforms := []Platform{
		{Name: "django"},
	}
	conn, _ := db.Conn()
	defer conn.Close()
	for _, p := range platforms {
		conn.Platforms().Insert(p)
		defer conn.Platforms().Remove(p)
	}
	err := CreateApp(&a, s.user)
	e, ok := err.(*errors.ValidationError)
	c.Assert(ok, gocheck.Equals, true)
	msg := "Invalid app name: platform name already exists " +
		"with the same name"
	c.Assert(e.Message, gocheck.Equals, msg)
}

func (s *S) TestDoesNotSaveTheAppInTheDatabaseIfProvisionerFail(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	s.provisioner.PrepareFailure("Provision", stderr.New("exit status 1"))
	a := App{
		Name:     "theirapp",
		Platform: "python",
	}
	err := CreateApp(&a, s.user)
	defer Delete(&a) // clean mess if test fail
	c.Assert(err, gocheck.NotNil)
	expected := `tsuru failed to create the app "theirapp": exit status 1`
	c.Assert(err.Error(), gocheck.Equals, expected)
	_, err = GetByName(a.Name)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestCreateAppCreatesRepositoryInGandalf(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	a := App{
		Name:     "someapp",
		Platform: "python",
		Teams:    []string{s.team.Name},
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, gocheck.IsNil)
	app, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	defer Delete(app)
	c.Assert(h.url[0], gocheck.Equals, "/repository")
	c.Assert(h.method[0], gocheck.Equals, "POST")
	expected := fmt.Sprintf(`{"name":"someapp","users":["%s"],"ispublic":false}`, s.user.Email)
	c.Assert(string(h.body[0]), gocheck.Equals, expected)
}

func (s *S) TestCreateAppDoesNotSaveTheAppWhenGandalfFailstoCreateTheRepository(c *gocheck.C) {
	ts := testing.StartGandalfTestServer(&testBadHandler{msg: "could not create the repository"})
	defer ts.Close()
	a := App{Name: "otherapp", Platform: "python"}
	err := CreateApp(&a, s.user)
	c.Assert(err, gocheck.NotNil)
	count, err := s.conn.Apps().Find(bson.M{"name": a.Name}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 0)
}

func (s *S) TestAddUnits(c *gocheck.C) {
	app := App{
		Name: "warpaint", Platform: "python",
		Quota: quota.Unlimited,
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	callCount := 0
	rollback := s.addServiceInstance(c, app.Name, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	})
	defer rollback()
	err = app.AddUnits(5)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Units(), gocheck.HasLen, 5)
	err = app.AddUnits(2)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Units(), gocheck.HasLen, 7)
	for _, unit := range app.Units() {
		c.Assert(unit.AppName, gocheck.Equals, app.Name)
	}
	c.Assert(callCount, gocheck.Equals, 7)
}

func (s *S) TestAddUnitsQuota(c *gocheck.C) {
	app := App{
		Name: "warpaint", Platform: "python",
		Quota: quota.Quota{Limit: 7},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	otherApp := App{Name: "warpaint"}
	err = otherApp.AddUnits(5)
	c.Assert(err, gocheck.IsNil)
	units := s.provisioner.GetUnits(&app)
	c.Assert(units, gocheck.HasLen, 5)
	err = otherApp.AddUnits(2)
	c.Assert(err, gocheck.IsNil)
	units = s.provisioner.GetUnits(&app)
	c.Assert(units, gocheck.HasLen, 7)
	err = reserveUnits(&app, 1)
	_, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestAddUnitsQuotaExceeded(c *gocheck.C) {
	app := App{Name: "warpaint", Platform: "ruby"}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	err := app.AddUnits(1)
	e, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Available, gocheck.Equals, uint(0))
	c.Assert(e.Requested, gocheck.Equals, uint(1))
	units := s.provisioner.GetUnits(&app)
	c.Assert(units, gocheck.HasLen, 0)
}

func (s *S) TestAddUnitsMultiple(c *gocheck.C) {
	app := App{
		Name: "warpaint", Platform: "ruby",
		Quota: quota.Quota{Limit: 10},
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	err := app.AddUnits(11)
	e, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Available, gocheck.Equals, uint(10))
	c.Assert(e.Requested, gocheck.Equals, uint(11))
}

func (s *S) TestAddZeroUnits(c *gocheck.C) {
	app := App{Name: "warpaint", Platform: "ruby"}
	err := app.AddUnits(0)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Cannot add zero units.")
}

func (s *S) TestAddUnitsFailureInProvisioner(c *gocheck.C) {
	app := App{Name: "scars", Platform: "golang", Quota: quota.Unlimited}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	err := app.AddUnits(2)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "App is not provisioned.")
}

func (s *S) TestAddUnitsIsAtomic(c *gocheck.C) {
	app := App{
		Name: "warpaint", Platform: "golang",
		Quota: quota.Unlimited,
	}
	err := app.AddUnits(2)
	c.Assert(err, gocheck.NotNil)
	_, err = GetByName(app.Name)
	c.Assert(err, gocheck.Equals, ErrAppNotFound)
}

func (s *S) TestRemoveUnitsWithQuota(c *gocheck.C) {
	a := App{
		Name:  "ble",
		Quota: quota.Quota{Limit: 6, InUse: 6},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 5)
	defer s.provisioner.Destroy(&a)
	err = a.RemoveUnits(4)
	c.Assert(err, gocheck.IsNil)
	time.Sleep(1e9)
	app, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Quota.InUse, gocheck.Equals, 1)
}

func (s *S) TestRemoveUnits(c *gocheck.C) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	app := App{
		Name:     "chemistry",
		Platform: "python",
		Quota:    quota.Unlimited,
	}
	instance := service.ServiceInstance{
		Name:        "my-inst",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{app.Name},
	}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-inst"})
	err = s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	c.Assert(err, gocheck.IsNil)
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	app.AddUnits(4)
	err = app.RemoveUnits(2)
	c.Assert(err, gocheck.IsNil)
	time.Sleep(1e9)
	ts.Close()
	units := app.Units()
	c.Assert(units, gocheck.HasLen, 2)
	gotApp, err := GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	gotApp, err = GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(gotApp.Quota.InUse, gocheck.Equals, 2)
}

func (s *S) TestRemoveUnitsInvalidValues(c *gocheck.C) {
	var tests = []struct {
		n        uint
		expected string
	}{
		{0, "Cannot remove zero units."},
		{3, "Cannot remove all units from an app."},
		{4, "Cannot remove 4 units from this app, it has only 3 units."},
	}
	app := App{
		Name:     "chemistryii",
		Platform: "python",
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	s.provisioner.AddUnits(&app, 3)
	for _, test := range tests {
		err := app.RemoveUnits(test.n)
		c.Check(err, gocheck.NotNil)
		c.Check(err.Error(), gocheck.Equals, test.expected)
	}
}

func (s *S) TestSetUnitStatus(c *gocheck.C) {
	a := App{Name: "appName", Platform: "python"}
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 3)
	units := a.Units()
	err := a.SetUnitStatus(units[0].Name, provision.StatusError)
	c.Assert(err, gocheck.IsNil)
	units = a.Units()
	c.Assert(units[0].Status, gocheck.Equals, provision.StatusError)
}

func (s *S) TestSetUnitStatusPartialID(c *gocheck.C) {
	a := App{Name: "appName", Platform: "python"}
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 3)
	units := a.Units()
	name := units[0].Name
	err := a.SetUnitStatus(name[0:len(name)-2], provision.StatusError)
	c.Assert(err, gocheck.IsNil)
	units = a.Units()
	c.Assert(units[0].Status, gocheck.Equals, provision.StatusError)
}

func (s *S) TestSetUnitStatusNotFound(c *gocheck.C) {
	a := App{Name: "appName", Platform: "django"}
	err := a.SetUnitStatus("someunit", provision.StatusError)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "unit not found")
}

func (s *S) TestGrantAccess(c *gocheck.C) {
	a := App{Name: "appName", Platform: "django", Teams: []string{}}
	err := a.Grant(&s.team)
	c.Assert(err, gocheck.IsNil)
	_, found := a.find(&s.team)
	c.Assert(found, gocheck.Equals, true)
}

func (s *S) TestGrantAccessKeepTeamsSorted(c *gocheck.C) {
	a := App{Name: "appName", Platform: "django", Teams: []string{"acid-rain", "zito"}}
	err := a.Grant(&s.team)
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.Teams, gocheck.DeepEquals, []string{"acid-rain", s.team.Name, "zito"})
}

func (s *S) TestGrantAccessFailsIfTheTeamAlreadyHasAccessToTheApp(c *gocheck.C) {
	a := App{Name: "appName", Platform: "django", Teams: []string{s.team.Name}}
	err := a.Grant(&s.team)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^This team already has access to this app$")
}

func (s *S) TestRevokeAccess(c *gocheck.C) {
	a := App{Name: "appName", Platform: "django", Teams: []string{s.team.Name}}
	err := a.Revoke(&s.team)
	c.Assert(err, gocheck.IsNil)
	_, found := a.find(&s.team)
	c.Assert(found, gocheck.Equals, false)
}

func (s *S) TestRevoke(c *gocheck.C) {
	a := App{Name: "test", Teams: []string{"team1", "team2", "team3", "team4"}}
	err := a.Revoke(&auth.Team{Name: "team2"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.Teams, gocheck.DeepEquals, []string{"team1", "team3", "team4"})
	err = a.Revoke(&auth.Team{Name: "team4"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.Teams, gocheck.DeepEquals, []string{"team1", "team3"})
	err = a.Revoke(&auth.Team{Name: "team1"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.Teams, gocheck.DeepEquals, []string{"team3"})
}

func (s *S) TestRevokeAccessFailsIfTheTeamsDoesNotHaveAccessToTheApp(c *gocheck.C) {
	a := App{Name: "appName", Platform: "django", Teams: []string{}}
	err := a.Revoke(&s.team)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "^This team does not have access to this app$")
}

func (s *S) TestSetEnvNewAppsTheMapIfItIsNil(c *gocheck.C) {
	a := App{Name: "how-many-more-times"}
	c.Assert(a.Env, gocheck.IsNil)
	env := bind.EnvVar{Name: "PATH", Value: "/"}
	a.setEnv(env)
	c.Assert(a.Env, gocheck.NotNil)
}

func (s *S) TestSetEnvironmentVariableToApp(c *gocheck.C) {
	a := App{Name: "appName", Platform: "django"}
	a.setEnv(bind.EnvVar{Name: "PATH", Value: "/", Public: true})
	env := a.Env["PATH"]
	c.Assert(env.Name, gocheck.Equals, "PATH")
	c.Assert(env.Value, gocheck.Equals, "/")
	c.Assert(env.Public, gocheck.Equals, true)
}

func (s *S) TestSetEnvRespectsThePublicOnlyFlagKeepPrivateVariablesWhenItsTrue(c *gocheck.C) {
	a := App{
		Name: "myapp",
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST": {
				Name:   "DATABASE_HOST",
				Value:  "localhost",
				Public: false,
			},
		},
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
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
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	err = a.setEnvsToApp(envs, true, true)
	c.Assert(err, gocheck.IsNil)
	newApp, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
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
	c.Assert(newApp.Env, gocheck.DeepEquals, expected)
	c.Assert(s.provisioner.Restarts(&a), gocheck.Equals, 1)
}

func (s *S) TestSetEnvRespectsThePublicOnlyFlagOverwrittenAllVariablesWhenItsFalse(c *gocheck.C) {
	a := App{
		Name: "myapp",
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST": {
				Name:   "DATABASE_HOST",
				Value:  "localhost",
				Public: false,
			},
		},
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
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
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	err = a.setEnvsToApp(envs, false, true)
	c.Assert(err, gocheck.IsNil)
	newApp, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
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
	c.Assert(newApp.Env, gocheck.DeepEquals, expected)
	c.Assert(s.provisioner.Restarts(&a), gocheck.Equals, 1)
}

func (s *S) TestUnsetEnvRespectsThePublicOnlyFlagKeepPrivateVariablesWhenItsTrue(c *gocheck.C) {
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
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	err = a.UnsetEnvs([]string{"DATABASE_HOST", "DATABASE_PASSWORD"}, true)
	c.Assert(err, gocheck.IsNil)
	newApp, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:   "DATABASE_HOST",
			Value:  "localhost",
			Public: false,
		},
	}
	c.Assert(newApp.Env, gocheck.DeepEquals, expected)
	c.Assert(s.provisioner.Restarts(&a), gocheck.Equals, 1)
}

func (s *S) TestUnsetEnvRespectsThePublicOnlyFlagUnsettingAllVariablesWhenItsFalse(c *gocheck.C) {
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
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	err = a.UnsetEnvs([]string{"DATABASE_HOST", "DATABASE_PASSWORD"}, false)
	c.Assert(err, gocheck.IsNil)
	newApp, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(newApp.Env, gocheck.DeepEquals, map[string]bind.EnvVar{})
	c.Assert(s.provisioner.Restarts(&a), gocheck.Equals, 1)
}

func (s *S) TestGetEnvironmentVariableFromApp(c *gocheck.C) {
	a := App{Name: "whole-lotta-love"}
	a.setEnv(bind.EnvVar{Name: "PATH", Value: "/"})
	v, err := a.getEnv("PATH")
	c.Assert(err, gocheck.IsNil)
	c.Assert(v.Value, gocheck.Equals, "/")
}

func (s *S) TestGetEnvReturnsErrorIfTheVariableIsNotDeclared(c *gocheck.C) {
	a := App{Name: "what-is-and-what-should-never"}
	a.Env = make(map[string]bind.EnvVar)
	_, err := a.getEnv("PATH")
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestGetEnvReturnsErrorIfTheEnvironmentMapIsNil(c *gocheck.C) {
	a := App{Name: "what-is-and-what-should-never"}
	_, err := a.getEnv("PATH")
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestInstanceEnvironmentReturnEnvironmentVariablesForTheServer(c *gocheck.C) {
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
	c.Assert(a.InstanceEnv("mysql"), gocheck.DeepEquals, expected)
}

func (s *S) TestInstanceEnvironmentDoesNotPanicIfTheEnvMapIsNil(c *gocheck.C) {
	a := App{Name: "hi-there"}
	c.Assert(a.InstanceEnv("mysql"), gocheck.DeepEquals, map[string]bind.EnvVar{})
}

func (s *S) TestSetCName(c *gocheck.C) {
	app := &App{Name: "ktulu"}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(app)
	defer s.provisioner.Destroy(app)
	err = app.SetCName("ktulu.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	app, err = GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.CName, gocheck.Equals, "ktulu.mycompany.com")
}

func (s *S) TestSetCNameWithWildCard(c *gocheck.C) {
	app := &App{Name: "ktulu"}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(app)
	defer s.provisioner.Destroy(app)
	err = app.SetCName("*.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	app, err = GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.CName, gocheck.Equals, "*.mycompany.com")
}

func (s *S) TestSetCNameErrsOnInvalid(c *gocheck.C) {
	app := &App{Name: "ktulu"}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(app)
	defer s.provisioner.Destroy(app)
	err = app.SetCName("_ktulu.mycompany.com")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Invalid cname")
}

func (s *S) TestSetCNamePartialUpdate(c *gocheck.C) {
	a := &App{Name: "master", Platform: "puppet"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(a)
	defer s.provisioner.Destroy(a)
	other := App{Name: a.Name}
	err = other.SetCName("ktulu.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(a.Platform, gocheck.Equals, "puppet")
	c.Assert(a.Name, gocheck.Equals, "master")
	c.Assert(a.CName, gocheck.Equals, "ktulu.mycompany.com")
}

func (s *S) TestSetCNameUnknownApp(c *gocheck.C) {
	a := App{Name: "ktulu"}
	err := a.SetCName("ktulu.mycompany.com")
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestSetCNameValidatesTheCName(c *gocheck.C) {
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
		{"", true},
	}
	a := App{Name: "live-to-die"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	for _, t := range data {
		err := a.SetCName(t.input)
		if !t.valid {
			c.Check(err.Error(), gocheck.Equals, "Invalid cname")
		} else {
			c.Check(err, gocheck.IsNil)
		}
	}
}

func (s *S) TestSetCNameCallsProvisionerSetCName(c *gocheck.C) {
	a := App{Name: "ktulu"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	err = a.SetCName("ktulu.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	hasCName := s.provisioner.HasCName(&a, "ktulu.mycompany.com")
	c.Assert(hasCName, gocheck.Equals, true)
}

func (s *S) TestUnsetCNameRemovesFromDatabase(c *gocheck.C) {
	a := &App{Name: "ktulu"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(a)
	defer s.provisioner.Destroy(a)
	err = a.SetCName("ktulu.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	err = a.UnsetCName()
	c.Assert(err, gocheck.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.CName, gocheck.Equals, "")
}

func (s *S) TestUnsetCNameRemovesFromRouter(c *gocheck.C) {
	a := App{Name: "ktulu"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	err = a.SetCName("ktulu.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	err = a.UnsetCName()
	c.Assert(err, gocheck.IsNil)
	hasCName := s.provisioner.HasCName(&a, "ktulu.mycompany.com")
	c.Assert(hasCName, gocheck.Equals, false)
}

func (s *S) TestIsValid(c *gocheck.C) {
	var data = []struct {
		name     string
		expected bool
	}{
		{"myappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyapp", false},
		{"myappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyap", false},
		{"myappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmya", true},
		{"myApp", false},
		{"my app", false},
		{"123myapp", false},
		{"myapp", true},
		{"_theirapp", false},
		{"my-app", true},
		{"-myapp", false},
		{"my_app", false},
		{"b", true},
		{InternalAppName, false},
	}
	for _, d := range data {
		a := App{Name: d.name}
		if valid := a.isValid(); valid != d.expected {
			c.Errorf("Is %q a valid app name? Expected: %v. Got: %v.", d.name, d.expected, valid)
		}
	}
}

func (s *S) TestReady(c *gocheck.C) {
	a := App{Name: "twisted"}
	s.conn.Apps().Insert(a)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err := a.Ready()
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.State, gocheck.Equals, "ready")
	other, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(other.State, gocheck.Equals, "ready")
}

func (s *S) TestRestart(c *gocheck.C) {
	s.provisioner.PrepareOutput([]byte("not yaml")) // loadConf
	a := App{
		Name:     "someApp",
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	var b bytes.Buffer
	err := a.Restart(&b)
	c.Assert(err, gocheck.IsNil)
	result := strings.Replace(b.String(), "\n", "#", -1)
	c.Assert(result, gocheck.Matches, ".*# ---> Restarting your app#.*")
	restarts := s.provisioner.Restarts(&a)
	c.Assert(restarts, gocheck.Equals, 1)
}

func (s *S) TestRestartRunsHooksAfterAndBefore(c *gocheck.C) {
	var runner fakeHookRunner
	a := App{Name: "child", Platform: "django", hr: &runner}
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	var buf bytes.Buffer
	err := a.Restart(&buf)
	c.Assert(err, gocheck.IsNil)
	expected := map[string]int{
		"before": 1, "after": 1,
	}
	c.Assert(runner.calls, gocheck.DeepEquals, expected)
}

func (s *S) TestRestartHookFailureBefore(c *gocheck.C) {
	errMock := stderr.New("Before failed")
	runner := fakeHookRunner{
		result: func(kind string) error {
			if kind == "before" {
				return errMock
			}
			return nil
		},
	}
	app := App{Name: "pat", Platform: "python", hr: &runner}
	var buf bytes.Buffer
	err := app.Restart(&buf)
	c.Assert(err, gocheck.Equals, errMock)
}

func (s *S) TestRestartHookFailureAfter(c *gocheck.C) {
	errMock := stderr.New("Before failed")
	runner := fakeHookRunner{
		result: func(kind string) error {
			if kind == "after" {
				return errMock
			}
			return nil
		},
	}
	app := App{Name: "pat", Platform: "python", hr: &runner}
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	var buf bytes.Buffer
	err := app.Restart(&buf)
	c.Assert(err, gocheck.Equals, errMock)
}

func (s *S) TestHookRunnerNil(c *gocheck.C) {
	a := App{Name: "jungle"}
	runner := a.hookRunner()
	c.Assert(runner, gocheck.FitsTypeOf, &yamlHookRunner{})
}

func (s *S) TestHookRunnerNotNil(c *gocheck.C) {
	var fakeRunner fakeHookRunner
	a := App{Name: "jungle", hr: &fakeRunner}
	runner := a.hookRunner()
	c.Assert(runner, gocheck.Equals, &fakeRunner)
}

func (s *S) TestStop(c *gocheck.C) {
	a := App{Name: "app"}
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	var buf bytes.Buffer
	err = a.Stop(&buf)
	c.Assert(err, gocheck.IsNil)
	err = s.conn.Apps().Find(bson.M{"name": a.GetName()}).One(&a)
	c.Assert(err, gocheck.IsNil)
	for _, u := range a.Units() {
		c.Assert(u.Status, gocheck.Equals, provision.StatusStopped)
	}
}

func (s *S) TestLog(c *gocheck.C) {
	a := App{Name: "newApp"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Logs(a.Name).DropCollection()
	}()
	err = a.Log("last log msg", "tsuru", "outermachine")
	c.Assert(err, gocheck.IsNil)
	var logs []Applog
	err = s.conn.Logs(a.Name).Find(nil).All(&logs)
	c.Assert(err, gocheck.IsNil)
	c.Assert(logs, gocheck.HasLen, 1)
	c.Assert(logs[0].Message, gocheck.Equals, "last log msg")
	c.Assert(logs[0].Source, gocheck.Equals, "tsuru")
	c.Assert(logs[0].AppName, gocheck.Equals, a.Name)
	c.Assert(logs[0].Unit, gocheck.Equals, "outermachine")
}

func (s *S) TestLogShouldAddOneRecordByLine(c *gocheck.C) {
	a := App{Name: "newApp"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Logs(a.Name).DropCollection()
	}()
	err = a.Log("last log msg\nfirst log", "source", "machine")
	c.Assert(err, gocheck.IsNil)
	var logs []Applog
	err = s.conn.Logs(a.Name).Find(nil).Sort("$natural").All(&logs)
	c.Assert(err, gocheck.IsNil)
	c.Assert(logs, gocheck.HasLen, 2)
	c.Assert(logs[0].Message, gocheck.Equals, "last log msg")
	c.Assert(logs[1].Message, gocheck.Equals, "first log")
}

func (s *S) TestLogShouldNotLogBlankLines(c *gocheck.C) {
	a := App{Name: "ich"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = a.Log("some message", "tsuru", "machine")
	c.Assert(err, gocheck.IsNil)
	err = a.Log("", "", "")
	c.Assert(err, gocheck.IsNil)
	count, err := s.conn.Logs(a.Name).Find(nil).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(count, gocheck.Equals, 1)
}

func (s *S) TestLogWithListeners(c *gocheck.C) {
	var logs struct {
		l []Applog
		sync.Mutex
	}
	a := App{
		Name: "newApp",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	l, err := NewLogListener(&a, Applog{})
	c.Assert(err, gocheck.IsNil)
	defer l.Close()
	go func() {
		for log := range l.C {
			logs.Lock()
			logs.l = append(logs.l, log)
			logs.Unlock()
		}
	}()
	err = a.Log("last log msg", "tsuru", "machine")
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Logs(a.Name).DropCollection()
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
	c.Assert(logs.l, gocheck.HasLen, 1)
	log := logs.l[0]
	logs.Unlock()
	c.Assert(log.Message, gocheck.Equals, "last log msg")
	c.Assert(log.Source, gocheck.Equals, "tsuru")
	c.Assert(log.Unit, gocheck.Equals, "machine")
}

func (s *S) TestLastLogs(c *gocheck.C) {
	app := App{
		Name:     "app3",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	for i := 0; i < 15; i++ {
		app.Log(strconv.Itoa(i), "tsuru", "rdaneel")
		time.Sleep(1e6) // let the time flow
	}
	app.Log("app3 log from circus", "circus", "rdaneel")
	logs, err := app.LastLogs(10, Applog{Source: "tsuru"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(logs, gocheck.HasLen, 10)
	for i := 5; i < 15; i++ {
		c.Check(logs[i-5].Message, gocheck.Equals, strconv.Itoa(i))
		c.Check(logs[i-5].Source, gocheck.Equals, "tsuru")
	}
}

func (s *S) TestLastLogsUnitFilter(c *gocheck.C) {
	app := App{
		Name:     "app3",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	for i := 0; i < 15; i++ {
		app.Log(strconv.Itoa(i), "tsuru", "rdaneel")
		time.Sleep(1e6) // let the time flow
	}
	app.Log("app3 log from circus", "circus", "rdaneel")
	app.Log("app3 log from tsuru", "tsuru", "seldon")
	logs, err := app.LastLogs(10, Applog{Source: "tsuru", Unit: "rdaneel"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(logs, gocheck.HasLen, 10)
	for i := 5; i < 15; i++ {
		c.Check(logs[i-5].Message, gocheck.Equals, strconv.Itoa(i))
		c.Check(logs[i-5].Source, gocheck.Equals, "tsuru")
	}
}

func (s *S) TestLastLogsEmpty(c *gocheck.C) {
	app := App{
		Name:     "app33",
		Platform: "vougan",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	logs, err := app.LastLogs(10, Applog{Source: "tsuru"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(logs, gocheck.DeepEquals, []Applog{})
}

func (s *S) TestGetTeams(c *gocheck.C) {
	app := App{Name: "app", Teams: []string{s.team.Name}}
	teams := app.GetTeams()
	c.Assert(teams, gocheck.HasLen, 1)
	c.Assert(teams[0].Name, gocheck.Equals, s.team.Name)
}

func (s *S) TestSetTeams(c *gocheck.C) {
	app := App{Name: "app"}
	app.SetTeams([]auth.Team{s.team})
	c.Assert(app.Teams, gocheck.DeepEquals, []string{s.team.Name})
}

func (s *S) TestSetTeamsSortTeamNames(c *gocheck.C) {
	app := App{Name: "app"}
	app.SetTeams([]auth.Team{s.team, {Name: "zzz"}, {Name: "aaa"}})
	c.Assert(app.Teams, gocheck.DeepEquals, []string{"aaa", s.team.Name, "zzz"})
}

func (s *S) TestGetUnits(c *gocheck.C) {
	app := App{Name: "app"}
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	s.provisioner.AddUnits(&app, 1)
	c.Assert(app.GetUnits(), gocheck.HasLen, 1)
	c.Assert(app.Units()[0].Ip, gocheck.Equals, app.GetUnits()[0].GetIp())
}

func (s *S) TestAppMarshalJSON(c *gocheck.C) {
	app := App{
		Name:     "name",
		Platform: "Framework",
		Teams:    []string{"team1"},
		Ip:       "10.10.10.1",
		CName:    "name.mycompany.com",
		Owner:    "appOwner",
		Deploys:  7,
		Memory:   64,
		Swap:     128,
	}
	expected := make(map[string]interface{})
	expected["name"] = "name"
	expected["platform"] = "Framework"
	expected["repository"] = repository.ReadWriteURL(app.Name)
	expected["teams"] = []interface{}{"team1"}
	expected["units"] = nil
	expected["ip"] = "10.10.10.1"
	expected["cname"] = "name.mycompany.com"
	expected["owner"] = "appOwner"
	expected["deploys"] = float64(7)
	expected["memory"] = "64"
	// Expected swap is the "real" swap size. App object has the sum of memory + swap but in json we return it as real size (swap - memory)
	expected["swap"] = "64"
	expected["ready"] = false
	data, err := app.MarshalJSON()
	c.Assert(err, gocheck.IsNil)
	result := make(map[string]interface{})
	err = json.Unmarshal(data, &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.DeepEquals, expected)
}

func (s *S) TestAppMarshalJSONReady(c *gocheck.C) {
	app := App{
		Name:     "name",
		Platform: "Framework",
		Teams:    []string{"team1"},
		Ip:       "10.10.10.1",
		CName:    "name.mycompany.com",
		State:    "ready",
		Owner:    "appOwner",
		Deploys:  7,
		Memory:   64,
		Swap:     128,
	}
	expected := make(map[string]interface{})
	expected["name"] = "name"
	expected["platform"] = "Framework"
	expected["repository"] = repository.ReadWriteURL(app.Name)
	expected["teams"] = []interface{}{"team1"}
	expected["units"] = nil
	expected["ip"] = "10.10.10.1"
	expected["cname"] = "name.mycompany.com"
	expected["owner"] = "appOwner"
	expected["deploys"] = float64(7)
	expected["memory"] = "64"
	// Expected swap is the "real" swap size. App object has the sum of memory + swap but in json we return it as real size (swap - memory)
	expected["swap"] = "64"
	expected["ready"] = true
	data, err := app.MarshalJSON()
	c.Assert(err, gocheck.IsNil)
	result := make(map[string]interface{})
	err = json.Unmarshal(data, &result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.DeepEquals, expected)
}

func (s *S) TestRun(c *gocheck.C) {
	s.provisioner.PrepareOutput([]byte("a lot of files"))
	app := App{
		Name: "myapp",
	}
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	s.provisioner.AddUnits(&app, 1)
	var buf bytes.Buffer
	err := app.Run("ls -lh", &buf, false)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "a lot of files")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls -lh"
	cmds := s.provisioner.GetCmds(expected, &app)
	c.Assert(cmds, gocheck.HasLen, 1)
}

func (s *S) TestRunOnce(c *gocheck.C) {
	s.provisioner.PrepareOutput([]byte("a lot of files"))
	app := App{
		Name: "myapp",
	}
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	s.provisioner.AddUnits(&app, 1)
	var buf bytes.Buffer
	err := app.Run("ls -lh", &buf, true)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "a lot of files")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls -lh"
	cmds := s.provisioner.GetCmds(expected, &app)
	c.Assert(cmds, gocheck.HasLen, 1)
}

func (s *S) TestRunWithoutEnv(c *gocheck.C) {
	s.provisioner.PrepareOutput([]byte("a lot of files"))
	app := App{
		Name: "myapp",
	}
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	s.provisioner.AddUnits(&app, 1)
	var buf bytes.Buffer
	err := app.run("ls -lh", &buf, false)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "a lot of files")
	cmds := s.provisioner.GetCmds("ls -lh", &app)
	c.Assert(cmds, gocheck.HasLen, 1)
}

func (s *S) TestEnvs(c *gocheck.C) {
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
	c.Assert(env, gocheck.DeepEquals, app.Env)
}

func (s *S) TestListReturnsAppsForAGivenUser(c *gocheck.C) {
	a := App{
		Name:  "testapp",
		Teams: []string{s.team.Name},
	}
	a2 := App{
		Name:  "othertestapp",
		Teams: []string{"commonteam", s.team.Name},
	}
	err := s.conn.Apps().Insert(&a)
	c.Assert(err, gocheck.IsNil)
	err = s.conn.Apps().Insert(&a2)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Apps().Remove(bson.M{"name": a2.Name})
	}()
	apps, err := List(s.user)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(apps), gocheck.Equals, 2)
}

func (s *S) TestListReturnsEmptyAppArrayWhenUserHasNoAccessToAnyApp(c *gocheck.C) {
	apps, err := List(s.user)
	c.Assert(err, gocheck.IsNil)
	c.Assert(apps, gocheck.DeepEquals, []App(nil))
}

func (s *S) TestListReturnsAllAppsWhenUserIsInAdminTeam(c *gocheck.C) {
	a := App{Name: "testApp", Teams: []string{"notAdmin", "noSuperUser"}}
	err := s.conn.Apps().Insert(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.createAdminUserAndTeam(c)
	defer s.removeAdminUserAndTeam(c)
	apps, err := List(s.admin)
	c.Assert(len(apps), Greater, 0)
	c.Assert(apps[0].Name, gocheck.Equals, "testApp")
	c.Assert(apps[0].Teams, gocheck.DeepEquals, []string{"notAdmin", "noSuperUser"})
}

func (s *S) TestReturnTrueIfSameAppNameAndPlatformName(c *gocheck.C) {
	a := App{
		Name:     "sameName",
		Platform: "sameName",
	}
	c.Assert(a.equalAppNameAndPlatformName(), gocheck.Equals, true)
}

func (s *S) TestReturnFalseIfSameAppNameAndPlatformName(c *gocheck.C) {
	a := App{
		Name:     "sameName",
		Platform: "differentName",
	}
	c.Assert(a.equalAppNameAndPlatformName(), gocheck.Equals, false)
}

func (s *S) TestReturnTrueIfAppNameEqualToSomePlatformName(c *gocheck.C) {
	a := App{Name: "sameName"}
	platforms := []Platform{
		{Name: "not"},
		{Name: "sameName"},
		{Name: "nothing"},
	}
	conn, _ := db.Conn()
	defer conn.Close()
	for _, p := range platforms {
		conn.Platforms().Insert(p)
		defer conn.Platforms().Remove(p)
	}
	c.Assert(a.equalToSomePlatformName(), gocheck.Equals, true)
}

func (s *S) TestReturnFalseIfAppNameEqualToSomePlatformName(c *gocheck.C) {
	a := App{Name: "differentName"}
	platforms := []Platform{
		{Name: "yyyyy"},
		{Name: "xxxxx"},
	}
	conn, _ := db.Conn()
	defer conn.Close()
	for _, p := range platforms {
		conn.Platforms().Insert(p)
		defer conn.Platforms().Remove(p)
	}
	c.Assert(a.equalToSomePlatformName(), gocheck.Equals, false)
}

func (s *S) TestGetName(c *gocheck.C) {
	a := App{Name: "something"}
	c.Assert(a.GetName(), gocheck.Equals, a.Name)
}

func (s *S) TestGetIP(c *gocheck.C) {
	a := App{Ip: "10.10.10.10"}
	c.Assert(a.GetIp(), gocheck.Equals, a.Ip)
}

func (s *S) TestGetPlatform(c *gocheck.C) {
	a := App{Platform: "django"}
	c.Assert(a.GetPlatform(), gocheck.Equals, a.Platform)
}

func (s *S) TestGetDeploys(c *gocheck.C) {
	a := App{Deploys: 3}
	c.Assert(a.GetDeploys(), gocheck.Equals, a.Deploys)
}

func (s *S) TestGetMemory(c *gocheck.C) {
	a := App{Memory: 64}
	c.Assert(a.GetMemory(), gocheck.Equals, a.Memory)
}

func (s *S) TestGetSwap(c *gocheck.C) {
	a := App{Swap: 64}
	c.Assert(a.GetSwap(), gocheck.Equals, a.Swap)
}

func (s *S) TestAppUnits(c *gocheck.C) {
	a := App{Name: "anycolor"}
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1)
	c.Assert(a.Units(), gocheck.HasLen, 1)
}

func (s *S) TestAppAvailable(c *gocheck.C) {
	a := App{
		Name: "anycolor",
	}
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1)
	c.Assert(a.Available(), gocheck.Equals, true)
	s.provisioner.Stop(&a)
	c.Assert(a.Available(), gocheck.Equals, false)
}

func (s *S) TestSwap(c *gocheck.C) {
	var err error
	app1 := &App{Name: "app1", CName: "cname"}
	err = s.provisioner.Provision(app1)
	c.Assert(err, gocheck.IsNil)
	app1.Ip, err = s.provisioner.Addr(app1)
	c.Assert(err, gocheck.IsNil)
	oldIp1 := app1.Ip
	err = s.conn.Apps().Insert(app1)
	c.Assert(err, gocheck.IsNil)
	app2 := &App{Name: "app2"}
	err = s.provisioner.Provision(app2)
	c.Assert(err, gocheck.IsNil)
	app2.Ip, err = s.provisioner.Addr(app2)
	c.Assert(err, gocheck.IsNil)
	oldIp2 := app2.Ip
	err = s.conn.Apps().Insert(app2)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": app1.Name})
		s.conn.Apps().Remove(bson.M{"name": app2.Name})
	}()
	err = Swap(app1, app2)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app1.CName, gocheck.Equals, "")
	c.Assert(app2.CName, gocheck.Equals, "cname")
	c.Assert(app1.Ip, gocheck.Equals, oldIp2)
	c.Assert(app2.Ip, gocheck.Equals, oldIp1)
}

func (s *S) TestStart(c *gocheck.C) {
	s.provisioner.PrepareOutput([]byte("not yaml")) // loadConf
	a := App{
		Name:     "someApp",
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	var b bytes.Buffer
	err = a.Start(&b)
	c.Assert(err, gocheck.IsNil)
	starts := s.provisioner.Starts(&a)
	c.Assert(starts, gocheck.Equals, 1)
}

func (s *S) TestAppSetUpdatePlatform(c *gocheck.C) {
	a := App{
		Name:     "someApp",
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	a.SetUpdatePlatform(true)
	app, err := GetByName("someApp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.UpdatePlatform, gocheck.Equals, true)
}

func (s *S) TestAppAcquireApplicationLock(c *gocheck.C) {
	a := App{
		Name: "someApp",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	locked, err := AcquireApplicationLock(a.Name, "foo", "/something")
	c.Assert(err, gocheck.IsNil)
	c.Assert(locked, gocheck.Equals, true)
	app, err := GetByName("someApp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Lock.Locked, gocheck.Equals, true)
	c.Assert(app.Lock.Owner, gocheck.Equals, "foo")
	c.Assert(app.Lock.Reason, gocheck.Equals, "/something")
	c.Assert(app.Lock.AcquireDate, gocheck.NotNil)
}

func (s *S) TestAppAcquireApplicationLockNonExistentApp(c *gocheck.C) {
	locked, err := AcquireApplicationLock("myApp", "foo", "/something")
	c.Assert(err, gocheck.IsNil)
	c.Assert(locked, gocheck.Equals, false)
}

func (s *S) TestAppAcquireApplicationLockAlreadyLocked(c *gocheck.C) {
	a := App{
		Name: "someApp",
		Lock: AppLock{
			Locked:      true,
			Reason:      "/app/my-app/deploy",
			Owner:       "someone",
			AcquireDate: time.Date(2048, time.November, 10, 10, 0, 0, 0, time.UTC),
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	locked, err := AcquireApplicationLock(a.Name, "foo", "/something")
	c.Assert(err, gocheck.IsNil)
	c.Assert(locked, gocheck.Equals, false)
	app, err := GetByName("someApp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Lock.Locked, gocheck.Equals, true)
	c.Assert(app.Lock.Owner, gocheck.Equals, "someone")
	c.Assert(app.Lock.Reason, gocheck.Equals, "/app/my-app/deploy")
	c.Assert(app.Lock.AcquireDate, gocheck.NotNil)
}

func (s *S) TestAppLockStringUnlocked(c *gocheck.C) {
	lock := AppLock{Locked: false}
	c.Assert(lock.String(), gocheck.Equals, "Not locked")
}

func (s *S) TestAppLockStringLocked(c *gocheck.C) {
	lock := AppLock{
		Locked:      true,
		Reason:      "/app/my-app/deploy",
		Owner:       "someone",
		AcquireDate: time.Date(2048, time.November, 10, 10, 0, 0, 0, time.UTC),
	}
	c.Assert(lock.String(), gocheck.Matches, "App locked by someone, running /app/my-app/deploy. Acquired in 2048-11-10T.*")
}
