// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"io"

	"github.com/tsuru/config"
	"github.com/tsuru/go-gandalfclient"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrAppAlreadyExists = errors.New("there is already an app with this name")
	ErrAppNotFound      = errors.New("app not found")
)

// reserveUserApp reserves the app for the user, only if the user has a quota
// of apps. If the user does not have a quota, meaning that it's unlimited,
// reserveUserApp.Forward just return nil.
var reserveUserApp = action.Action{
	Name: "reserve-user-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var app *App
		switch ctx.Params[0].(type) {
		case *App:
			app = ctx.Params[0].(*App)
		default:
			return nil, errors.New("First parameter must be *App.")
		}
		var user auth.User
		switch ctx.Params[1].(type) {
		case auth.User:
			user = ctx.Params[1].(auth.User)
		case *auth.User:
			user = *ctx.Params[1].(*auth.User)
		default:
			return nil, errors.New("Third parameter must be auth.User or *auth.User.")
		}
		usr, err := auth.GetUserByEmail(user.Email)
		if err != nil {
			return nil, err
		}
		if err := auth.ReserveApp(usr); err != nil {
			return nil, err
		}
		return map[string]string{"app": app.Name, "user": user.Email}, nil
	},
	Backward: func(ctx action.BWContext) {
		m := ctx.FWResult.(map[string]string)
		if user, err := auth.GetUserByEmail(m["user"]); err == nil {
			auth.ReleaseApp(user)
		}
	},
	MinParams: 2,
}

// insertApp is an action that inserts an app in the database in Forward and
// removes it in the Backward.
//
// The first argument in the context must be an App or a pointer to an App.
var insertApp = action.Action{
	Name: "insert-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var app *App
		switch ctx.Params[0].(type) {
		case *App:
			app = ctx.Params[0].(*App)
		default:
			return nil, errors.New("First parameter must be *App.")
		}
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		app.Quota = quota.Unlimited
		if limit, err := config.GetInt("quota:units-per-app"); err == nil {
			app.Quota.Limit = limit
		}
		err = conn.Apps().Insert(app)
		if mgo.IsDup(err) {
			return nil, ErrAppAlreadyExists
		}
		return app, err
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.FWResult.(*App)
		conn, err := db.Conn()
		if err != nil {
			log.Errorf("Could not connect to the database: %s", err)
			return
		}
		defer conn.Close()
		conn.Apps().Remove(bson.M{"name": app.Name})
	},
	MinParams: 1,
}

// exportEnvironmentsAction exports tsuru's default environment variables in a
// new app. It requires a pointer to an App instance as the first parameter.
var exportEnvironmentsAction = action.Action{
	Name: "export-environments",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(*App)
		app, err := GetByName(app.Name)
		if err != nil {
			return nil, err
		}
		t, err := AuthScheme.AppLogin(app.Name)
		if err != nil {
			return nil, err
		}
		host, _ := config.GetString("host")
		appdir, _ := config.GetString("git:unit-repo")
		envVars := []bind.EnvVar{
			{Name: "TSURU_APPNAME", Value: app.Name},
			{Name: "TSURU_APPDIR", Value: appdir},
			{Name: "TSURU_HOST", Value: host},
			{Name: "TSURU_APP_TOKEN", Value: t.GetValue()},
		}
		err = app.setEnvsToApp(envVars, false, false)
		if err != nil {
			return nil, err
		}
		return ctx.Previous, nil
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.Params[0].(*App)
		AuthScheme.Logout(app.Env["TSURU_APP_TOKEN"].Value)
		app, err := GetByName(app.Name)
		if err == nil {
			vars := []string{"TSURU_HOST", "TSURU_APPNAME", "TSURU_APP_TOKEN"}
			app.UnsetEnvs(vars, false)
		}
	},
	MinParams: 1,
}

// createRepository creates a repository for the app in Gandalf.
var createRepository = action.Action{
	Name: "create-repository",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var app *App
		switch ctx.Params[0].(type) {
		case *App:
			app = ctx.Params[0].(*App)
		default:
			return nil, errors.New("First parameter must be *App.")
		}
		gURL := repository.ServerURL()
		var users []string
		for _, t := range app.GetTeams() {
			users = append(users, t.Users...)
		}
		c := gandalf.Client{Endpoint: gURL}
		_, err := c.NewRepository(app.Name, users, false)
		return app, err
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.FWResult.(*App)
		gURL := repository.ServerURL()
		c := gandalf.Client{Endpoint: gURL}
		c.RemoveRepository(app.Name)
	},
	MinParams: 1,
}

// provisionApp provisions the app in the provisioner. It takes two arguments:
// the app, and the number of units to create (an unsigned integer).
var provisionApp = action.Action{
	Name: "provision-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var app *App
		switch ctx.Params[0].(type) {
		case *App:
			app = ctx.Params[0].(*App)
		default:
			return nil, errors.New("First parameter must be *App.")
		}
		err := Provisioner.Provision(app)
		if err != nil {
			return nil, err
		}
		return app, nil
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.FWResult.(*App)
		Provisioner.Destroy(app)
	},
	MinParams: 1,
}

var setAppIp = action.Action{
	Name: "set-app-ip",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var app *App
		switch ctx.Params[0].(type) {
		case *App:
			app = ctx.Params[0].(*App)
		default:
			return nil, errors.New("First parameter must be *App.")
		}
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		app.Ip, err = Provisioner.Addr(app)
		if err != nil {
			return nil, err
		}
		err = conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$set": bson.M{"ip": app.Ip}})
		if err != nil {
			return nil, err
		}
		return app, nil
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.FWResult.(*App)
		app.Ip = ""
		conn, err := db.Conn()
		if err != nil {
			log.Errorf("Error trying to get connection to rollback setAppIp action: %s", err)
		}
		defer conn.Close()
		err = conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$unset": bson.M{"ip": ""}})
		if err != nil {
			log.Errorf("Error trying to update app to rollback setAppIp action: %s", err)
		}
	},
	MinParams: 1,
}

var reserveUnitsToAdd = action.Action{
	Name: "reserve-units-to-add",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var app *App
		switch ctx.Params[0].(type) {
		case *App:
			app = ctx.Params[0].(*App)
		default:
			return nil, errors.New("First parameter must be *App.")
		}
		var n int
		switch ctx.Params[1].(type) {
		case int:
			n = ctx.Params[1].(int)
		case uint:
			n = int(ctx.Params[1].(uint))
		default:
			return nil, errors.New("Second parameter must be int or uint.")
		}
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		app, err = GetByName(app.Name)
		if err != nil {
			return nil, ErrAppNotFound
		}
		err = reserveUnits(app, n)
		if err != nil {
			return nil, err
		}
		return n, nil
	},
	Backward: func(ctx action.BWContext) {
		var app *App
		switch ctx.Params[0].(type) {
		case *App:
			app = ctx.Params[0].(*App)
		}
		qty := ctx.FWResult.(int)
		err := releaseUnits(app, qty)
		if err != nil {
			log.Errorf("Failed to rollback reserveUnitsToAdd: %s", err)
		}
	},
	MinParams: 2,
}

var provisionAddUnits = action.Action{
	Name: "provision-add-units",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var app *App
		switch ctx.Params[0].(type) {
		case *App:
			app = ctx.Params[0].(*App)
		default:
			return nil, errors.New("First parameter must be *App.")
		}
		var w io.Writer
		if len(ctx.Params) >= 3 {
			w, _ = ctx.Params[2].(io.Writer)
		}
		n := ctx.Previous.(int)
		units, err := Provisioner.AddUnits(app, uint(n), w)
		if err != nil {
			return nil, err
		}
		return units, nil
	},
	Backward: func(ctx action.BWContext) {
		units := ctx.FWResult.([]provision.Unit)
		for _, unit := range units {
			Provisioner.RemoveUnit(unit)
		}
	},
	MinParams: 1,
}

var BindService = action.Action{
	Name: "bind-service",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var app *App
		switch ctx.Params[0].(type) {
		case *App:
			app = ctx.Params[0].(*App)
		case DeployOptions:
			opts := ctx.Params[0].(DeployOptions)
			app = opts.App
		default:
			return nil, errors.New("First parameter must be *App or DeployOptions.")
		}
		units, _ := ctx.Previous.([]provision.Unit)
		app, err := GetByName(app.Name)
		if err != nil {
			return nil, ErrAppNotFound
		}
		if len(units) == 0 {
			units = app.Units()
		}
		for _, unit := range units {
			err := app.BindUnit(&unit)
			if err != nil {
				return nil, err
			}
		}
		return nil, nil
	},
	MinParams: 1,
}

// ProvisionerDeploy is an actions that call the Provisioner.Deploy.
var ProvisionerDeploy = action.Action{
	Name: "provisioner-deploy",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		opts, ok := ctx.Params[0].(DeployOptions)
		if !ok {
			return nil, errors.New("First parameter must be DeployOptions")
		}
		writer, ok := ctx.Params[1].(io.Writer)
		if !ok {
			return nil, errors.New("Second parameter must be an io.Writer")
		}
		if opts.File != nil {
			if deployer, ok := Provisioner.(provision.UploadDeployer); ok {
				return nil, deployer.UploadDeploy(opts.App, opts.File, writer)
			}
		}
		if opts.ArchiveURL != "" {
			if deployer, ok := Provisioner.(provision.ArchiveDeployer); ok {
				return nil, deployer.ArchiveDeploy(opts.App, opts.ArchiveURL, writer)
			}
		}
		err := Provisioner.(provision.GitDeployer).GitDeploy(opts.App, opts.Version, writer)
		return nil, err
	},
	Backward: func(ctx action.BWContext) {
	},
	MinParams: 2,
}

// Increment is an actions that increments the deploy number.
var IncrementDeploy = action.Action{
	Name: "increment-deploy",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		opts, ok := ctx.Params[0].(DeployOptions)
		if !ok {
			return nil, errors.New("First parameter must be DeployOptions")
		}
		err := incrementDeploy(opts.App)
		return nil, err
	},
	Backward: func(ctx action.BWContext) {
	},
	MinParams: 1,
}
