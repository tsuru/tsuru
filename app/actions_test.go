// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"io"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	appTypes "github.com/tsuru/tsuru/types/app"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	eventTypes "github.com/tsuru/tsuru/types/event"
	permTypes "github.com/tsuru/tsuru/types/permission"
	provisionTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/types/quota"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	check "gopkg.in/check.v1"
)

func (s *S) TestReserveUserAppName(c *check.C) {
	c.Assert(reserveUserApp.Name, check.Equals, "reserve-user-app")
}

func (s *S) TestInsertAppName(c *check.C) {
	c.Assert(insertApp.Name, check.Equals, "insert-app")
}

func (s *S) TestExportEnvironmentsName(c *check.C) {
	c.Assert(exportEnvironmentsAction.Name, check.Equals, "export-environments")
}

func (s *S) TestProvisionAppName(c *check.C) {
	c.Assert(provisionApp.Name, check.Equals, "provision-app")
}

func (s *S) TestReserveUnitsToAddName(c *check.C) {
	c.Assert(reserveUnitsToAdd.Name, check.Equals, "reserve-units-to-add")
}

func (s *S) TestProvisionAddUnitsName(c *check.C) {
	c.Assert(provisionAddUnits.Name, check.Equals, "provision-add-units")
}

func (s *S) TestBootstrapDeployAppName(c *check.C) {
	c.Assert(bootstrapDeployApp.Name, check.Equals, "bootstrap-deploy-app")
}

func (s *S) TestInsertAppForward(c *check.C) {
	app := &appTypes.App{Name: "conviction", Platform: "evergrey"}
	ctx := action.FWContext{
		Params: []interface{}{app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(err, check.IsNil)
	a, ok := r.(*appTypes.App)
	c.Assert(ok, check.Equals, true)
	c.Assert(a.Name, check.Equals, app.Name)
	c.Assert(a.Platform, check.Equals, app.Platform)
	gotApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Quota, check.DeepEquals, quota.UnlimitedQuota)
}

func (s *S) TestInsertAppForwardWithQuota(c *check.C) {
	config.Set("quota:units-per-app", 2)
	defer config.Unset("quota:units-per-app")
	app := &appTypes.App{Name: "come", Platform: "beatles"}
	ctx := action.FWContext{
		Params: []interface{}{app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(err, check.IsNil)

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	defer appsCollection.DeleteOne(context.TODO(), mongoBSON.M{"name": app.Name})
	expected := quota.Quota{Limit: 2}
	a, ok := r.(*appTypes.App)
	c.Assert(ok, check.Equals, true)
	c.Assert(app.Quota, check.DeepEquals, expected)
	c.Assert(a.Name, check.Equals, app.Name)
	c.Assert(a.Platform, check.Equals, app.Platform)
	c.Assert(a.Quota, check.DeepEquals, expected)
	gotApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Quota, check.DeepEquals, expected)
}

func (s *S) TestInsertAppForwardAppPointer(c *check.C) {
	app := appTypes.App{Name: "conviction", Platform: "evergrey"}
	ctx := action.FWContext{
		Params: []interface{}{&app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(err, check.IsNil)
	a, ok := r.(*appTypes.App)
	c.Assert(ok, check.Equals, true)
	c.Assert(a.Name, check.Equals, app.Name)
	c.Assert(a.Platform, check.Equals, app.Platform)
	_, err = GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
}

func (s *S) TestInsertAppForwardInvalidValue(c *check.C) {
	app := appTypes.App{Name: "conviction", Platform: "evergrey"}
	ctx := action.FWContext{
		Params: []interface{}{app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(r, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "First parameter must be *App.")
}

func (s *S) TestInsertAppDuplication(c *check.C) {
	app := appTypes.App{Name: "come", Platform: "gotthard", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	ctx := action.FWContext{
		Params: []interface{}{&app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(r, check.IsNil)
	c.Assert(err, check.Equals, ErrAppAlreadyExists)
}

func (s *S) TestInsertAppBackward(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	app := appTypes.App{Name: "conviction", Platform: "evergrey", TeamOwner: s.team.Name}
	ctx := action.BWContext{
		Params:   []interface{}{app},
		FWResult: &app,
	}
	err = CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	insertApp.Backward(ctx)
	n, err := appsCollection.CountDocuments(context.TODO(), mongoBSON.M{"name": app.Name})
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, int64(0))
}

func (s *S) TestInsertAppMinimumParams(c *check.C) {
	c.Assert(insertApp.MinParams, check.Equals, 1)
}

func (s *S) TestExportEnvironmentsForward(c *check.C) {
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	app := appTypes.App{Name: "mist", Platform: "opeth", TeamOwner: s.team.Name}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	ctx := action.FWContext{Params: []interface{}{&app}}
	result, err := exportEnvironmentsAction.Forward(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.FitsTypeOf, &app)
	c.Assert(result.(*appTypes.App).Name, check.Equals, app.Name)
	gotApp, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	appEnv := provision.EnvsForApp(gotApp)
	c.Assert(appEnv["TSURU_APPNAME"].Value, check.Equals, app.Name)
	c.Assert(appEnv["TSURU_APPNAME"].Public, check.Equals, true)
	c.Assert(appEnv["TSURU_APPDir"].Value, check.Not(check.Equals), "/home/application/current")
	c.Assert(appEnv["TSURU_APPDir"].Public, check.Equals, false)
}

func (s *S) TestExportEnvironmentsBackward(c *check.C) {
	envNames := []string{
		"TSURU_APPNAME",
	}
	app := appTypes.App{
		Name:      "moon",
		Platform:  "opeth",
		Env:       make(map[string]bindTypes.EnvVar),
		TeamOwner: s.team.Name,
	}
	for _, name := range envNames {
		envVar := bindTypes.EnvVar{Name: name, Value: name, Public: false}
		app.Env[name] = envVar
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	ctx := action.BWContext{Params: []interface{}{&app}}
	exportEnvironmentsAction.Backward(ctx)
	copy, err := GetByName(context.TODO(), app.Name)
	c.Assert(err, check.IsNil)
	for _, name := range envNames {
		if _, ok := copy.Env[name]; ok {
			c.Errorf("Variable %q should be unexported, but it's still exported.", name)
		}
	}
}

func (s *S) TestExportEnvironmentsMinParams(c *check.C) {
	c.Assert(exportEnvironmentsAction.MinParams, check.Equals, 1)
}

func (s *S) TestProvisionAppForward(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	app := appTypes.App{
		Name:     "earthshine",
		Platform: "django",
		Routers:  []appTypes.AppRouter{{Name: "fake"}},
	}
	_, err = appsCollection.InsertOne(context.TODO(), app)
	c.Assert(err, check.IsNil)
	ctx := action.FWContext{Params: []interface{}{&app, 4}}
	result, err := provisionApp.Forward(ctx)
	c.Assert(err, check.IsNil)
	a, ok := result.(*appTypes.App)
	c.Assert(ok, check.Equals, true)
	c.Assert(a.Name, check.Equals, app.Name)
	c.Assert(s.provisioner.Provisioned(&app), check.Equals, true)
}

func (s *S) TestProvisionAppForwardAppPointer(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	app := appTypes.App{
		Name:     "earthshine",
		Platform: "django",
		Routers:  []appTypes.AppRouter{{Name: "fake"}},
	}
	_, err = appsCollection.InsertOne(context.TODO(), app)
	c.Assert(err, check.IsNil)
	ctx := action.FWContext{Params: []interface{}{&app, 4}}
	result, err := provisionApp.Forward(ctx)
	c.Assert(err, check.IsNil)
	a, ok := result.(*appTypes.App)
	c.Assert(ok, check.Equals, true)
	c.Assert(a.Name, check.Equals, app.Name)
	c.Assert(s.provisioner.Provisioned(&app), check.Equals, true)
}

func (s *S) TestProvisionAppForwardInvalidApp(c *check.C) {
	ctx := action.FWContext{Params: []interface{}{"something", 1}}
	_, err := provisionApp.Forward(ctx)
	c.Assert(err, check.NotNil)
}

func (s *S) TestProvisionAppBackward(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	app := appTypes.App{
		Name:     "earthshine",
		Platform: "django",
		Routers:  []appTypes.AppRouter{{Name: "fake"}},
	}
	_, err = appsCollection.InsertOne(context.TODO(), app)
	c.Assert(err, check.IsNil)
	fwctx := action.FWContext{Params: []interface{}{&app, 4}}
	result, err := provisionApp.Forward(fwctx)
	c.Assert(err, check.IsNil)
	bwctx := action.BWContext{Params: []interface{}{&app, 4}, FWResult: result}
	provisionApp.Backward(bwctx)
	c.Assert(s.provisioner.Provisioned(&app), check.Equals, false)
}

func (s *S) TestProvisionAppMinParams(c *check.C) {
	c.Assert(provisionApp.MinParams, check.Equals, 1)
}

func (s *S) TestBootstrapDeployAppForwardNoConfig(c *check.C) {
	config.Unset("apps:bootstrap:image")
	app := &appTypes.App{
		Name:      "myapp",
		Platform:  "django",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), app, s.user)
	c.Assert(err, check.IsNil)
	ctx := action.FWContext{
		Context: context.TODO(),
		Params:  []interface{}{app},
	}
	result, err := bootstrapDeployApp.Forward(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.Equals, app)
}

func (s *S) TestBootstrapDeployAppForwardWithConfig(c *check.C) {
	config.Set("apps:bootstrap:image", "myregistry/bootstrap:latest")
	defer config.Unset("apps:bootstrap:image")

	var deployOpts DeployOptions
	mockDeployFunc = func(ctx context.Context, opts DeployOptions) (string, error) {
		deployOpts = opts
		return "", nil
	}
	defer func() { mockDeployFunc = nil }()

	app := &appTypes.App{
		Name:      "myapp",
		Platform:  "django",
		TeamOwner: s.team.Name,
		Routers:   []appTypes.AppRouter{{Name: "fake"}},
	}
	err := CreateApp(context.TODO(), app, s.user)
	c.Assert(err, check.IsNil)
	ctx := action.FWContext{
		Context: context.TODO(),
		Params:  []interface{}{app},
	}
	result, err := bootstrapDeployApp.Forward(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.Equals, app)

	c.Assert(deployOpts.App.Name, check.Equals, "myapp")
	c.Assert(deployOpts.Image, check.Equals, "myregistry/bootstrap:latest")
	c.Assert(deployOpts.Message, check.Equals, "bootstrap deploy")
	c.Assert(deployOpts.Kind, check.Equals, provisionTypes.DeployKind("image"))
	c.Assert(deployOpts.OutputStream, check.Equals, io.Discard)
	c.Assert(deployOpts.Event, check.NotNil)

	evt := deployOpts.Event
	c.Assert(evt.Target, check.Equals, eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: app.Name})
	c.Assert(evt.Kind, check.Equals, eventTypes.Kind{Type: "internal", Name: "bootstrap deploy"})
	c.Assert(evt.Lock, check.IsNil)
	c.Assert(evt.Allowed, check.DeepEquals, event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxApp, app.Name)))
}

func (s *S) TestBootstrapDeployAppForwardInvalidParam(c *check.C) {
	ctx := action.FWContext{
		Context: context.TODO(),
		Params:  []interface{}{"invalid"},
	}
	result, err := bootstrapDeployApp.Forward(ctx)
	c.Assert(result, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "First parameter must be *App.")
}

func (s *S) TestBootstrapDeployAppMinParams(c *check.C) {
	c.Assert(bootstrapDeployApp.MinParams, check.Equals, 1)
}

func (s *S) TestReserveUserAppForward(c *check.C) {
	user := auth.User{
		Email: "clap@yes.com",
		Quota: quota.Quota{Limit: 1},
	}
	s.mockService.UserQuota.OnInc = func(item quota.QuotaItem, q int) error {
		c.Assert(item.GetName(), check.Equals, user.Email)
		return nil
	}
	err := user.Create(context.TODO())
	c.Assert(err, check.IsNil)
	app := appTypes.App{
		Name:     "clap",
		Platform: "django",
	}
	expected := map[string]string{"user": user.Email, "app": app.Name}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{&app, &user}})
	c.Assert(err, check.IsNil)
	c.Assert(previous, check.DeepEquals, expected)
}

func (s *S) TestReserveUserAppForwardNonPointer(c *check.C) {
	user := auth.User{
		Email: "clap@yes.com",
		Quota: quota.Quota{Limit: 1},
	}
	s.mockService.UserQuota.OnInc = func(item quota.QuotaItem, q int) error {
		c.Assert(item.GetName(), check.Equals, user.Email)
		return nil
	}
	err := user.Create(context.TODO())
	c.Assert(err, check.IsNil)
	app := appTypes.App{
		Name:     "clap",
		Platform: "django",
	}
	expected := map[string]string{"user": user.Email, "app": app.Name}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{&app, user}})
	c.Assert(err, check.IsNil)
	c.Assert(previous, check.DeepEquals, expected)
}

func (s *S) TestReserveUserAppForwardAppNotPointer(c *check.C) {
	user := auth.User{
		Email: "clap@yes.com",
		Quota: quota.Quota{Limit: 1},
	}
	s.mockService.UserQuota.OnInc = func(item quota.QuotaItem, q int) error {
		c.Assert(item.GetName(), check.Equals, user.Email)
		return nil
	}
	err := user.Create(context.TODO())
	c.Assert(err, check.IsNil)
	app := appTypes.App{
		Name:     "clap",
		Platform: "django",
	}
	expected := map[string]string{"user": user.Email, "app": app.Name}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{&app, user}})
	c.Assert(err, check.IsNil)
	c.Assert(previous, check.DeepEquals, expected)
}

func (s *S) TestReserveUserAppForwardInvalidApp(c *check.C) {
	user := auth.User{Email: "clap@yes.com"}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{"something", user}})
	c.Assert(previous, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "First parameter must be *App.")
}

func (s *S) TestReserveUserAppForwardInvalidUser(c *check.C) {
	app := appTypes.App{
		Name:     "clap",
		Platform: "django",
	}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{&app, "something"}})
	c.Assert(previous, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Second parameter must be auth.User or *auth.User.")
}

func (s *S) TestReserveUserAppForwardQuotaExceeded(c *check.C) {
	user := auth.User{
		Email: "clap@yes.com",
		Quota: quota.Quota{Limit: 1, InUse: 1},
	}
	s.mockService.UserQuota.OnInc = func(item quota.QuotaItem, q int) error {
		c.Assert(item.GetName(), check.Equals, user.Email)
		return &quota.QuotaExceededError{Available: 0, Requested: 1}
	}
	err := user.Create(context.TODO())
	c.Assert(err, check.IsNil)
	app := appTypes.App{
		Name:     "clap",
		Platform: "django",
	}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{&app, user}})
	c.Assert(previous, check.IsNil)
	e, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Available, check.Equals, uint(0))
	c.Assert(e.Requested, check.Equals, uint(1))
}

func (s *S) TestReserveUserAppBackward(c *check.C) {
	user := auth.User{
		Email: "clap@yes.com",
		Quota: quota.Quota{Limit: 1, InUse: 1},
	}
	s.mockService.UserQuota.OnInc = func(item quota.QuotaItem, q int) error {
		c.Assert(item.GetName(), check.Equals, user.Email)
		return nil
	}
	err := user.Create(context.TODO())
	c.Assert(err, check.IsNil)
	app := appTypes.App{
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
}

func (s *S) TestReserveUserAppMinParams(c *check.C) {
	c.Assert(reserveUserApp.MinParams, check.Equals, 2)
}

func (s *S) TestReserveUnitsToAddForward(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	app := appTypes.App{
		Name:     "visions",
		Platform: "django",
		Quota:    quota.UnlimitedQuota,
		Routers:  []appTypes.AppRouter{{Name: "fake"}},
	}
	s.mockService.AppQuota.OnInc = func(a *appTypes.App, quantity int) error {
		c.Assert(a.Name, check.Equals, app.Name)
		c.Assert(quantity, check.Equals, 3)
		return nil
	}
	_, err = appsCollection.InsertOne(context.TODO(), app)
	c.Assert(err, check.IsNil)
	result, err := reserveUnitsToAdd.Forward(action.FWContext{Params: []interface{}{&app, 3}})
	c.Assert(err, check.IsNil)
	c.Assert(result.(int), check.Equals, 3)
}

func (s *S) TestReserveUnitsToAddForwardUint(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	app := appTypes.App{
		Name:     "visions",
		Platform: "django",
		Quota:    quota.UnlimitedQuota,
		Routers:  []appTypes.AppRouter{{Name: "fake"}},
	}
	s.mockService.AppQuota.OnInc = func(a *appTypes.App, quantity int) error {
		c.Assert(a.Name, check.Equals, app.Name)
		c.Assert(quantity, check.Equals, 3)
		return nil
	}
	_, err = appsCollection.InsertOne(context.TODO(), app)
	c.Assert(err, check.IsNil)
	result, err := reserveUnitsToAdd.Forward(action.FWContext{Params: []interface{}{&app, uint(3)}})
	c.Assert(err, check.IsNil)
	c.Assert(result.(int), check.Equals, 3)
}

func (s *S) TestReserveUnitsToAddForwardQuotaExceeded(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	app := appTypes.App{
		Name:     "visions",
		Platform: "django",
		Quota:    quota.Quota{Limit: 1, InUse: 1},
		Routers:  []appTypes.AppRouter{{Name: "fake"}},
	}
	s.mockService.AppQuota.OnInc = func(a *appTypes.App, quantity int) error {
		c.Assert(a.Name, check.Equals, app.Name)
		c.Assert(quantity, check.Equals, 1)
		return &quota.QuotaExceededError{Available: 0, Requested: 1}
	}
	_, err = appsCollection.InsertOne(context.TODO(), app)
	c.Assert(err, check.IsNil)
	result, err := reserveUnitsToAdd.Forward(action.FWContext{Params: []interface{}{&app, 1}})
	c.Assert(result, check.IsNil)
	c.Assert(err, check.NotNil)
	e, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Available, check.Equals, uint(0))
	c.Assert(e.Requested, check.Equals, uint(1))
}

func (s *S) TestReserveUnitsToAddForwardInvalidApp(c *check.C) {
	result, err := reserveUnitsToAdd.Forward(action.FWContext{Params: []interface{}{"something", 3}})
	c.Assert(result, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "First parameter must be *App.")
}

func (s *S) TestReserveUnitsToAddAppNotFound(c *check.C) {
	app := appTypes.App{Name: "something"}
	result, err := reserveUnitsToAdd.Forward(action.FWContext{Params: []interface{}{&app, 3}})
	c.Assert(result, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "App not found")
}

func (s *S) TestReserveUnitsToAddForwardInvalidNumber(c *check.C) {
	result, err := reserveUnitsToAdd.Forward(action.FWContext{Params: []interface{}{&appTypes.App{}, "what"}})
	c.Assert(result, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Second parameter must be int or uint.")
}

func (s *S) TestReserveUnitsToAddBackward(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	app := appTypes.App{
		Name:     "visions",
		Platform: "django",
		Quota:    quota.Quota{Limit: 5, InUse: 4},
		Routers:  []appTypes.AppRouter{{Name: "fake"}},
	}
	s.mockService.AppQuota.OnInc = func(a *appTypes.App, quantity int) error {
		c.Assert(a.Name, check.Equals, app.Name)
		c.Assert(quantity, check.Equals, -3)
		return nil
	}
	_, err = appsCollection.InsertOne(context.TODO(), app)
	c.Assert(err, check.IsNil)
	reserveUnitsToAdd.Backward(action.BWContext{Params: []interface{}{&app, 3}, FWResult: 3})
}

func (s *S) TestReserveUnitsMinParams(c *check.C) {
	c.Assert(reserveUnitsToAdd.MinParams, check.Equals, 2)
}

func (s *S) TestProvisionAddUnits(c *check.C) {
	app := appTypes.App{
		Name:      "visions",
		Platform:  "django",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	version := newSuccessfulAppVersion(c, &app)
	ctx := action.FWContext{Previous: 3, Params: []interface{}{&app, 3, nil, "web", version}}
	_, err = provisionAddUnits.Forward(ctx)
	c.Assert(err, check.IsNil)
	units := s.provisioner.GetUnits(&app)
	c.Assert(units, check.HasLen, 3)
}

func (s *S) TestProvisionAddUnitsProvisionFailure(c *check.C) {
	s.provisioner.PrepareFailure("AddUnits", errors.New("Failed to add units"))
	app := appTypes.App{
		Name:      "visions",
		Platform:  "django",
		TeamOwner: s.team.Name,
	}
	err := CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	version := newSuccessfulAppVersion(c, &app)
	ctx := action.FWContext{Previous: 3, Params: []interface{}{&app, 3, nil, "web", version}}
	result, err := provisionAddUnits.Forward(ctx)
	c.Assert(result, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to add units")
}

func (s *S) TestProvisionAddUnitsInvalidApp(c *check.C) {
	result, err := provisionAddUnits.Forward(action.FWContext{Params: []interface{}{"something"}})
	c.Assert(result, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "First parameter must be *App.")
}

func (s *S) TestProvisionAddUnitsMinParams(c *check.C) {
	c.Assert(provisionAddUnits.MinParams, check.Equals, 1)
}

func (s *S) TestUpdateAppProvisionerBackward(c *check.C) {
	ctx := context.Background()
	p1 := provisiontest.NewFakeProvisioner()
	p1.Name = "fake1"
	provision.Register("fake1", func() (provision.Provisioner, error) {
		return p1, nil
	})
	opts := pool.AddPoolOptions{Name: "test", Provisioner: "fake1", Public: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	app := appTypes.App{Name: "myapp", Platform: "django", Pool: "test", TeamOwner: s.team.Name}
	err = CreateApp(context.TODO(), &app, s.user)
	c.Assert(err, check.IsNil)
	newApp := appTypes.App{Name: "myapp", Platform: "python", Pool: "test", TeamOwner: s.team.Name}
	newSuccessfulAppVersion(c, &app)
	err = AddUnits(ctx, &app, 1, "web", "", nil)
	c.Assert(err, check.IsNil)
	fwctx := action.FWContext{Params: []interface{}{&newApp, &app, io.Discard}}
	_, err = updateAppProvisioner.Forward(fwctx)
	c.Assert(err, check.IsNil)
	units, err := AppUnits(ctx, &app)
	c.Assert(err, check.IsNil)
	provApp, err := p1.GetAppFromUnitID(units[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(provApp.Platform, check.Equals, "python")
	bwctx := action.BWContext{Params: []interface{}{&newApp, &app, io.Discard}}
	updateAppProvisioner.Backward(bwctx)
	units, err = AppUnits(ctx, &app)
	c.Assert(err, check.IsNil)
	provApp, err = p1.GetAppFromUnitID(units[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(provApp.Platform, check.Equals, "django")
}
