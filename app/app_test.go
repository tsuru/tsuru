// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"encoding/json"
	stderr "errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/safe"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/testing"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
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
	env := retrievedApp.InstanceEnv("")
	c.Assert(env["TSURU_APPNAME"].Value, gocheck.Equals, a.Name)
	c.Assert(env["TSURU_APPNAME"].Public, gocheck.Equals, false)
	c.Assert(env["TSURU_HOST"].Value, gocheck.Equals, expectedHost)
	c.Assert(env["TSURU_HOST"].Public, gocheck.Equals, false)
	err = auth.ReserveApp(s.user)
	_, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestCreateAppDefaultPlan(c *gocheck.C) {
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
	c.Assert(err, gocheck.IsNil)
	c.Assert(retrievedApp.Plan, gocheck.DeepEquals, s.defaultPlan)
}

func (s *S) TestCreateAppWithoutDefaultPlan(c *gocheck.C) {
	s.conn.Plans().RemoveAll(nil)
	defer s.conn.Plans().Insert(s.defaultPlan)
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
	c.Assert(err, gocheck.IsNil)
	c.Assert(retrievedApp.Plan, gocheck.DeepEquals, Plan{
		Name:     "autogenerated",
		Memory:   0,
		Swap:     0,
		CpuShare: 100,
	})
}

func (s *S) TestCreateAppWithExplicitPlan(c *gocheck.C) {
	myPlan := Plan{
		Name:     "myplan",
		Memory:   1,
		Swap:     2,
		CpuShare: 3,
	}
	err := myPlan.Save()
	c.Assert(err, gocheck.IsNil)
	defer PlanRemove(myPlan.Name)
	ts := testing.StartGandalfTestServer(&testHandler{})
	defer ts.Close()
	a := App{
		Name:     "appname",
		Platform: "python",
		Plan:     Plan{Name: "myplan"},
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": 1}})
	defer s.conn.Users().Update(bson.M{"email": s.user.Email}, bson.M{"$set": bson.M{"quota.limit": -1}})
	config.Set("quota:units-per-app", 3)
	defer config.Unset("quota:units-per-app")
	err = CreateApp(&a, s.user)
	c.Assert(err, gocheck.IsNil)
	defer Delete(&a)
	retrievedApp, err := GetByName(a.Name)
	c.Assert(retrievedApp.Plan, gocheck.DeepEquals, myPlan)
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
	c.Assert(err, gocheck.ErrorMatches, "team not found")
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

func (s *S) TestBindUnit(c *gocheck.C) {
	var requests []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
	}))
	defer server.Close()
	app := App{
		Name: "warpaint", Platform: "python",
		Quota: quota.Unlimited,
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	srvc := service.Service{
		Name:     "mysql",
		Endpoint: map[string]string{"production": server.URL},
	}
	err = srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer srvc.Delete()
	si1 := service.ServiceInstance{Name: "mydb", ServiceName: "mysql", Apps: []string{app.Name}}
	err = si1.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si1.Name})
	si2 := service.ServiceInstance{Name: "yourdb", ServiceName: "mysql", Apps: []string{app.Name}}
	err = si2.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si2.Name})
	unit := provision.Unit{Name: "some-unit", Ip: "127.0.2.1"}
	err = app.BindUnit(&unit)
	c.Assert(err, gocheck.IsNil)
	c.Assert(requests, gocheck.HasLen, 2)
	c.Assert(requests[0].Method, gocheck.Equals, "POST")
	c.Assert(requests[0].URL.Path, gocheck.Equals, "/resources/mydb/bind")
	c.Assert(requests[1].Method, gocheck.Equals, "POST")
	c.Assert(requests[1].URL.Path, gocheck.Equals, "/resources/yourdb/bind")
}

func (s *S) TestUnbindUnit(c *gocheck.C) {
	var requests []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
	}))
	defer server.Close()
	app := App{
		Name: "warpaint", Platform: "python",
		Quota: quota.Unlimited,
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	srvc := service.Service{
		Name:     "mysql",
		Endpoint: map[string]string{"production": server.URL},
	}
	err = srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer srvc.Delete()
	si1 := service.ServiceInstance{Name: "mydb", ServiceName: "mysql", Apps: []string{app.Name}}
	err = si1.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si1.Name})
	si2 := service.ServiceInstance{Name: "yourdb", ServiceName: "mysql", Apps: []string{app.Name}}
	err = si2.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si2.Name})
	unit := provision.Unit{Name: "some-unit", Ip: "127.0.2.1"}
	err = app.UnbindUnit(&unit)
	c.Assert(err, gocheck.IsNil)
	c.Assert(requests, gocheck.HasLen, 2)
	c.Assert(requests[0].Method, gocheck.Equals, "DELETE")
	c.Assert(requests[0].URL.Path, gocheck.Equals, "/resources/mydb/bind")
	c.Assert(requests[1].Method, gocheck.Equals, "DELETE")
	c.Assert(requests[1].URL.Path, gocheck.Equals, "/resources/yourdb/bind")
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
	err = app.AddUnits(5, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Units(), gocheck.HasLen, 5)
	err = app.AddUnits(2, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Units(), gocheck.HasLen, 7)
	for _, unit := range app.Units() {
		c.Assert(unit.AppName, gocheck.Equals, app.Name)
	}
}

func (s *S) TestAddUnitsWithWriter(c *gocheck.C) {
	app := App{
		Name: "warpaint", Platform: "python",
		Quota: quota.Unlimited,
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	var buf bytes.Buffer
	err = app.AddUnits(2, &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Units(), gocheck.HasLen, 2)
	for _, unit := range app.Units() {
		c.Assert(unit.AppName, gocheck.Equals, app.Name)
	}
	c.Assert(buf.String(), gocheck.Equals, "added 2 units")
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
	err = otherApp.AddUnits(5, nil)
	c.Assert(err, gocheck.IsNil)
	units := s.provisioner.GetUnits(&app)
	c.Assert(units, gocheck.HasLen, 5)
	err = otherApp.AddUnits(2, nil)
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
	err := app.AddUnits(1, nil)
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
	err := app.AddUnits(11, nil)
	e, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Available, gocheck.Equals, uint(10))
	c.Assert(e.Requested, gocheck.Equals, uint(11))
}

func (s *S) TestAddZeroUnits(c *gocheck.C) {
	app := App{Name: "warpaint", Platform: "ruby"}
	err := app.AddUnits(0, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Cannot add zero units.")
}

func (s *S) TestAddUnitsFailureInProvisioner(c *gocheck.C) {
	app := App{Name: "scars", Platform: "golang", Quota: quota.Unlimited}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	err := app.AddUnits(2, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "App is not provisioned.")
}

func (s *S) TestAddUnitsIsAtomic(c *gocheck.C) {
	app := App{
		Name: "warpaint", Platform: "golang",
		Quota: quota.Unlimited,
	}
	err := app.AddUnits(2, nil)
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
	s.provisioner.AddUnits(&a, 5, nil)
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
	app.AddUnits(4, nil)
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
	s.provisioner.AddUnits(&app, 3, nil)
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
	s.provisioner.AddUnits(&a, 3, nil)
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
	s.provisioner.AddUnits(&a, 3, nil)
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
	var buf bytes.Buffer
	err = a.setEnvsToApp(envs, true, true, &buf)
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
	c.Assert(buf.String(), gocheck.Equals, "---- Setting 2 new environment variables ----\nrestarting app")
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
	err = a.setEnvsToApp(envs, false, true, nil)
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

func (s *S) TestSetEnvsWhenAppHaveNoUnits(c *gocheck.C) {
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
	err = a.setEnvsToApp(envs, false, false, nil)
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
	c.Assert(s.provisioner.Restarts(&a), gocheck.Equals, 0)
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
		Quota: quota.Quota{
			Limit: 10,
		},
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	err = a.AddUnits(1, nil)
	c.Assert(err, gocheck.IsNil)
	err = a.UnsetEnvs([]string{"DATABASE_HOST", "DATABASE_PASSWORD"}, true, nil)
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
		Quota: quota.Quota{
			Limit: 10,
		},
	}
	s.provisioner.PrepareOutput([]byte("exported"))
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	err = a.AddUnits(1, nil)
	c.Assert(err, gocheck.IsNil)
	err = a.UnsetEnvs([]string{"DATABASE_HOST", "DATABASE_PASSWORD"}, false, nil)
	c.Assert(err, gocheck.IsNil)
	newApp, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(newApp.Env, gocheck.DeepEquals, map[string]bind.EnvVar{})
	c.Assert(s.provisioner.Restarts(&a), gocheck.Equals, 1)
}

func (s *S) TestUnsetEnvNoUnits(c *gocheck.C) {
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
	err = a.UnsetEnvs([]string{"DATABASE_HOST", "DATABASE_PASSWORD"}, false, nil)
	c.Assert(err, gocheck.IsNil)
	newApp, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(newApp.Env, gocheck.DeepEquals, map[string]bind.EnvVar{})
	c.Assert(s.provisioner.Restarts(&a), gocheck.Equals, 0)
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

func (s *S) TestAddCName(c *gocheck.C) {
	app := &App{Name: "ktulu"}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(app)
	defer s.provisioner.Destroy(app)
	err = app.AddCName("ktulu.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	app, err = GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.CName, gocheck.DeepEquals, []string{"ktulu.mycompany.com"})
	err = app.AddCName("ktulu2.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	app, err = GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.CName, gocheck.DeepEquals, []string{"ktulu.mycompany.com", "ktulu2.mycompany.com"})
}

func (s *S) TestAddCNameCantBeDuplicated(c *gocheck.C) {
	app := &App{Name: "ktulu"}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(app)
	defer s.provisioner.Destroy(app)
	err = app.AddCName("ktulu.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	err = app.AddCName("ktulu.mycompany.com")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "cname already exists!")
	app2 := &App{Name: "ktulu2"}
	err = s.conn.Apps().Insert(app2)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app2.Name})
	err = app2.AddCName("ktulu.mycompany.com")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "cname already exists!")
}

func (s *S) TestAddCNameWithWildCard(c *gocheck.C) {
	app := &App{Name: "ktulu"}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(app)
	defer s.provisioner.Destroy(app)
	err = app.AddCName("*.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	app, err = GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.CName, gocheck.DeepEquals, []string{"*.mycompany.com"})
}

func (s *S) TestAddCNameErrsOnInvalid(c *gocheck.C) {
	app := &App{Name: "ktulu"}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(app)
	defer s.provisioner.Destroy(app)
	err = app.AddCName("_ktulu.mycompany.com")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Invalid cname")
}

func (s *S) TestAddCNamePartialUpdate(c *gocheck.C) {
	a := &App{Name: "master", Platform: "puppet"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(a)
	defer s.provisioner.Destroy(a)
	other := App{Name: a.Name}
	err = other.AddCName("ktulu.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(a.Platform, gocheck.Equals, "puppet")
	c.Assert(a.Name, gocheck.Equals, "master")
	c.Assert(a.CName, gocheck.DeepEquals, []string{"ktulu.mycompany.com"})
}

func (s *S) TestAddCNameUnknownApp(c *gocheck.C) {
	a := App{Name: "ktulu"}
	err := a.AddCName("ktulu.mycompany.com")
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestAddCNameValidatesTheCName(c *gocheck.C) {
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
		err := a.AddCName(t.input)
		if !t.valid {
			c.Check(err.Error(), gocheck.Equals, "Invalid cname")
		} else {
			c.Check(err, gocheck.IsNil)
		}
	}
}

func (s *S) TestAddCNameCallsProvisionerSetCName(c *gocheck.C) {
	a := App{Name: "ktulu"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	err = a.AddCName("ktulu.mycompany.com", "ktulu2.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	hasCName := s.provisioner.HasCName(&a, "ktulu.mycompany.com")
	c.Assert(hasCName, gocheck.Equals, true)
	hasCName = s.provisioner.HasCName(&a, "ktulu2.mycompany.com")
	c.Assert(hasCName, gocheck.Equals, true)
}

func (s *S) TestRemoveCNameRemovesFromDatabase(c *gocheck.C) {
	a := &App{Name: "ktulu"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(a)
	defer s.provisioner.Destroy(a)
	err = a.AddCName("ktulu.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	err = a.RemoveCName("ktulu.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.CName, gocheck.DeepEquals, []string{})
}

func (s *S) TestRemoveCNameWhichNoExists(c *gocheck.C) {
	a := &App{Name: "ktulu"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(a)
	defer s.provisioner.Destroy(a)
	err = a.RemoveCName("ktulu.mycompany.com")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "cname not exists!")
}

func (s *S) TestRemoveMoreThanOneCName(c *gocheck.C) {
	a := &App{Name: "ktulu"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(a)
	defer s.provisioner.Destroy(a)
	err = a.AddCName("ktulu.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	err = a.AddCName("ktulu2.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	err = a.RemoveCName("ktulu.mycompany.com", "ktulu2.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.CName, gocheck.DeepEquals, []string{})
}

func (s *S) TestRemoveCNameRemovesFromRouter(c *gocheck.C) {
	a := App{Name: "ktulu"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	err = a.AddCName("ktulu.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	err = a.RemoveCName("ktulu.mycompany.com")
	c.Assert(err, gocheck.IsNil)
	hasCName := s.provisioner.HasCName(&a, "ktulu.mycompany.com")
	c.Assert(hasCName, gocheck.Equals, false)
}

func (s *S) TestAddInstanceFirst(c *gocheck.C) {
	a := &App{Name: "dark"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(a)
	defer s.provisioner.Destroy(a)
	instance := bind.ServiceInstance{
		Name: "myinstance",
		Envs: map[string]string{
			"DATABASE_HOST": "localhost",
			"DATABASE_PORT": "3306",
			"DATABASE_USER": "root",
		},
	}
	err = a.AddInstance("myservice", instance, nil)
	c.Assert(err, gocheck.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	expected := map[string][]bind.ServiceInstance{"myservice": {instance}}
	env, ok := a.Env[TsuruServicesEnvVar]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(env.Public, gocheck.Equals, false)
	c.Assert(env.Name, gocheck.Equals, TsuruServicesEnvVar)
	var got map[string][]bind.ServiceInstance
	err = json.Unmarshal([]byte(env.Value), &got)
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.DeepEquals, expected)
	delete(a.Env, TsuruServicesEnvVar)
	c.Assert(a.Env, gocheck.DeepEquals, map[string]bind.EnvVar{
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
	c.Assert(s.provisioner.Restarts(a), gocheck.Equals, 0)
}

func (s *S) TestAddInstanceWithUnits(c *gocheck.C) {
	a := &App{Name: "dark", Quota: quota.Quota{Limit: 10}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(a)
	defer s.provisioner.Destroy(a)
	err = a.AddUnits(1, nil)
	c.Assert(err, gocheck.IsNil)
	instance := bind.ServiceInstance{
		Name: "myinstance",
		Envs: map[string]string{
			"DATABASE_HOST": "localhost",
		},
	}
	err = a.AddInstance("myservice", instance, nil)
	c.Assert(err, gocheck.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	expected := map[string][]bind.ServiceInstance{"myservice": {instance}}
	env, ok := a.Env[TsuruServicesEnvVar]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(env.Public, gocheck.Equals, false)
	c.Assert(env.Name, gocheck.Equals, TsuruServicesEnvVar)
	var got map[string][]bind.ServiceInstance
	err = json.Unmarshal([]byte(env.Value), &got)
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.DeepEquals, expected)
	delete(a.Env, TsuruServicesEnvVar)
	c.Assert(a.Env, gocheck.DeepEquals, map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:         "DATABASE_HOST",
			Value:        "localhost",
			Public:       false,
			InstanceName: "myinstance",
		},
	})
	c.Assert(s.provisioner.Restarts(a), gocheck.Equals, 1)
}

func (s *S) TestAddInstanceMultipleServices(c *gocheck.C) {
	a := &App{
		Name: "dark",
		Env: map[string]bind.EnvVar{
			TsuruServicesEnvVar: {
				Name:   TsuruServicesEnvVar,
				Public: false,
				Value:  `{"mysql": [{"instance_name": "mydb", "envs": {"DATABASE_NAME": "mydb"}}]}`,
			},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(a)
	defer s.provisioner.Destroy(a)
	instance1 := bind.ServiceInstance{
		Name: "myinstance",
		Envs: map[string]string{"DATABASE_NAME": "myinstance"},
	}
	err = a.AddInstance("mysql", instance1, nil)
	c.Assert(err, gocheck.IsNil)
	instance2 := bind.ServiceInstance{
		Name: "yourinstance",
		Envs: map[string]string{"DATABASE_NAME": "supermongo"},
	}
	err = a.AddInstance("mongodb", instance2, nil)
	c.Assert(err, gocheck.IsNil)
	expected := map[string][]bind.ServiceInstance{
		"mysql":   {bind.ServiceInstance{Name: "mydb", Envs: map[string]string{"DATABASE_NAME": "mydb"}}, instance1},
		"mongodb": {instance2},
	}
	a, err = GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.parsedTsuruServices(), gocheck.DeepEquals, expected)
	delete(a.Env, "TSURU_SERVICES")
	c.Assert(a.Env, gocheck.DeepEquals, map[string]bind.EnvVar{
		"DATABASE_NAME": {
			Name:         "DATABASE_NAME",
			Value:        "supermongo",
			Public:       false,
			InstanceName: "yourinstance",
		},
	})
}

func (s *S) TestAddInstanceAndRemoveInstanceMultipleServices(c *gocheck.C) {
	a := &App{
		Name: "fuchsia",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(a)
	defer s.provisioner.Destroy(a)
	instance1 := bind.ServiceInstance{
		Name: "myinstance",
		Envs: map[string]string{"DATABASE_NAME": "myinstance"},
	}
	err = a.AddInstance("mysql", instance1, nil)
	c.Assert(err, gocheck.IsNil)
	instance2 := bind.ServiceInstance{
		Name: "yourinstance",
		Envs: map[string]string{"DATABASE_NAME": "supermongo"},
	}
	err = a.AddInstance("mongodb", instance2, nil)
	c.Assert(err, gocheck.IsNil)
	err = a.RemoveInstance("mysql", instance1, nil)
	c.Assert(err, gocheck.IsNil)
	expected := map[string][]bind.ServiceInstance{
		"mysql":   {},
		"mongodb": {instance2},
	}
	a, err = GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(a.parsedTsuruServices(), gocheck.DeepEquals, expected)
	delete(a.Env, "TSURU_SERVICES")
	c.Assert(a.Env, gocheck.DeepEquals, map[string]bind.EnvVar{
		"DATABASE_NAME": {
			Name:         "DATABASE_NAME",
			Value:        "supermongo",
			Public:       false,
			InstanceName: "yourinstance",
		},
	})
}

func (s *S) TestRemoveInstance(c *gocheck.C) {
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
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(a)
	defer s.provisioner.Destroy(a)
	instance := bind.ServiceInstance{Name: "mydb", Envs: map[string]string{"DATABASE_NAME": "mydb"}}
	err = a.RemoveInstance("mysql", instance, nil)
	c.Assert(err, gocheck.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	env, ok := a.Env[TsuruServicesEnvVar]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(env.Value, gocheck.Equals, `{"mysql":[]}`)
	c.Assert(env.Public, gocheck.Equals, false)
	c.Assert(env.Name, gocheck.Equals, TsuruServicesEnvVar)
	delete(a.Env, TsuruServicesEnvVar)
	c.Assert(a.Env, gocheck.DeepEquals, map[string]bind.EnvVar{})
	c.Assert(s.provisioner.Restarts(a), gocheck.Equals, 0)
}

func (s *S) TestRemoveInstanceShifts(c *gocheck.C) {
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
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(a)
	defer s.provisioner.Destroy(a)
	instance := bind.ServiceInstance{Name: "hisdb"}
	err = a.RemoveInstance("mysql", instance, nil)
	c.Assert(err, gocheck.IsNil)
	expected := map[string][]bind.ServiceInstance{
		"mysql": {
			bind.ServiceInstance{Name: "mydb", Envs: map[string]string{"DATABASE_NAME": "mydb"}},
			bind.ServiceInstance{Name: "yourdb", Envs: map[string]string{"DATABASE_NAME": "yourdb"}},
			bind.ServiceInstance{Name: "herdb", Envs: map[string]string{"DATABASE_NAME": "herdb"}},
			bind.ServiceInstance{Name: "ourdb", Envs: map[string]string{"DATABASE_NAME": "ourdb"}},
		},
	}
	a, err = GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	env, ok := a.Env[TsuruServicesEnvVar]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(env.Public, gocheck.Equals, false)
	c.Assert(env.Name, gocheck.Equals, TsuruServicesEnvVar)
	var got map[string][]bind.ServiceInstance
	err = json.Unmarshal([]byte(env.Value), &got)
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.DeepEquals, expected)
}

func (s *S) TestRemoveInstanceNotFound(c *gocheck.C) {
	a := &App{
		Name: "dark",
		Env: map[string]bind.EnvVar{
			TsuruServicesEnvVar: {
				Name:   TsuruServicesEnvVar,
				Public: false,
				Value:  `{"mysql": [{"instance_name": "mydb", "envs": {"DATABASE_NAME": "mydb"}}]}`,
			},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(a)
	defer s.provisioner.Destroy(a)
	instance := bind.ServiceInstance{Name: "yourdb"}
	err = a.RemoveInstance("mysql", instance, nil)
	c.Assert(err, gocheck.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	services := a.parsedTsuruServices()
	c.Assert(services, gocheck.DeepEquals, map[string][]bind.ServiceInstance{
		"mysql": {
			{
				Name: "mydb",
				Envs: map[string]string{"DATABASE_NAME": "mydb"},
			},
		},
	})
}

func (s *S) TestRemoveInstanceServiceNotFound(c *gocheck.C) {
	a := &App{
		Name: "dark",
		Env: map[string]bind.EnvVar{
			TsuruServicesEnvVar: {
				Name:   TsuruServicesEnvVar,
				Public: false,
				Value:  `{"mysql": [{"instance_name": "mydb", "envs": {"DATABASE_NAME": "mydb"}}]}`,
			},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(a)
	defer s.provisioner.Destroy(a)
	instance := bind.ServiceInstance{Name: "mydb"}
	err = a.RemoveInstance("mongodb", instance, nil)
	c.Assert(err, gocheck.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	services := a.parsedTsuruServices()
	c.Assert(services, gocheck.DeepEquals, map[string][]bind.ServiceInstance{
		"mysql": {
			{
				Name: "mydb",
				Envs: map[string]string{"DATABASE_NAME": "mydb"},
			},
		},
	})
}

func (s *S) TestRemoveInstanceWithUnits(c *gocheck.C) {
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
		Quota: quota.Quota{Limit: 10},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(a)
	defer s.provisioner.Destroy(a)
	err = a.AddUnits(1, nil)
	c.Assert(err, gocheck.IsNil)
	instance := bind.ServiceInstance{Name: "mydb", Envs: map[string]string{"DATABASE_NAME": "mydb"}}
	err = a.RemoveInstance("mysql", instance, nil)
	c.Assert(err, gocheck.IsNil)
	a, err = GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	env, ok := a.Env[TsuruServicesEnvVar]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(env.Value, gocheck.Equals, `{"mysql":[]}`)
	c.Assert(env.Public, gocheck.Equals, false)
	c.Assert(env.Name, gocheck.Equals, TsuruServicesEnvVar)
	delete(a.Env, TsuruServicesEnvVar)
	c.Assert(a.Env, gocheck.DeepEquals, map[string]bind.EnvVar{})
	c.Assert(s.provisioner.Restarts(a), gocheck.Equals, 1)
}

func (s *S) TestIsValid(c *gocheck.C) {
	errMsg := "Invalid app name, your app should have at most 63 characters, containing only lower case letters, numbers or dashes, starting with a letter."
	var data = []struct {
		name     string
		expected string
	}{
		{"myappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyapp", errMsg},
		{"myappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyap", errMsg},
		{"myappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmyappmya", ""},
		{"myApp", errMsg},
		{"my app", errMsg},
		{"123myapp", errMsg},
		{"myapp", ""},
		{"_theirapp", errMsg},
		{"my-app", ""},
		{"-myapp", errMsg},
		{"my_app", errMsg},
		{"b", ""},
		{InternalAppName, errMsg},
	}
	for _, d := range data {
		a := App{Name: d.name}
		if valid := a.validate(); valid != nil && valid.Error() != d.expected {
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
	c.Assert(b.String(), gocheck.Matches, "(?s).*---- Restarting your app ----.*")
	restarts := s.provisioner.Restarts(&a)
	c.Assert(restarts, gocheck.Equals, 1)
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

func (s *S) TestGetUnits(c *gocheck.C) {
	app := App{Name: "app"}
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	s.provisioner.AddUnits(&app, 1, nil)
	c.Assert(app.GetUnits(), gocheck.HasLen, 1)
	c.Assert(app.Units()[0].Ip, gocheck.Equals, app.GetUnits()[0].GetIp())
}

func (s *S) TestAppMarshalJSON(c *gocheck.C) {
	app := App{
		Name:      "name",
		Platform:  "Framework",
		Teams:     []string{"team1"},
		Ip:        "10.10.10.1",
		CName:     []string{"name.mycompany.com"},
		Owner:     "appOwner",
		Deploys:   7,
		Plan:      Plan{Name: "myplan", Memory: 64, Swap: 128, CpuShare: 100},
		TeamOwner: "myteam",
		AutoScaleConfig: &AutoScaleConfig{
			Increase: Action{Units: 1, Expression: "{cpu} > 80"},
			Decrease: Action{Units: 1, Expression: "{cpu} < 20"},
			Enabled:  true,
			MaxUnits: 10,
			MinUnits: 2,
		},
	}
	expected := make(map[string]interface{})
	expected["name"] = "name"
	expected["platform"] = "Framework"
	expected["repository"] = repository.ReadWriteURL(app.Name)
	expected["teams"] = []interface{}{"team1"}
	expected["units"] = nil
	expected["ip"] = "10.10.10.1"
	expected["cname"] = []interface{}{"name.mycompany.com"}
	expected["owner"] = "appOwner"
	expected["deploys"] = float64(7)
	expected["plan"] = map[string]interface{}{"name": "myplan", "memory": float64(64), "swap": float64(128), "cpushare": float64(100)}
	expected["teamowner"] = "myteam"
	expected["ready"] = false
	expected["autoScaleConfig"] = map[string]interface{}{
		"increase": map[string]interface{}{
			"wait":       float64(0),
			"expression": "{cpu} > 80",
			"units":      float64(1),
		},
		"decrease": map[string]interface{}{
			"wait":       float64(0),
			"expression": "{cpu} < 20",
			"units":      float64(1),
		},
		"minUnits": float64(2),
		"maxUnits": float64(10),
		"enabled":  true,
	}
	data, err := app.MarshalJSON()
	c.Assert(err, gocheck.IsNil)
	result := make(map[string]interface{})
	err = json.Unmarshal(data, &result)
	c.Assert(err, gocheck.IsNil)
	autoScaleConfig := result["autoScaleConfig"].(map[string]interface{})
	autoScaleConfigExpected := expected["autoScaleConfig"].(map[string]interface{})
	c.Assert(autoScaleConfig["enabled"], gocheck.Equals, autoScaleConfigExpected["enabled"])
	c.Assert(autoScaleConfig["minUnits"], gocheck.Equals, autoScaleConfigExpected["minUnits"])
	c.Assert(autoScaleConfig["maxUnits"], gocheck.Equals, autoScaleConfigExpected["maxUnits"])
	increase := autoScaleConfig["increase"].(map[string]interface{})
	increaseExpected := autoScaleConfigExpected["increase"].(map[string]interface{})
	c.Assert(increase["expression"], gocheck.Equals, increaseExpected["expression"])
	c.Assert(increase["units"], gocheck.Equals, increaseExpected["units"])
	c.Assert(increase["wait"], gocheck.Equals, increaseExpected["wait"])
	decrease := autoScaleConfig["decrease"].(map[string]interface{})
	decreaseExpected := autoScaleConfigExpected["decrease"].(map[string]interface{})
	c.Assert(decrease["expression"], gocheck.Equals, decreaseExpected["expression"])
	c.Assert(decrease["units"], gocheck.Equals, decreaseExpected["units"])
	c.Assert(decrease["wait"], gocheck.Equals, decreaseExpected["wait"])
}

func (s *S) TestAppMarshalJSONReady(c *gocheck.C) {
	app := App{
		Name:      "name",
		Platform:  "Framework",
		Teams:     []string{"team1"},
		Ip:        "10.10.10.1",
		CName:     []string{"name.mycompany.com"},
		State:     "ready",
		Owner:     "appOwner",
		Deploys:   7,
		TeamOwner: "myteam",
		Plan:      Plan{Name: "myplan", Memory: 64, Swap: 128, CpuShare: 100},
	}
	expected := make(map[string]interface{})
	expected["name"] = "name"
	expected["platform"] = "Framework"
	expected["repository"] = repository.ReadWriteURL(app.Name)
	expected["teams"] = []interface{}{"team1"}
	expected["units"] = nil
	expected["ip"] = "10.10.10.1"
	expected["cname"] = []interface{}{"name.mycompany.com"}
	expected["owner"] = "appOwner"
	expected["deploys"] = float64(7)
	expected["teamowner"] = "myteam"
	expected["autoScaleConfig"] = nil
	expected["plan"] = map[string]interface{}{"name": "myplan", "memory": float64(64), "swap": float64(128), "cpushare": float64(100)}
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
	s.provisioner.AddUnits(&app, 1, nil)
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
	s.provisioner.AddUnits(&app, 1, nil)
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
	s.provisioner.AddUnits(&app, 1, nil)
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

func (s *S) TestListAll(c *gocheck.C) {
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
	apps, err := List(nil)
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
	a := App{Plan: Plan{Memory: 10}}
	c.Assert(a.GetMemory(), gocheck.Equals, a.Plan.Memory)
}

func (s *S) TestGetSwap(c *gocheck.C) {
	a := App{Plan: Plan{Swap: 20}}
	c.Assert(a.GetSwap(), gocheck.Equals, a.Plan.Swap)
}

func (s *S) TestAppUnits(c *gocheck.C) {
	a := App{Name: "anycolor"}
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, nil)
	c.Assert(a.Units(), gocheck.HasLen, 1)
}

func (s *S) TestAppAvailable(c *gocheck.C) {
	a := App{
		Name: "anycolor",
	}
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, nil)
	c.Assert(a.Available(), gocheck.Equals, true)
	s.provisioner.Stop(&a)
	c.Assert(a.Available(), gocheck.Equals, false)
}

func (s *S) TestSwap(c *gocheck.C) {
	var err error
	app1 := &App{Name: "app1", CName: []string{"cname"}}
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
	c.Assert(app1.CName, gocheck.IsNil)
	c.Assert(app2.CName, gocheck.DeepEquals, []string{"cname"})
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

func (s *S) TestAppRegisterUnit(c *gocheck.C) {
	a := App{Name: "appName", Platform: "python"}
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 3, nil)
	units := a.Units()
	var ips []string
	for _, u := range units {
		ips = append(ips, u.Ip)
	}
	err := a.RegisterUnit(units[0].Name)
	c.Assert(err, gocheck.IsNil)
	units = a.Units()
	c.Assert(units[0].Ip, gocheck.Equals, ips[0]+"-updated")
	c.Assert(units[1].Ip, gocheck.Equals, ips[1])
	c.Assert(units[2].Ip, gocheck.Equals, ips[2])
}

func (s *S) TestAppRegisterUnitInvalidUnit(c *gocheck.C) {
	a := App{Name: "appName", Platform: "python"}
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	err := a.RegisterUnit("oddity")
	c.Assert(err, gocheck.Equals, ErrUnitNotFound)
}

func (s *S) TestAppValidateTeamOwner(c *gocheck.C) {
	team := auth.Team{Name: "test", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(team)
	defer s.conn.Teams().Remove(bson.M{"_id": team.Name})
	c.Assert(err, gocheck.IsNil)
	a := App{Name: "test", Platform: "python", TeamOwner: team.Name}
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	err = a.ValidateTeamOwner(s.user)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestAppValidateTeamOwnerSetAnTeamWhichNotExistsAndUserIsAdmin(c *gocheck.C) {
	a := App{Name: "test", Platform: "python", TeamOwner: "not-exists"}
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	err := a.ValidateTeamOwner(s.admin)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.Equals, auth.ErrTeamNotFound)
}

func (s *S) TestAppValidateTeamOwnerToUserWhoIsNotThatTeam(c *gocheck.C) {
	team := auth.Team{Name: "test"}
	err := s.conn.Teams().Insert(team)
	defer s.conn.Teams().Remove(bson.M{"_id": team.Name})
	c.Assert(err, gocheck.IsNil)
	a := App{Name: "test", Platform: "python", TeamOwner: team.Name}
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	err = a.ValidateTeamOwner(s.user)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "You can not set test team as app's owner. Please set one of your teams as app's owner.")
}

func (s *S) TestAppValidateTeamOwnerAdminCanSetAppToAnyTeam(c *gocheck.C) {
	admin := &auth.User{Email: "admin@a.com"}
	teamAdmin := auth.Team{Name: "admin", Users: []string{admin.Email}}
	err := s.conn.Teams().Insert(teamAdmin)
	defer s.conn.Teams().Remove(bson.M{"_id": teamAdmin.Name})
	c.Assert(err, gocheck.IsNil)
	team := auth.Team{Name: "test"}
	err = s.conn.Teams().Insert(team)
	defer s.conn.Teams().Remove(bson.M{"_id": team.Name})
	c.Assert(err, gocheck.IsNil)
	a := App{Name: "test", Platform: "python", TeamOwner: team.Name}
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	err = a.ValidateTeamOwner(admin)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestUpdateCustomData(c *gocheck.C) {
	a := App{Name: "my-test-app"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	customData := map[string]interface{}{
		"hooks": map[string]interface{}{
			"build": []interface{}{"a", "b"},
		},
	}
	err = a.UpdateCustomData(customData)
	c.Assert(err, gocheck.IsNil)
	dbApp, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dbApp.CustomData, gocheck.DeepEquals, customData)
}

func (s *S) TestGetTsuruYamlData(c *gocheck.C) {
	a := App{Name: "my-test-app"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	customData := map[string]interface{}{
		"hooks": map[string]interface{}{
			"restart": map[string]interface{}{
				"before": []interface{}{"rb1", "rb2"},
				"after":  []interface{}{"ra1", "ra2"},
			},
			"build": []interface{}{"ba1", "ba2"},
		},
		"healthcheck": map[string]interface{}{
			"path":             "/test",
			"method":           "PUT",
			"status":           200,
			"match":            ".*a.*",
			"allowed_failures": 10,
		},
	}
	err = a.UpdateCustomData(customData)
	c.Assert(err, gocheck.IsNil)
	yamlData, err := a.GetTsuruYamlData()
	c.Assert(err, gocheck.IsNil)
	c.Assert(yamlData, gocheck.DeepEquals, TsuruYamlData{
		Hooks: TsuruYamlHooks{
			Restart: TsuruYamlRestartHooks{
				Before: []string{"rb1", "rb2"},
				After:  []string{"ra1", "ra2"},
			},
			Build: []string{"ba1", "ba2"},
		},
		Healthcheck: TsuruYamlHealthcheck{
			Path:            "/test",
			Method:          "PUT",
			Status:          200,
			Match:           ".*a.*",
			AllowedFailures: 10,
		},
	})
}

func (s *S) TestSshToAnApp(c *gocheck.C) {
	a := App{Name: "my-test-app"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	buf := safe.NewBuffer([]byte("echo teste"))
	conn := &testing.FakeConn{buf}
	err = a.Ssh(conn, 10, 10)
	c.Assert(err, gocheck.IsNil)
}
