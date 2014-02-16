// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"encoding/json"
	stderr "errors"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/queue"
	"github.com/globocom/tsuru/quota"
	"github.com/globocom/tsuru/repository"
	"github.com/globocom/tsuru/service"
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
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
		Units:    []Unit{{Name: "duvido", Machine: 3}},
		Owner:    s.user.Email,
	}
	err := s.conn.Apps().Insert(&a)
	c.Assert(err, gocheck.IsNil)
	app, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	err = Delete(app)
	c.Assert(err, gocheck.IsNil)
	_, err = GetByName(app.Name)
	c.Assert(err, gocheck.NotNil)
	c.Assert(s.provisioner.Provisioned(&a), gocheck.Equals, false)
	err = auth.ReserveApp(s.user)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestDestroy(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	a := App{
		Name:     "ritual",
		Platform: "python",
		Units: []Unit{
			{
				Name:    "duvido",
				Machine: 3,
			},
		},
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, gocheck.IsNil)
	app, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	token := app.Env["TSURU_APP_TOKEN"].Value
	err = Delete(app)
	c.Assert(err, gocheck.IsNil)
	_, err = GetByName(app.Name)
	c.Assert(err, gocheck.NotNil)
	c.Assert(s.provisioner.Provisioned(&a), gocheck.Equals, false)
	msg, err := aqueue().Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(msg.Args, gocheck.DeepEquals, []string{app.Name})
	_, err = auth.GetToken("bearer " + token)
	c.Assert(err, gocheck.Equals, auth.ErrInvalidToken)
}

func (s *S) TestDestroyWithoutBucketSupport(c *gocheck.C) {
	config.Unset("bucket-support")
	defer config.Set("bucket-support", true)
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	a := App{
		Name:     "blinded",
		Platform: "python",
		Units:    []Unit{{Name: "duvido", Machine: 3}},
	}
	err := CreateApp(&a, s.user)
	c.Assert(err, gocheck.IsNil)
	app, err := GetByName(a.Name)
	err = Delete(app)
	c.Assert(err, gocheck.IsNil)
	_, err = GetByName(app.Name)
	c.Assert(err, gocheck.NotNil)
	c.Assert(s.provisioner.Provisioned(&a), gocheck.Equals, false)
	msg, err := aqueue().Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(msg.Args, gocheck.DeepEquals, []string{app.Name})
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
	msg, err := aqueue().Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(msg.Args, gocheck.DeepEquals, []string{a.Name})
}

func (s *S) TestCreateApp(c *gocheck.C) {
	patchRandomReader()
	defer unpatchRandomReader()
	ts := testing.StartGandalfTestServer(&testHandler{})
	defer ts.Close()
	a := App{
		Name:     "appname",
		Platform: "python",
		Units:    []Unit{{Machine: 3}},
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
	env := retrievedApp.InstanceEnv(s3InstanceName)
	c.Assert(env["TSURU_S3_ENDPOINT"].Value, gocheck.Equals, s.t.S3Server.URL())
	c.Assert(env["TSURU_S3_ENDPOINT"].Public, gocheck.Equals, false)
	c.Assert(env["TSURU_S3_LOCATIONCONSTRAINT"].Value, gocheck.Equals, "true")
	c.Assert(env["TSURU_S3_LOCATIONCONSTRAINT"].Public, gocheck.Equals, false)
	e, ok := env["TSURU_S3_ACCESS_KEY_ID"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Public, gocheck.Equals, false)
	e, ok = env["TSURU_S3_SECRET_KEY"]
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Public, gocheck.Equals, false)
	c.Assert(env["TSURU_S3_BUCKET"].Value, gocheck.HasLen, maxBucketSize)
	c.Assert(env["TSURU_S3_BUCKET"].Value, gocheck.Equals, "appnamee3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3")
	c.Assert(env["TSURU_S3_BUCKET"].Public, gocheck.Equals, false)
	env = retrievedApp.InstanceEnv("")
	c.Assert(env["TSURU_APPNAME"].Value, gocheck.Equals, a.Name)
	c.Assert(env["TSURU_APPNAME"].Public, gocheck.Equals, false)
	c.Assert(env["TSURU_HOST"].Value, gocheck.Equals, expectedHost)
	c.Assert(env["TSURU_HOST"].Public, gocheck.Equals, false)
	expectedMessage := queue.Message{
		Action: regenerateApprc,
		Args:   []string{a.Name},
	}
	message, err := aqueue().Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(message.Action, gocheck.Equals, expectedMessage.Action)
	c.Assert(message.Args, gocheck.DeepEquals, expectedMessage.Args)
	c.Assert(s.provisioner.GetUnits(&a), gocheck.HasLen, 1)
	err = auth.ReserveApp(s.user)
	_, ok = err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
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

func (s *S) TestCreateWithoutBucketSupport(c *gocheck.C) {
	config.Unset("bucket-support")
	defer config.Set("bucket-support", true)
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	a := App{
		Name:     "sorry",
		Platform: "python",
		Units:    []Unit{{Machine: 3}},
	}
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	err := CreateApp(&a, s.user)
	c.Assert(err, gocheck.IsNil)
	defer Delete(&a)
	retrievedApp, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(retrievedApp.Name, gocheck.Equals, a.Name)
	c.Assert(retrievedApp.Platform, gocheck.Equals, a.Platform)
	c.Assert(retrievedApp.Teams, gocheck.DeepEquals, []string{s.team.Name})
	env := a.InstanceEnv(s3InstanceName)
	c.Assert(env, gocheck.DeepEquals, map[string]bind.EnvVar{})
	message, err := aqueue().Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(message.Action, gocheck.Equals, regenerateApprc)
	c.Assert(message.Args, gocheck.DeepEquals, []string{a.Name})
	c.Assert(s.provisioner.GetUnits(&a), gocheck.HasLen, 1)
}

func (s *S) TestCannotCreateAppWithUnknownPlatform(c *gocheck.C) {
	a := App{Name: "paradisum", Platform: "unknown"}
	err := CreateApp(&a, s.user)
	_, ok := err.(InvalidPlatformError)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestCannotCreateAppWithoutTeams(c *gocheck.C) {
	u := auth.User{Email: "perpetual@yes.com", Password: "123678"}
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
	c.Assert(e.Err.Error(), gocheck.Equals, "there is already an app with this name.")
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
		Units:    []Unit{{Machine: 1}},
	}
	err := CreateApp(&a, s.user)
	defer Delete(&a) // clean mess if test fail
	c.Assert(err, gocheck.NotNil)
	expected := `Tsuru failed to create the app "theirapp": exit status 1`
	c.Assert(err.Error(), gocheck.Equals, expected)
	_, err = GetByName(a.Name)
	c.Assert(err, gocheck.NotNil)
	msg, err := aqueue().Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(msg.Args, gocheck.DeepEquals, []string{a.Name})
}

func (s *S) TestDeletesIAMCredentialsAndS3BucketIfProvisionerFail(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	s.provisioner.PrepareFailure("Provision", stderr.New("exit status 1"))
	source := patchRandomReader()
	defer unpatchRandomReader()
	a := App{
		Name:     "theirapp",
		Platform: "python",
		Units:    []Unit{{Machine: 1}},
	}
	err := CreateApp(&a, s.user)
	defer Delete(&a) // clean mess if test fail
	c.Assert(err, gocheck.NotNil)
	iam := getIAMEndpoint()
	_, err = iam.GetUser("theirapp")
	c.Assert(err, gocheck.NotNil)
	s3 := getS3Endpoint()
	bucketName := fmt.Sprintf("%s%x", a.Name, source[:(maxBucketSize-len(a.Name)/2)])
	bucket := s3.Bucket(bucketName)
	_, err = bucket.Get("")
	c.Assert(err, gocheck.NotNil)
	msg, err := aqueue().Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(msg.Args, gocheck.DeepEquals, []string{a.Name})
}

func (s *S) TestCreateAppCreatesRepositoryInGandalf(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	a := App{
		Name:     "someapp",
		Platform: "python",
		Teams:    []string{s.team.Name},
		Units:    []Unit{{Machine: 3}},
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
	msg, err := aqueue().Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(msg.Args, gocheck.DeepEquals, []string{app.Name})
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
	msg, err := aqueue().Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(msg.Args, gocheck.DeepEquals, []string{a.Name})
}

func (s *S) TestAppendOrUpdate(c *gocheck.C) {
	a := App{
		Name:     "appName",
		Platform: "django",
	}
	u := Unit{Name: "i-00000zz8", Ip: "", Machine: 1}
	a.AddUnit(&u)
	c.Assert(len(a.Units), gocheck.Equals, 1)
	u = Unit{
		Name: "i-00000zz8",
		Ip:   "192.168.0.12",
	}
	a.AddUnit(&u)
	c.Assert(len(a.Units), gocheck.Equals, 1)
	c.Assert(a.Units[0], gocheck.DeepEquals, u)
}

func (s *S) TestAddUnitPlaceHolder(c *gocheck.C) {
	a := App{
		Name:     "appName",
		Platform: "django",
		Units:    []Unit{{}},
	}
	u := Unit{Name: "i-000000zzz8", Machine: 1}
	a.AddUnit(&u)
	c.Assert(len(a.Units), gocheck.Equals, 1)
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
	otherApp := App{Name: "warpaint"}
	err = otherApp.AddUnits(5)
	c.Assert(err, gocheck.IsNil)
	units := s.provisioner.GetUnits(&app)
	c.Assert(units, gocheck.HasLen, 6)
	err = otherApp.AddUnits(2)
	c.Assert(err, gocheck.IsNil)
	units = s.provisioner.GetUnits(&app)
	c.Assert(units, gocheck.HasLen, 8)
	for _, unit := range units {
		c.Assert(unit.AppName, gocheck.Equals, app.Name)
	}
	gotApp, err := GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(gotApp.Units, gocheck.HasLen, 7)
	c.Assert(gotApp.Platform, gocheck.Equals, "python")
	var expectedMessages MessageList
	for i, unit := range gotApp.Units {
		expectedName := fmt.Sprintf("%s/%d", app.Name, i+1)
		c.Check(unit.Name, gocheck.Equals, expectedName)
		messages := []queue.Message{
			{Action: regenerateApprc, Args: []string{gotApp.Name, unit.Name}},
			{Action: BindService, Args: []string{gotApp.Name, unit.Name}},
		}
		expectedMessages = append(expectedMessages, messages...)
	}
	gotMessages := make(MessageList, expectedMessages.Len())
	for i := range expectedMessages {
		message, err := aqueue().Get(1e6)
		c.Check(err, gocheck.IsNil)
		gotMessages[i] = queue.Message{
			Action: message.Action,
			Args:   message.Args,
		}
	}
	sort.Sort(expectedMessages)
	sort.Sort(gotMessages)
	c.Assert(gotMessages, gocheck.DeepEquals, expectedMessages)
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
	c.Assert(units, gocheck.HasLen, 6)
	err = otherApp.AddUnits(2)
	c.Assert(err, gocheck.IsNil)
	units = s.provisioner.GetUnits(&app)
	c.Assert(units, gocheck.HasLen, 8)
	for _, unit := range units {
		c.Assert(unit.AppName, gocheck.Equals, app.Name)
	}
	gotApp, err := GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(gotApp.Units, gocheck.HasLen, 7)
	c.Assert(gotApp.Platform, gocheck.Equals, "python")
	var expectedMessages MessageList
	for i, unit := range gotApp.Units {
		expectedName := fmt.Sprintf("%s/%d", app.Name, i+1)
		c.Assert(unit.Name, gocheck.Equals, expectedName)
		messages := []queue.Message{
			{Action: regenerateApprc, Args: []string{gotApp.Name, unit.Name}},
			{Action: BindService, Args: []string{gotApp.Name, unit.Name}},
		}
		expectedMessages = append(expectedMessages, messages...)
	}
	gotMessages := make(MessageList, expectedMessages.Len())
	for i := range expectedMessages {
		message, err := aqueue().Get(1e6)
		c.Assert(err, gocheck.IsNil)
		gotMessages[i] = queue.Message{
			Action: message.Action,
			Args:   message.Args,
		}
	}
	sort.Sort(expectedMessages)
	sort.Sort(gotMessages)
	c.Assert(gotMessages, gocheck.DeepEquals, expectedMessages)
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

type hasUnitChecker struct{}

func (c *hasUnitChecker) Info() *gocheck.CheckerInfo {
	return &gocheck.CheckerInfo{Name: "HasUnit", Params: []string{"app", "unit"}}
}

func (c *hasUnitChecker) Check(params []interface{}, names []string) (bool, string) {
	a, ok := params[0].(*App)
	if !ok {
		return false, "first parameter should be a pointer to an app instance"
	}
	u, ok := params[1].(provision.AppUnit)
	if !ok {
		return false, "second parameter should be a pointer to an unit instance"
	}
	for _, unit := range a.ProvisionedUnits() {
		if reflect.DeepEqual(unit, u) {
			return true, ""
		}
	}
	return false, ""
}

var HasUnit gocheck.Checker = &hasUnitChecker{}

func (s *S) TestRemoveUnitsPriority(c *gocheck.C) {
	unitList := []Unit{
		{Name: "ble/0", State: provision.StatusStarted.String()},
		{Name: "ble/1", State: provision.StatusDown.String()},
		{Name: "ble/2", State: provision.StatusBuilding.String()},
	}
	a := App{Name: "ble", Units: unitList}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	s.provisioner.AddUnits(&a, 3)
	units := a.ProvisionedUnits()
	for _, unit := range units {
		c.Assert(&a, HasUnit, unit)
	}
	removeUnits := []int{1, 2}
	for _, unit := range removeUnits {
		err = a.RemoveUnits(1)
		c.Assert(err, gocheck.IsNil)
		c.Assert(&a, gocheck.Not(HasUnit), units[unit])
	}
	c.Assert(&a, HasUnit, units[0])
	c.Assert(a.ProvisionedUnits(), gocheck.HasLen, 1)
}

func (s *S) TestRemoveUnitsWithQuota(c *gocheck.C) {
	units := []Unit{
		{Name: "ble/0", State: provision.StatusStarted.String()},
		{Name: "ble/1", State: provision.StatusDown.String()},
		{Name: "ble/2", State: provision.StatusBuilding.String()},
		{Name: "ble/3", State: provision.StatusBuilding.String()},
		{Name: "ble/4", State: provision.StatusStarted.String()},
		{Name: "ble/5", State: provision.StatusBuilding.String()},
	}
	a := App{
		Name: "ble", Units: units,
		Quota: quota.Quota{Limit: 6, InUse: 6},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	s.provisioner.AddUnits(&a, 6)
	defer s.provisioner.Destroy(&a)
	err = a.RemoveUnits(4)
	c.Assert(err, gocheck.IsNil)
	app, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Quota.InUse, gocheck.Equals, 2)
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
	defer testing.CleanQ(queueName)
	gotApp, err := GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	otherApp := App{Name: gotApp.Name, Units: gotApp.Units}
	err = otherApp.RemoveUnits(2)
	c.Assert(err, gocheck.IsNil)
	ts.Close()
	units := s.provisioner.GetUnits(&app)
	c.Assert(units, gocheck.HasLen, 3)
	c.Assert(units[0].Name, gocheck.Equals, "chemistry/0")
	c.Assert(units[1].Name, gocheck.Equals, "chemistry/3")
	c.Assert(units[2].Name, gocheck.Equals, "chemistry/4")
	gotApp, err = GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(gotApp.Platform, gocheck.Equals, "python")
	c.Assert(gotApp.Units, gocheck.HasLen, 2)
	c.Assert(gotApp.Units[0].Name, gocheck.Equals, "chemistry/3")
	c.Assert(gotApp.Units[1].Name, gocheck.Equals, "chemistry/4")
	c.Assert(gotApp.Quota.InUse, gocheck.Equals, 2)
}

func (s *S) TestRemoveUnitsInvalidValues(c *gocheck.C) {
	var tests = []struct {
		n        uint
		expected string
	}{
		{0, "Cannot remove zero units."},
		{4, "Cannot remove all units from an app."},
		{5, "Cannot remove 5 units from this app, it has only 4 units."},
	}
	app := App{
		Name:     "chemistry",
		Platform: "python",
		Units: []Unit{
			{Name: "chemistry/0"},
			{Name: "chemistry/1"},
			{Name: "chemistry/2"},
			{Name: "chemistry/3"},
		},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	s.provisioner.AddUnits(&app, 4)
	for _, test := range tests {
		err := app.RemoveUnits(test.n)
		c.Check(err, gocheck.NotNil)
		c.Check(err.Error(), gocheck.Equals, test.expected)
	}
}

func (s *S) TestRemoveUnitsFromIndicesSlice(c *gocheck.C) {
	var tests = []struct {
		input    []Unit
		indices  []int
		expected []Unit
	}{
		{
			input:    []Unit{{Name: "unit1"}, {Name: "unit2"}, {Name: "unit3"}, {Name: "unit4"}},
			indices:  []int{0, 1, 2},
			expected: []Unit{{Name: "unit4"}},
		},
		{
			input:    []Unit{{Name: "unit1"}, {Name: "unit2"}, {Name: "unit3"}, {Name: "unit4"}},
			indices:  []int{0, 3, 4},
			expected: []Unit{{Name: "unit2"}},
		},
		{
			input:    []Unit{{Name: "unit1"}, {Name: "unit2"}, {Name: "unit3"}, {Name: "unit4"}},
			indices:  []int{4},
			expected: []Unit{{Name: "unit1"}, {Name: "unit2"}, {Name: "unit3"}},
		},
	}
	for _, t := range tests {
		a := App{Units: t.input}
		a.removeUnits(t.indices)
		c.Check(a.Units, gocheck.DeepEquals, t.expected)
	}
}

func (s *S) TestRemoveUnitByNameOrInstanceID(c *gocheck.C) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "nosql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "nosql"})
	app := App{
		Name:     "physics",
		Platform: "python",
		Units: []Unit{
			{Name: "physics/0"},
		},
		Quota: quota.Unlimited,
	}
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "nosql",
		Teams:       []string{s.team.Name},
		Apps:        []string{app.Name},
	}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	err = s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	err = s.provisioner.Provision(&app)
	c.Assert(err, gocheck.IsNil)
	err = app.AddUnits(4)
	c.Assert(err, gocheck.IsNil)
	defer testing.CleanQ(queueName)
	defer func() {
		s.provisioner.Destroy(&app)
		s.conn.Apps().Remove(bson.M{"name": app.Name})
	}()
	otherApp, err := GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	err = otherApp.RemoveUnit(app.Units[0].Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(otherApp.Platform, gocheck.Equals, "python")
	c.Assert(otherApp.Units, gocheck.HasLen, 4)
	c.Assert(otherApp.Units[0].Name, gocheck.Equals, "physics/1")
	c.Assert(otherApp.Units[1].Name, gocheck.Equals, "physics/2")
	c.Assert(otherApp.Units[2].Name, gocheck.Equals, "physics/3")
	c.Assert(otherApp.Units[3].Name, gocheck.Equals, "physics/4")
	err = otherApp.RemoveUnit(otherApp.Units[1].InstanceId)
	c.Assert(err, gocheck.IsNil)
	units := s.provisioner.GetUnits(otherApp)
	c.Assert(units, gocheck.HasLen, 3)
	c.Assert(units[0].Name, gocheck.Equals, "physics/1")
	c.Assert(units[1].Name, gocheck.Equals, "physics/3")
	c.Assert(units[2].Name, gocheck.Equals, "physics/4")
	ok := make(chan int8)
	go func() {
		for _ = range time.Tick(1e3) {
			if atomic.LoadInt32(&calls) == 2 {
				ok <- 1
				return
			}
		}
	}()
	select {
	case <-ok:
	case <-time.After(2e9):
		c.Fatal("Did not call service endpoint twice.")
	}
}

func (s *S) TestRemoveAbsentUnit(c *gocheck.C) {
	app := &App{
		Name:     "chemistry",
		Platform: "python",
		Units: []Unit{
			{Name: "chemistry/0"},
		},
		Quota: quota.Unlimited,
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	err = s.provisioner.Provision(app)
	c.Assert(err, gocheck.IsNil)
	err = app.AddUnits(1)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		s.provisioner.Destroy(app)
		s.conn.Apps().Remove(bson.M{"name": app.Name})
	}()
	app, err = GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	instID := app.Units[1].InstanceId
	err = app.RemoveUnit(instID)
	c.Assert(err, gocheck.IsNil)
	err = app.RemoveUnit(instID)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.ErrorMatches, "Unit not found.")
	app, err = GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Units, gocheck.HasLen, 1)
	c.Assert(app.Units[0].Name, gocheck.Equals, "chemistry/0")
	units := s.provisioner.GetUnits(app)
	c.Assert(units, gocheck.HasLen, 1)
	c.Assert(units[0].Name, gocheck.Equals, "chemistry/0")
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
		Name:  "myapp",
		Units: []Unit{{Machine: 1}},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST": {
				Name:   "DATABASE_HOST",
				Value:  "localhost",
				Public: false,
			},
		},
	}
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
	err = a.setEnvsToApp(envs, true, false)
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
}

func (s *S) TestSetEnvRespectsThePublicOnlyFlagOverwrittenAllVariablesWhenItsFalse(c *gocheck.C) {
	a := App{
		Name: "myapp",
		Units: []Unit{
			{Machine: 1},
		},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST": {
				Name:   "DATABASE_HOST",
				Value:  "localhost",
				Public: false,
			},
		},
	}
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
	err = a.setEnvsToApp(envs, false, false)
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
}

func (s *S) TestUnsetEnvRespectsThePublicOnlyFlagKeepPrivateVariablesWhenItsTrue(c *gocheck.C) {
	a := App{
		Name: "myapp",
		Units: []Unit{
			{Machine: 1},
		},
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
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
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
}

func (s *S) TestUnsetEnvRespectsThePublicOnlyFlagUnsettingAllVariablesWhenItsFalse(c *gocheck.C) {
	a := App{
		Name: "myapp",
		Units: []Unit{
			{Machine: 1},
		},
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
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = a.UnsetEnvs([]string{"DATABASE_HOST", "DATABASE_PASSWORD"}, false)
	c.Assert(err, gocheck.IsNil)
	newApp, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(newApp.Env, gocheck.DeepEquals, map[string]bind.EnvVar{})
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
		Units:    []Unit{{Name: "i-0800", State: "started"}},
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

func (s *S) TestLog(c *gocheck.C) {
	a := App{Name: "newApp"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Logs().Remove(bson.M{"appname": a.Name})
	}()
	err = a.Log("last log msg", "tsuru")
	c.Assert(err, gocheck.IsNil)
	var logs []Applog
	err = s.conn.Logs().Find(bson.M{"appname": a.Name}).All(&logs)
	c.Assert(err, gocheck.IsNil)
	c.Assert(logs, gocheck.HasLen, 1)
	c.Assert(logs[0].Message, gocheck.Equals, "last log msg")
	c.Assert(logs[0].Source, gocheck.Equals, "tsuru")
	c.Assert(logs[0].AppName, gocheck.Equals, a.Name)
}

func (s *S) TestLogShouldAddOneRecordByLine(c *gocheck.C) {
	a := App{Name: "newApp"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": a.Name})
		s.conn.Logs().Remove(bson.M{"appname": a.Name})
	}()
	err = a.Log("last log msg\nfirst log", "source")
	c.Assert(err, gocheck.IsNil)
	var logs []Applog
	err = s.conn.Logs().Find(bson.M{"appname": a.Name}).Sort("$natural").All(&logs)
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
	err = a.Log("some message", "tsuru")
	c.Assert(err, gocheck.IsNil)
	err = a.Log("", "")
	c.Assert(err, gocheck.IsNil)
	count, err := s.conn.Logs().Find(bson.M{"appname": a.Name}).Count()
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
	l := NewLogListener(&a)
	defer l.Close()
	go func() {
		for log := range l.C {
			logs.Lock()
			logs.l = append(logs.l, log)
			logs.Unlock()
		}
	}()
	err = a.Log("last log msg", "tsuru")
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Logs().Remove(bson.M{"appname": a.Name})
	done := make(chan bool, 1)
	q := make(chan bool)
	go func(quit chan bool) {
		for _ = range time.Tick(1e3) {
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
		app.Log(strconv.Itoa(i), "tsuru")
		time.Sleep(1e6) // let the time flow
	}
	app.Log("app3 log from circus", "circus")
	logs, err := app.LastLogs(10, "tsuru")
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
	logs, err := app.LastLogs(10, "tsuru")
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
	ips := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"}
	units := make([]Unit, len(ips))
	got := make([]string, len(ips))
	for i, ip := range ips {
		unit := Unit{Ip: ip}
		units[i] = unit
	}
	app := App{Units: units}
	for i, unit := range app.GetUnits() {
		got[i] = unit.GetIp()
	}
	c.Assert(got, gocheck.DeepEquals, ips)
}

func (s *S) TestAppMarshalJSON(c *gocheck.C) {
	app := App{
		Name:     "name",
		Platform: "Framework",
		Teams:    []string{"team1"},
		Ip:       "10.10.10.1",
		CName:    "name.mycompany.com",
	}
	expected := make(map[string]interface{})
	expected["name"] = "name"
	expected["platform"] = "Framework"
	expected["repository"] = repository.ReadWriteURL(app.Name)
	expected["teams"] = []interface{}{"team1"}
	expected["units"] = nil
	expected["ip"] = "10.10.10.1"
	expected["cname"] = "name.mycompany.com"
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
	}
	expected := make(map[string]interface{})
	expected["name"] = "name"
	expected["platform"] = "Framework"
	expected["repository"] = repository.ReadWriteURL(app.Name)
	expected["teams"] = []interface{}{"team1"}
	expected["units"] = nil
	expected["ip"] = "10.10.10.1"
	expected["cname"] = "name.mycompany.com"
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
		Name:  "myapp",
		Units: []Unit{{Name: "i-0800", State: "started"}},
	}
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
		Units: []Unit{
			{Name: "i-0800", State: "started"},
			{Name: "i-0801", State: "started"},
		},
	}
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
		Name:  "myapp",
		Units: []Unit{{Name: "i-0800", State: "started"}},
	}
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

func (s *S) TestSerializeEnvVars(c *gocheck.C) {
	s.provisioner.PrepareOutput([]byte("exported"))
	app := App{
		Name:  "time",
		Teams: []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"http_proxy": {
				Name:   "http_proxy",
				Value:  "http://theirproxy.com:3128/",
				Public: true,
			},
		},
		Units: []Unit{{Name: "i-0800", State: "started"}},
	}
	err := app.SerializeEnvVars()
	c.Assert(err, gocheck.IsNil)
	cmds := s.provisioner.GetCmds("", &app)
	c.Assert(cmds, gocheck.HasLen, 1)
	cmdRegexp := `^cat > /home/application/apprc <<END # generated by tsuru .*`
	cmdRegexp += ` export http_proxy="http://theirproxy.com:3128/" END $`
	cmd := strings.Replace(cmds[0].Cmd, "\n", " ", -1)
	c.Assert(cmd, gocheck.Matches, cmdRegexp)
}

func (s *S) TestSerializeEnvVarsErrorWithoutOutput(c *gocheck.C) {
	s.provisioner.PrepareFailure("ExecuteCommand", stderr.New("Failed to run commands"))
	app := App{
		Name: "intheend",
		Env: map[string]bind.EnvVar{
			"https_proxy": {
				Name:   "https_proxy",
				Value:  "https://secureproxy.com:3128/",
				Public: true,
			},
		},
		Units: []Unit{
			{Name: "i-0801", State: "started"},
		},
	}
	err := app.SerializeEnvVars()
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Failed to write env vars: Failed to run commands.")
}

func (s *S) TestSerializeEnvVarsErrorWithOutput(c *gocheck.C) {
	s.provisioner.PrepareOutput([]byte("This program has performed an illegal operation"))
	s.provisioner.PrepareFailure("ExecuteCommand", stderr.New("exit status 1"))
	app := App{
		Name: "intheend",
		Env: map[string]bind.EnvVar{
			"https_proxy": {
				Name:   "https_proxy",
				Value:  "https://secureproxy.com:3128/",
				Public: true,
			},
		},
		Units: []Unit{{Name: "i-0800", State: "started"}},
	}
	err := app.SerializeEnvVars()
	c.Assert(err, gocheck.NotNil)
	expected := "Failed to write env vars (exit status 1): This program has performed an illegal operation."
	c.Assert(err.Error(), gocheck.Equals, expected)
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

func (s *S) TestListAppDeploys(c *gocheck.C) {
	s.conn.Deploys().RemoveAll(nil)
	a := App{Name: "g1"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	insert := []interface{}{
		Deploy{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		Deploy{App: "g1", Timestamp: time.Now()},
	}
	s.conn.Deploys().Insert(insert...)
	defer s.conn.Deploys().RemoveAll(bson.M{"app": a.Name})
	expected := []Deploy{insert[1].(Deploy), insert[0].(Deploy)}
	deploys, err := a.ListDeploys()
	c.Assert(err, gocheck.IsNil)
	for i := 0; i < 2; i++ {
		ts := expected[i].Timestamp
		expected[i].Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
		ts = deploys[i].Timestamp
		deploys[i].Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
	}
	c.Assert(deploys, gocheck.DeepEquals, expected)
}

func (s *S) TestListAllDeploys(c *gocheck.C) {
	s.conn.Deploys().RemoveAll(nil)
	insert := []interface{}{
		Deploy{App: "g1", Timestamp: time.Now().Add(-3600 * time.Second)},
		Deploy{App: "ge", Timestamp: time.Now()},
	}
	s.conn.Deploys().Insert(insert...)
	defer s.conn.Deploys().RemoveAll(nil)
	expected := []Deploy{insert[1].(Deploy), insert[0].(Deploy)}
	deploys, err := ListDeploys()
	c.Assert(err, gocheck.IsNil)
	for i := 0; i < 2; i++ {
		ts := expected[i].Timestamp
		expected[i].Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
		ts = deploys[i].Timestamp
		deploys[i].Timestamp = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC)
	}
	c.Assert(deploys, gocheck.DeepEquals, expected)
}

func (s *S) TestGetProvisionedUnits(c *gocheck.C) {
	a := App{Name: "anycolor", Units: []Unit{{Name: "i-0800"}, {Name: "i-0900"}, {Name: "i-a00"}}}
	gotUnits := a.ProvisionedUnits()
	for i := range a.Units {
		if gotUnits[i].GetName() != a.Units[i].Name {
			c.Errorf("Failed at position %d: Want %q. Got %q.", i, a.Units[i].Name, gotUnits[i].GetName())
		}
	}
}

func (s *S) TestAppAvailableShouldReturnsTrueWhenOneUnitIsStarted(c *gocheck.C) {
	a := App{
		Name: "anycolor",
		Units: []Unit{
			{Name: "i-0800", State: "started"},
			{Name: "i-0900", State: "pending"},
			{Name: "i-a00", State: "stopped"},
		},
	}
	c.Assert(a.Available(), gocheck.Equals, true)
}

func (s *S) TestAppAvailableShouldReturnsTrueWhenOneUnitIsUnreachable(c *gocheck.C) {
	a := App{
		Name: "anycolor",
		Units: []Unit{
			{Name: "i-0800", State: "unreachable"},
			{Name: "i-0900", State: "pending"},
			{Name: "i-a00", State: "stopped"},
		},
	}
	c.Assert(a.Available(), gocheck.Equals, true)
}

func (s *S) TestSwap(c *gocheck.C) {
	app1 := &App{Name: "app1", CName: "cname"}
	err := s.conn.Apps().Insert(app1)
	c.Assert(err, gocheck.IsNil)
	app2 := &App{Name: "app2"}
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
}

func (s *S) TestDeployApp(c *gocheck.C) {
	a := App{
		Name:     "someApp",
		Platform: "django",
		Teams:    []string{s.team.Name},
		Units:    []Unit{{Name: "i-0800", State: "started"}},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	err = DeployApp(&a, "version", writer)
	c.Assert(err, gocheck.IsNil)
	logs := writer.String()
	c.Assert(logs, gocheck.Equals, "Deploy called")
}

func (s *S) TestDeployAppIncrementDeployNumber(c *gocheck.C) {
	a := App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Units:    []Unit{{Name: "i-0800", State: "started"}},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	err = DeployApp(&a, "version", writer)
	c.Assert(err, gocheck.IsNil)
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(a.Deploys, gocheck.Equals, uint(1))
}

func (s *S) TestDeployAppSaveDeployData(c *gocheck.C) {
	a := App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Units:    []Unit{{Name: "i-0800", State: "started"}},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	err = DeployApp(&a, "version", writer)
	c.Assert(err, gocheck.IsNil)
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(a.Deploys, gocheck.Equals, uint(1))
	var result map[string]interface{}
	s.conn.Deploys().Find(bson.M{"app": a.Name}).One(&result)
	c.Assert(result["app"], gocheck.Equals, a.Name)
	now := time.Now()
	diff := now.Sub(result["timestamp"].(time.Time))
	c.Assert(diff < 60*time.Second, gocheck.Equals, true)
	c.Assert(result["duration"], gocheck.Not(gocheck.Equals), 0)
}

func (s *S) TestDeployCustomPipeline(c *gocheck.C) {
	a := App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Units:    []Unit{{Name: "i-0800", State: "started"}},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	err = DeployApp(&a, "version", writer)
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.provisioner.ExecutedPipeline(), gocheck.Equals, false)
	s.provisioner.CustomPipeline = true
	err = DeployApp(&a, "version", writer)
	c.Assert(err, gocheck.IsNil)
	c.Assert(s.provisioner.ExecutedPipeline(), gocheck.Equals, true)
}

func (s *S) TestStart(c *gocheck.C) {
	s.provisioner.PrepareOutput([]byte("not yaml")) // loadConf
	a := App{
		Name:     "someApp",
		Platform: "django",
		Teams:    []string{s.team.Name},
		Units:    []Unit{{Name: "i-0800", State: "started"}},
	}
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	var b bytes.Buffer
	err := a.Start(&b)
	c.Assert(err, gocheck.IsNil)
	starts := s.provisioner.Starts(&a)
	c.Assert(starts, gocheck.Equals, 1)
}
