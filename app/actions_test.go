// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/testing"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

func (s *S) TestReserveUserAppName(c *gocheck.C) {
	c.Assert(reserveUserApp.Name, gocheck.Equals, "reserve-user-app")
}

func (s *S) TestInsertAppName(c *gocheck.C) {
	c.Assert(insertApp.Name, gocheck.Equals, "insert-app")
}

func (s *S) TestExportEnvironmentsName(c *gocheck.C) {
	c.Assert(exportEnvironmentsAction.Name, gocheck.Equals, "export-environments")
}

func (s *S) TestCreateRepositoryName(c *gocheck.C) {
	c.Assert(createRepository.Name, gocheck.Equals, "create-repository")
}

func (s *S) TestProvisionAppName(c *gocheck.C) {
	c.Assert(provisionApp.Name, gocheck.Equals, "provision-app")
}

func (s *S) TestReserveUnitsToAddName(c *gocheck.C) {
	c.Assert(reserveUnitsToAdd.Name, gocheck.Equals, "reserve-units-to-add")
}

func (s *S) TestProvisionAddUnitsName(c *gocheck.C) {
	c.Assert(provisionAddUnits.Name, gocheck.Equals, "provision-add-units")
}

func (s *S) TestSetAppIpName(c *gocheck.C) {
	c.Assert(setAppIp.Name, gocheck.Equals, "set-app-ip")
}

func (s *S) TestInsertAppForward(c *gocheck.C) {
	app := &App{Name: "conviction", Platform: "evergrey"}
	ctx := action.FWContext{
		Params: []interface{}{app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	a, ok := r.(*App)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(a.Name, gocheck.Equals, app.Name)
	c.Assert(a.Platform, gocheck.Equals, app.Platform)
	gotApp, err := GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(gotApp.Quota, gocheck.DeepEquals, quota.Unlimited)
}

func (s *S) TestInsertAppForwardWithQuota(c *gocheck.C) {
	config.Set("quota:units-per-app", 2)
	defer config.Unset("quota:units-per-app")
	app := &App{Name: "come", Platform: "beatles"}
	ctx := action.FWContext{
		Params: []interface{}{app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	expected := quota.Quota{Limit: 2}
	a, ok := r.(*App)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(app.Quota, gocheck.DeepEquals, expected)
	c.Assert(a.Name, gocheck.Equals, app.Name)
	c.Assert(a.Platform, gocheck.Equals, app.Platform)
	c.Assert(a.Quota, gocheck.DeepEquals, expected)
	gotApp, err := GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(gotApp.Quota, gocheck.DeepEquals, expected)
}

func (s *S) TestInsertAppForwardAppPointer(c *gocheck.C) {
	app := App{Name: "conviction", Platform: "evergrey"}
	ctx := action.FWContext{
		Params: []interface{}{&app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	a, ok := r.(*App)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(a.Name, gocheck.Equals, app.Name)
	c.Assert(a.Platform, gocheck.Equals, app.Platform)
	_, err = GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestInsertAppForwardInvalidValue(c *gocheck.C) {
	app := App{Name: "conviction", Platform: "evergrey"}
	ctx := action.FWContext{
		Params: []interface{}{app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(r, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "First parameter must be *App.")
}

func (s *S) TestInsertAppDuplication(c *gocheck.C) {
	app := App{Name: "come", Platform: "gotthard"}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	ctx := action.FWContext{
		Params: []interface{}{&app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(r, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, ErrAppAlreadyExists)
}

func (s *S) TestInsertAppBackward(c *gocheck.C) {
	app := App{Name: "conviction", Platform: "evergrey"}
	ctx := action.BWContext{
		Params:   []interface{}{app},
		FWResult: &app,
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name}) // sanity
	insertApp.Backward(ctx)
	n, err := s.conn.Apps().Find(bson.M{"name": app.Name}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 0)
}

func (s *S) TestInsertAppMinimumParams(c *gocheck.C) {
	c.Assert(insertApp.MinParams, gocheck.Equals, 1)
}

func (s *S) TestExportEnvironmentsForward(c *gocheck.C) {
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	app := App{Name: "mist", Platform: "opeth"}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	ctx := action.FWContext{Params: []interface{}{&app}}
	result, err := exportEnvironmentsAction.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.Equals, nil)
	gotApp, err := GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	appEnv := gotApp.InstanceEnv("")
	c.Assert(appEnv["TSURU_APPNAME"].Value, gocheck.Equals, app.Name)
	c.Assert(appEnv["TSURU_APPNAME"].Public, gocheck.Equals, false)
	c.Assert(appEnv["TSURU_HOST"].Value, gocheck.Equals, expectedHost)
	c.Assert(appEnv["TSURU_HOST"].Public, gocheck.Equals, false)
	c.Assert(appEnv["TSURU_APP_TOKEN"].Value, gocheck.Not(gocheck.Equals), "")
	c.Assert(appEnv["TSURU_APP_TOKEN"].Public, gocheck.Equals, false)
	c.Assert(appEnv["TSURU_APPDir"].Value, gocheck.Not(gocheck.Equals), "/home/application/current")
	c.Assert(appEnv["TSURU_APPDir"].Public, gocheck.Equals, false)
	t, err := nativeScheme.Auth(appEnv["TSURU_APP_TOKEN"].Value)
	c.Assert(err, gocheck.IsNil)
	c.Assert(t.IsAppToken(), gocheck.Equals, true)
	c.Assert(t.GetAppName(), gocheck.Equals, app.Name)
}

func (s *S) TestExportEnvironmentsBackward(c *gocheck.C) {
	envNames := []string{
		"TSURU_APP_TOKEN",
	}
	app := App{Name: "moon", Platform: "opeth", Env: make(map[string]bind.EnvVar)}
	for _, name := range envNames {
		envVar := bind.EnvVar{Name: name, Value: name, Public: false}
		app.Env[name] = envVar
	}
	token, err := nativeScheme.AppLogin(app.Name)
	c.Assert(err, gocheck.IsNil)
	app.Env["TSURU_APP_TOKEN"] = bind.EnvVar{Name: "TSURU_APP_NAME", Value: token.GetValue()}
	err = s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	ctx := action.BWContext{Params: []interface{}{&app}}
	exportEnvironmentsAction.Backward(ctx)
	copy, err := GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	for _, name := range envNames {
		if _, ok := copy.Env[name]; ok {
			c.Errorf("Variable %q should be unexported, but it's still exported.", name)
		}
	}
	_, err = nativeScheme.Auth(token.GetValue())
	c.Assert(err, gocheck.Equals, auth.ErrInvalidToken)
}

func (s *S) TestExportEnvironmentsMinParams(c *gocheck.C) {
	c.Assert(exportEnvironmentsAction.MinParams, gocheck.Equals, 1)
}

func (s *S) TestCreateRepositoryForward(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{Name: "someapp", Teams: []string{s.team.Name}}
	ctx := action.FWContext{Params: []interface{}{&app}}
	result, err := createRepository.Forward(ctx)
	a, ok := result.(*App)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(a.Name, gocheck.Equals, app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(h.url[0], gocheck.Equals, "/repository")
	c.Assert(h.method[0], gocheck.Equals, "POST")
	expected := fmt.Sprintf(`{"name":"someapp","users":["%s"],"ispublic":false}`, s.user.Email)
	c.Assert(string(h.body[0]), gocheck.Equals, expected)
}

func (s *S) TestCreateRepositoryForwardAppPointer(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{Name: "someapp", Teams: []string{s.team.Name}}
	ctx := action.FWContext{Params: []interface{}{&app}}
	result, err := createRepository.Forward(ctx)
	a, ok := result.(*App)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(a.Name, gocheck.Equals, app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(h.url[0], gocheck.Equals, "/repository")
	c.Assert(h.method[0], gocheck.Equals, "POST")
	expected := fmt.Sprintf(`{"name":"someapp","users":["%s"],"ispublic":false}`, s.user.Email)
	c.Assert(string(h.body[0]), gocheck.Equals, expected)
}

func (s *S) TestCreateRepositoryForwardInvalidType(c *gocheck.C) {
	ctx := action.FWContext{Params: []interface{}{"something"}}
	_, err := createRepository.Forward(ctx)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "First parameter must be *App.")
}

func (s *S) TestCreateRepositoryBackward(c *gocheck.C) {
	h := testHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{Name: "someapp"}
	ctx := action.BWContext{FWResult: &app, Params: []interface{}{app}}
	createRepository.Backward(ctx)
	c.Assert(h.url[0], gocheck.Equals, "/repository/someapp")
	c.Assert(h.method[0], gocheck.Equals, "DELETE")
	c.Assert(string(h.body[0]), gocheck.Equals, "null")
}

func (s *S) TestCreateRepositoryMinParams(c *gocheck.C) {
	c.Assert(createRepository.MinParams, gocheck.Equals, 1)
}

func (s *S) TestProvisionAppForward(c *gocheck.C) {
	app := App{
		Name:     "earthshine",
		Platform: "django",
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	ctx := action.FWContext{Params: []interface{}{&app, 4}}
	result, err := provisionApp.Forward(ctx)
	defer s.provisioner.Destroy(&app)
	c.Assert(err, gocheck.IsNil)
	a, ok := result.(*App)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(a.Name, gocheck.Equals, app.Name)
	c.Assert(s.provisioner.Provisioned(&app), gocheck.Equals, true)
}

func (s *S) TestProvisionAppForwardAppPointer(c *gocheck.C) {
	app := App{
		Name:     "earthshine",
		Platform: "django",
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	ctx := action.FWContext{Params: []interface{}{&app, 4}}
	result, err := provisionApp.Forward(ctx)
	defer s.provisioner.Destroy(&app)
	c.Assert(err, gocheck.IsNil)
	a, ok := result.(*App)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(a.Name, gocheck.Equals, app.Name)
	c.Assert(s.provisioner.Provisioned(&app), gocheck.Equals, true)
}

func (s *S) TestProvisionAppForwardInvalidApp(c *gocheck.C) {
	ctx := action.FWContext{Params: []interface{}{"something", 1}}
	_, err := provisionApp.Forward(ctx)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestProvisionAppBackward(c *gocheck.C) {
	app := App{
		Name:     "earthshine",
		Platform: "django",
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	fwctx := action.FWContext{Params: []interface{}{&app, 4}}
	result, err := provisionApp.Forward(fwctx)
	c.Assert(err, gocheck.IsNil)
	bwctx := action.BWContext{Params: []interface{}{&app, 4}, FWResult: result}
	provisionApp.Backward(bwctx)
	c.Assert(s.provisioner.Provisioned(&app), gocheck.Equals, false)
}

func (s *S) TestProvisionAppMinParams(c *gocheck.C) {
	c.Assert(provisionApp.MinParams, gocheck.Equals, 1)
}

func (s *S) TestReserveUserAppForward(c *gocheck.C) {
	user := auth.User{
		Email: "clap@yes.com",
		Quota: quota.Quota{Limit: 1},
	}
	err := user.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": user.Email})
	app := App{
		Name:     "clap",
		Platform: "django",
	}
	expected := map[string]string{"user": user.Email, "app": app.Name}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{&app, &user}})
	c.Assert(err, gocheck.IsNil)
	c.Assert(previous, gocheck.DeepEquals, expected)
	err = auth.ReserveApp(&user)
	_, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
	err = auth.ReleaseApp(&user)
	c.Assert(err, gocheck.IsNil)
	err = auth.ReserveApp(&user)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestReserveUserAppForwardNonPointer(c *gocheck.C) {
	user := auth.User{
		Email: "clap@yes.com",
		Quota: quota.Quota{Limit: 1},
	}
	err := user.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": user.Email})
	app := App{
		Name:     "clap",
		Platform: "django",
	}
	expected := map[string]string{"user": user.Email, "app": app.Name}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{&app, user}})
	c.Assert(err, gocheck.IsNil)
	c.Assert(previous, gocheck.DeepEquals, expected)
	err = auth.ReserveApp(&user)
	_, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
	err = auth.ReleaseApp(&user)
	c.Assert(err, gocheck.IsNil)
	err = auth.ReserveApp(&user)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestReserveUserAppForwardAppNotPointer(c *gocheck.C) {
	user := auth.User{
		Email: "clap@yes.com",
		Quota: quota.Quota{Limit: 1},
	}
	err := user.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": user.Email})
	app := App{
		Name:     "clap",
		Platform: "django",
	}
	expected := map[string]string{"user": user.Email, "app": app.Name}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{&app, user}})
	c.Assert(err, gocheck.IsNil)
	c.Assert(previous, gocheck.DeepEquals, expected)
	err = auth.ReserveApp(&user)
	_, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
	err = auth.ReleaseApp(&user)
	c.Assert(err, gocheck.IsNil)
	err = auth.ReserveApp(&user)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestReserveUserAppForwardInvalidApp(c *gocheck.C) {
	user := auth.User{Email: "clap@yes.com"}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{"something", user}})
	c.Assert(previous, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "First parameter must be *App.")
}

func (s *S) TestReserveUserAppForwardInvalidUser(c *gocheck.C) {
	app := App{
		Name:     "clap",
		Platform: "django",
	}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{&app, "something"}})
	c.Assert(previous, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Third parameter must be auth.User or *auth.User.")
}

func (s *S) TestReserveUserAppForwardQuotaExceeded(c *gocheck.C) {
	user := auth.User{
		Email: "clap@yes.com",
		Quota: quota.Quota{Limit: 1, InUse: 1},
	}
	err := user.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": user.Email})
	app := App{
		Name:     "clap",
		Platform: "django",
	}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{&app, user}})
	c.Assert(previous, gocheck.IsNil)
	_, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestReserveUserAppBackward(c *gocheck.C) {
	user := auth.User{
		Email: "clap@yes.com",
		Quota: quota.Quota{Limit: 1, InUse: 1},
	}
	err := user.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": user.Email})
	app := App{
		Name:     "clap",
		Platform: "django",
	}
	ctx := action.BWContext{
		FWResult: map[string]string{
			"app":  app.Name,
			"user": user.Email,
		},
	}
	reserveUserApp.Backward(ctx)
	err = auth.ReserveApp(&user)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestReserveUserAppMinParams(c *gocheck.C) {
	c.Assert(reserveUserApp.MinParams, gocheck.Equals, 2)
}

func (s *S) TestReserveUnitsToAddForward(c *gocheck.C) {
	app := App{
		Name:     "visions",
		Platform: "django",
		Quota:    quota.Unlimited,
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	result, err := reserveUnitsToAdd.Forward(action.FWContext{Params: []interface{}{&app, 3}})
	c.Assert(err, gocheck.IsNil)
	c.Assert(result.(int), gocheck.Equals, 3)
	gotApp, err := GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(gotApp.InUse, gocheck.Equals, 3)
}

func (s *S) TestReserveUnitsToAddForwardUint(c *gocheck.C) {
	app := App{
		Name:     "visions",
		Platform: "django",
		Quota:    quota.Unlimited,
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	result, err := reserveUnitsToAdd.Forward(action.FWContext{Params: []interface{}{&app, uint(3)}})
	c.Assert(err, gocheck.IsNil)
	c.Assert(result.(int), gocheck.Equals, 3)
	gotApp, err := GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(gotApp.InUse, gocheck.Equals, 3)
}

func (s *S) TestReserveUnitsToAddForwardQuotaExceeded(c *gocheck.C) {
	app := App{
		Name:     "visions",
		Platform: "django",
		Quota:    quota.Quota{Limit: 1, InUse: 1},
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	result, err := reserveUnitsToAdd.Forward(action.FWContext{Params: []interface{}{&app, 1}})
	c.Assert(result, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Available, gocheck.Equals, uint(0))
	c.Assert(e.Requested, gocheck.Equals, uint(1))
}

func (s *S) TestReserveUnitsToAddForwardInvalidApp(c *gocheck.C) {
	result, err := reserveUnitsToAdd.Forward(action.FWContext{Params: []interface{}{"something", 3}})
	c.Assert(result, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "First parameter must be *App.")
}

func (s *S) TestReserveUnitsToAddAppNotFound(c *gocheck.C) {
	app := App{Name: "something"}
	result, err := reserveUnitsToAdd.Forward(action.FWContext{Params: []interface{}{&app, 3}})
	c.Assert(result, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "App not found.")
}

func (s *S) TestReserveUnitsToAddForwardInvalidNumber(c *gocheck.C) {
	result, err := reserveUnitsToAdd.Forward(action.FWContext{Params: []interface{}{&App{}, "what"}})
	c.Assert(result, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Second parameter must be int or uint.")
}

func (s *S) TestReserveUnitsToAddBackward(c *gocheck.C) {
	app := App{
		Name:     "visions",
		Platform: "django",
		Quota:    quota.Quota{Limit: 5, InUse: 4},
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	reserveUnitsToAdd.Backward(action.BWContext{Params: []interface{}{&app, 3}, FWResult: 3})
	gotApp, err := GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(gotApp.InUse, gocheck.Equals, 1)
}

func (s *S) TestReserveUnitsMinParams(c *gocheck.C) {
	c.Assert(reserveUnitsToAdd.MinParams, gocheck.Equals, 2)
}

func (s *S) TestProvisionAddUnits(c *gocheck.C) {
	app := App{
		Name:     "visions",
		Platform: "django",
	}
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	ctx := action.FWContext{Previous: 3, Params: []interface{}{&app}}
	fwresult, err := provisionAddUnits.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	units, ok := fwresult.([]provision.Unit)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(units, gocheck.HasLen, 3)
	c.Assert(units, gocheck.DeepEquals, s.provisioner.GetUnits(&app))
}

func (s *S) TestProvisionAddUnitsProvisionFailure(c *gocheck.C) {
	s.provisioner.PrepareFailure("AddUnits", errors.New("Failed to add units"))
	app := App{
		Name:     "visions",
		Platform: "django",
	}
	ctx := action.FWContext{Previous: 3, Params: []interface{}{&app}}
	result, err := provisionAddUnits.Forward(ctx)
	c.Assert(result, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Failed to add units")
}

func (s *S) TestProvisionAddUnitsInvalidApp(c *gocheck.C) {
	result, err := provisionAddUnits.Forward(action.FWContext{Params: []interface{}{"something"}})
	c.Assert(result, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "First parameter must be *App.")
}

func (s *S) TestProvisionAddUnitsBackward(c *gocheck.C) {
	app := App{
		Name:     "fiction",
		Platform: "django",
	}
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	units, err := s.provisioner.AddUnits(&app, 3, nil)
	c.Assert(err, gocheck.IsNil)
	ctx := action.BWContext{Params: []interface{}{&app}, FWResult: units}
	provisionAddUnits.Backward(ctx)
	c.Assert(s.provisioner.GetUnits(&app), gocheck.HasLen, 0)
}

func (s *S) TestProvisionAddUnitsMinParams(c *gocheck.C) {
	c.Assert(provisionAddUnits.MinParams, gocheck.Equals, 1)
}

func (s *S) TestProvisionerDeployName(c *gocheck.C) {
	c.Assert(ProvisionerDeploy.Name, gocheck.Equals, "provisioner-deploy")
}

func (s *S) TestProvisionerDeployMinParams(c *gocheck.C) {
	c.Assert(ProvisionerDeploy.MinParams, gocheck.Equals, 2)
}

func (s *S) TestProvisionerDeployGitForward(c *gocheck.C) {
	a := App{
		Name:     "someApp",
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	opts := DeployOptions{App: &a, Version: "version"}
	ctx := action.FWContext{Params: []interface{}{opts, writer}}
	_, err = ProvisionerDeploy.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	logs := writer.String()
	c.Assert(logs, gocheck.Equals, "Git deploy called")
}

func (s *S) TestProvisionerDeployArchiveForward(c *gocheck.C) {
	a := App{
		Name:     "someApp",
		Platform: "django",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	s.provisioner.Provision(&a)
	defer s.provisioner.Destroy(&a)
	writer := &bytes.Buffer{}
	opts := DeployOptions{App: &a, ArchiveURL: "https://s3.amazonaws.com/smt/archive.tar.gz"}
	ctx := action.FWContext{Params: []interface{}{opts, writer}}
	_, err = ProvisionerDeploy.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	logs := writer.String()
	c.Assert(logs, gocheck.Equals, "Archive deploy called")
}

func (s *S) TestProvisionerDeployParams(c *gocheck.C) {
	ctx := action.FWContext{Params: []interface{}{""}}
	_, err := ProvisionerDeploy.Forward(ctx)
	c.Assert(err.Error(), gocheck.Equals, "First parameter must be DeployOptions")
	ctx = action.FWContext{Params: []interface{}{DeployOptions{}, ""}}
	_, err = ProvisionerDeploy.Forward(ctx)
	c.Assert(err.Error(), gocheck.Equals, "Second parameter must be an io.Writer")
}

func (s *S) TestIncrementDeployName(c *gocheck.C) {
	c.Assert(IncrementDeploy.Name, gocheck.Equals, "increment-deploy")
}

func (s *S) TestIncrementDeployMinParams(c *gocheck.C) {
	c.Assert(IncrementDeploy.MinParams, gocheck.Equals, 1)
}

func (s *S) TestIncrementDeployForward(c *gocheck.C) {
	a := App{
		Name:     "otherapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	writer := &bytes.Buffer{}
	opts := DeployOptions{App: &a, Version: "version"}
	ctx := action.FWContext{Params: []interface{}{opts, writer}}
	_, err = IncrementDeploy.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(a.Deploys, gocheck.Equals, uint(1))
}

func (s *S) TestIncrementDeployParams(c *gocheck.C) {
	ctx := action.FWContext{Params: []interface{}{""}}
	_, err := IncrementDeploy.Forward(ctx)
	c.Assert(err.Error(), gocheck.Equals, "First parameter must be DeployOptions")
}

func (s *S) TestSetAppIpForward(c *gocheck.C) {
	app := &App{Name: "conviction", Platform: "evergrey"}
	err := s.provisioner.Provision(app)
	c.Assert(err, gocheck.IsNil)
	err = s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	ctx := action.FWContext{
		Params: []interface{}{app},
	}
	r, err := setAppIp.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Ip, gocheck.Equals, "conviction.fake-lb.tsuru.io")
	a, ok := r.(*App)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(a.Ip, gocheck.Equals, "conviction.fake-lb.tsuru.io")
	gotApp, err := GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(gotApp.Ip, gocheck.Equals, "conviction.fake-lb.tsuru.io")
}

func (s *S) TestSetAppIpBackward(c *gocheck.C) {
	app := &App{Name: "conviction", Platform: "evergrey", Ip: "some-value"}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	ctx := action.BWContext{
		Params:   []interface{}{app},
		FWResult: app,
	}
	setAppIp.Backward(ctx)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Ip, gocheck.Equals, "")
	gotApp, err := GetByName(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(gotApp.Ip, gocheck.Equals, "")
}
