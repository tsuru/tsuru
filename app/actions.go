// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/types/quota"
)

var (
	ErrAppAlreadyExists = errors.New("there is already an app with this name")
)

var reserveTeamApp = action.Action{
	Name: "reserve-team-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var app *App
		switch ctx.Params[0].(type) {
		case *App:
			app = ctx.Params[0].(*App)
		default:
			return nil, errors.New("first parameter must be *App.")
		}
		if err := servicemanager.TeamQuota.Inc(ctx.Context, &authTypes.Team{Name: app.TeamOwner}, 1); err != nil {
			return nil, err
		}
		return map[string]string{"app": app.Name, "team": app.TeamOwner}, nil
	},
	Backward:  func(ctx action.BWContext) {},
	MinParams: 2,
}

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
			return nil, errors.New("Second parameter must be auth.User or *auth.User.")
		}
		usr, err := auth.GetUserByEmail(user.Email)
		if err != nil {
			return nil, err
		}
		if err := servicemanager.UserQuota.Inc(ctx.Context, usr, 1); err != nil {
			return nil, err
		}
		return map[string]string{"app": app.Name, "user": user.Email}, nil
	},
	Backward: func(ctx action.BWContext) {
		m := ctx.FWResult.(map[string]string)
		if user, err := auth.GetUserByEmail(m["user"]); err == nil {
			servicemanager.UserQuota.Inc(ctx.Context, user, -1)
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
		err := createApp(app)
		if err != nil {
			return nil, err
		}
		return app, nil
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.FWResult.(*App)
		removeApp(app)
	},
	MinParams: 1,
}

func createApp(app *App) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	if app.Quota == (quota.Quota{}) {
		app.Quota = quota.UnlimitedQuota
	}
	var limit int
	if limit, err = config.GetInt("quota:units-per-app"); err == nil {
		app.Quota.Limit = limit
	}
	err = conn.Apps().Insert(app)
	if mgo.IsDup(err) {
		return ErrAppAlreadyExists
	}

	if plog, ok := servicemanager.AppLog.(appTypes.AppLogServiceProvision); ok {
		plog.Provision(app.Name)
	}
	return nil
}

func removeApp(app *App) error {
	if plog, ok := servicemanager.AppLog.(appTypes.AppLogServiceProvision); ok {
		err := plog.CleanUp(app.Name)
		if err != nil {
			log.Errorf("Unable to cleanup logs: %v", err)
		}
	}

	conn, err := db.Conn()
	if err != nil {
		log.Errorf("Could not connect to the database: %s", err)
		return err
	}
	defer conn.Close()
	conn.Apps().Remove(bson.M{"name": app.Name})
	return nil
}

// createAppToken generates a token for the app and saves it in the database.
// It requires a pointer to an App instance as the first parameter.
var createAppToken = action.Action{
	Name: "create-app-token",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app, ok := ctx.Params[0].(*App)
		if !ok {
			return nil, errors.New("First parameter must be *App.")
		}
		app, err := GetByName(ctx.Context, app.Name)
		if err != nil {
			return nil, err
		}
		t, err := AuthScheme.AppLogin(ctx.Context, app.Name)
		if err != nil {
			return nil, err
		}
		return &t, nil
	},
	Backward: func(ctx action.BWContext) {
		var tokenValue string
		if token, ok := ctx.FWResult.(*auth.Token); ok {
			tokenValue = (*token).GetValue()
		} else if app, ok := ctx.Params[0].(*App); ok {
			if tokenVar, ok := app.Env["TSURU_APP_TOKEN"]; ok {
				tokenValue = tokenVar.Value
			}
		}
		if tokenValue != "" {
			AuthScheme.Logout(ctx.Context, tokenValue)
		}
	},
	MinParams: 1,
}

// exportEnvironmentsAction exports tsuru's default environment variables in a
// new app. It requires a pointer to an App instance as the first parameter.
var exportEnvironmentsAction = action.Action{
	Name: "export-environments",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(*App)
		app, err := GetByName(ctx.Context, app.Name)
		if err != nil {
			return nil, err
		}
		t := ctx.Previous.(*auth.Token)
		envVars := []bind.EnvVar{
			{Name: "TSURU_APPNAME", Value: app.Name},
			{Name: "TSURU_APPDIR", Value: defaultAppDir},
			{Name: "TSURU_APP_TOKEN", Value: (*t).GetValue()},
		}
		err = app.SetEnvs(bind.SetEnvArgs{
			Envs:          envVars,
			ShouldRestart: false,
		})
		if err != nil {
			return nil, err
		}
		return app, nil
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.Params[0].(*App)
		app, err := GetByName(ctx.Context, app.Name)
		if err == nil {
			vars := []string{"TSURU_APPNAME", "TSURU_APPDIR", "TSURU_APP_TOKEN"}
			app.UnsetEnvs(bind.UnsetEnvArgs{
				VariableNames: vars,
				ShouldRestart: true,
			})
		}
	},
	MinParams: 1,
}

func removeAllRoutersBackend(ctx context.Context, app *App) error {
	multi := tsuruErrors.NewMultiError()
	for _, appRouter := range app.GetRouters() {
		r, err := router.Get(ctx, appRouter.Name)
		if err != nil {
			multi.Add(err)
			continue
		}
		err = r.RemoveBackend(ctx, app)
		if err != nil && err != router.ErrBackendNotFound {
			multi.Add(err)
		}
	}
	return multi.ToError()
}

var addRouterBackend = action.Action{
	Name: "add-router-backend",
	Forward: func(ctx action.FWContext) (result action.Result, err error) {
		stdCtx := ctx.Context
		var app *App
		switch ctx.Params[0].(type) {
		case *App:
			app = ctx.Params[0].(*App)
		default:
			return nil, errors.New("First parameter must be *App.")
		}
		defer func() {
			if err != nil {
				removeAllRoutersBackend(stdCtx, app)
			}
		}()
		for _, appRouter := range app.GetRouters() {
			r, err := router.Get(stdCtx, appRouter.Name)
			if err != nil {
				return nil, err
			}
			if _, ok := r.(router.RouterV2); ok {
				err = router.Store(app.GetName(), app.GetName(), r.GetType())
			} else if optsRouter, ok := r.(router.OptsRouter); ok {
				err = optsRouter.AddBackendOpts(ctx.Context, app, appRouter.Opts)
			} else {
				err = r.AddBackend(ctx.Context, app)
			}
			if err != nil {
				return nil, err
			}
		}
		return app, nil
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.FWResult.(*App)
		err := removeAllRoutersBackend(ctx.Context, app)
		if err != nil {
			log.Errorf("[add-router-backend rollback] unable to remove all routers backends: %s", err)
		}
	},
	MinParams: 1,
}

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
		prov, err := app.getProvisioner()
		if err != nil {
			return nil, err
		}
		err = prov.Provision(ctx.Context, app)
		if err != nil {
			return nil, err
		}
		return app, nil
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.FWResult.(*App)
		prov, err := app.getProvisioner()
		if err == nil {
			prov.Destroy(ctx.Context, app)
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
		app, err = GetByName(ctx.Context, app.Name)
		if err != nil {
			return nil, appTypes.ErrAppNotFound
		}
		err = servicemanager.AppQuota.Inc(ctx.Context, app, n)
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
		err := servicemanager.AppQuota.Inc(ctx.Context, app, -qty)
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
		w, _ := ctx.Params[2].(io.Writer)
		n := ctx.Previous.(int)
		process := ctx.Params[3].(string)
		version := ctx.Params[4].(appTypes.AppVersion)
		prov, err := app.getProvisioner()
		if err != nil {
			return nil, err
		}
		return nil, prov.AddUnits(ctx.Context, app, uint(n), process, version, w)
	},
	MinParams: 1,
}

var saveApp = action.Action{
	Name: "update-app-save-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app, ok := ctx.Params[0].(*App)
		if !ok {
			return nil, errors.New("expected app ptr as first arg")
		}
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		return nil, conn.Apps().Update(bson.M{"name": app.Name}, app)
	},
	Backward: func(ctx action.BWContext) {
		oldApp := ctx.Params[1].(*App)
		conn, err := db.Conn()
		if err != nil {
			log.Errorf("BACKWARD save app - failed to get database connection: %s", err)
			return
		}
		defer conn.Close()
		err = conn.Apps().Update(bson.M{"name": oldApp.Name}, oldApp)
		if err != nil {
			log.Errorf("BACKWARD save app - failed to update app: %s", err)
		}
	},
}

var restartApp = action.Action{
	Name: "update-app-restart-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app, ok := ctx.Params[0].(*App)
		if !ok {
			return nil, errors.New("expected app ptr as first arg")
		}
		w, _ := ctx.Params[2].(io.Writer)
		return nil, app.Restart(ctx.Context, "", "", w)
	},
	Backward: func(ctx action.BWContext) {
		oldApp := ctx.Params[1].(*App)
		w, _ := ctx.Params[2].(io.Writer)
		err := oldApp.Restart(ctx.Context, "", "", w)
		if err != nil {
			log.Errorf("BACKWARD update app - failed to restart app: %s", err)
		}
	},
}

var provisionAppNewProvisioner = action.Action{
	Name: "provision-app-new-provisioner",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app, ok := ctx.Params[0].(*App)
		if !ok {
			return nil, errors.New("expected app ptr as first arg")
		}
		prov, err := app.getProvisioner()
		if err != nil {
			return nil, err
		}
		return nil, prov.Provision(ctx.Context, app)
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.Params[0].(*App)
		prov, err := app.getProvisioner()
		if err != nil {
			log.Errorf("BACKWARD provision app - failed to get provisioner: %s", err)
		}
		err = prov.Destroy(ctx.Context, app)
		if err != nil {
			log.Errorf("BACKWARD provision app - failed to destroy app in prov: %s", err)
		}
	},
}

var provisionAppAddUnits = action.Action{
	Name: "provision-app-add-unit",
	Forward: func(ctx action.FWContext) (result action.Result, err error) {
		app, ok := ctx.Params[0].(*App)
		if !ok {
			return nil, errors.New("expected app ptr as first arg")
		}
		oldApp, ok := ctx.Params[1].(*App)
		if !ok {
			return nil, errors.New("expected app ptr as first arg")
		}
		w, _ := ctx.Params[2].(io.Writer)
		units, err := oldApp.Units()
		if err != nil {
			return nil, err
		}
		type unitKey struct {
			process string
			version int
		}
		unitCount := make(map[unitKey]uint)
		for _, u := range units {
			unitCount[unitKey{
				process: u.ProcessName,
				version: u.Version,
			}]++
		}
		routers := app.Routers
		router := app.Router
		app.Routers = nil
		app.Router = ""
		defer func() {
			app.Routers = routers
			app.Router = router
			if err == nil {
				_, err = rebuild.RebuildRoutes(ctx.Context, rebuild.RebuildRoutesOpts{
					App:  app,
					Wait: true,
				})
			}
		}()
		for processData, count := range unitCount {
			var version string
			if processData.version > 0 {
				version = strconv.Itoa(processData.version)
			}
			err = app.AddUnits(count, processData.process, version, w)
			if err != nil {
				return nil, err
			}
		}
		return nil, err
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.Params[0].(*App)
		rebuild.RoutesRebuildOrEnqueue(app.Name)
	},
}

var destroyAppOldProvisioner = action.Action{
	Name: "destroy-app-old-provisioner",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		oldApp, ok := ctx.Params[1].(*App)
		if !ok {
			return nil, errors.New("expected app ptr as second arg")
		}
		oldProv, err := oldApp.getProvisioner()
		if err != nil {
			return nil, err
		}
		return nil, oldProv.Destroy(ctx.Context, oldApp)
	},
}

var updateAppProvisioner = action.Action{
	Name: "update-app-provisioner",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app, ok := ctx.Params[0].(*App)
		if !ok {
			return nil, errors.New("expected app ptr as first arg")
		}
		oldApp, ok := ctx.Params[1].(*App)
		if !ok {
			return nil, errors.New("expected app ptr as second arg")
		}
		oldProv, err := oldApp.getProvisioner()
		if err != nil {
			return nil, err
		}
		w, _ := ctx.Params[2].(io.Writer)
		if upProv, ok := oldProv.(provision.UpdatableProvisioner); ok {
			return nil, upProv.UpdateApp(ctx.Context, oldApp, app, w)
		}
		return nil, nil
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.Params[0].(*App)
		oldApp := ctx.Params[1].(*App)
		newProv, err := app.getProvisioner()
		if err != nil {
			log.Errorf("BACKWARDS update-app-provisioner - failed to get app provisioner: %v", err)
			return
		}
		w := ctx.Params[2].(io.Writer)
		if upProv, ok := newProv.(provision.UpdatableProvisioner); ok {
			if err := upProv.UpdateApp(ctx.Context, app, oldApp, w); err != nil {
				log.Errorf("BACKWARDS update-app-provisioner - failed to update app back to previous state: %v", err)
			}
		}
	},
}

var validateNewCNames = action.Action{
	Name: "validate-new-cnames",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		cnameRegexp := regexp.MustCompile(`^(\*\.)?[a-zA-Z0-9][\w-.]+$`)
		cnames := ctx.Params[1].([]string)
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		for _, cname := range cnames {
			if !cnameRegexp.MatchString(cname) {
				return nil, errors.New("Invalid cname")
			}
			cs, err := conn.Apps().Find(bson.M{"cname": cname}).Count()
			if err != nil {
				return nil, err
			}
			if cs > 0 {
				return nil, errors.New("cname already exists!")
			}
		}
		return cnames, nil
	},
}

func setUnsetCnames(ctx context.Context, app *App, cnames []string, toSet bool) error {
	multi := tsuruErrors.NewMultiError()
	for _, appRouter := range app.GetRouters() {
		r, err := router.Get(ctx, appRouter.Name)
		if err != nil {
			multi.Add(err)
			continue
		}
		cnameRouter, ok := r.(router.CNameRouter)
		if !ok {
			continue
		}
		for _, c := range cnames {
			if toSet {
				err = cnameRouter.SetCName(ctx, c, app)
				if err == router.ErrCNameExists {
					err = nil
				}
			} else {
				err = cnameRouter.UnsetCName(ctx, c, app)
				if err == router.ErrCNameNotFound {
					err = nil
				}
			}
			if err != nil {
				multi.Add(err)
			}
		}
	}
	return multi.ToError()
}

var setNewCNamesToProvisioner = action.Action{
	Name: "set-new-cnames-to-provisioner",
	Forward: func(ctx action.FWContext) (result action.Result, err error) {
		stdCtx := ctx.Context
		app := ctx.Params[0].(*App)
		cnames := ctx.Params[1].([]string)
		defer func() {
			if err != nil {
				setUnsetCnames(stdCtx, app, cnames, false)
			}
		}()
		for _, appRouter := range app.GetRouters() {
			var r router.Router
			r, err = router.Get(stdCtx, appRouter.Name)
			if err != nil {
				return nil, err
			}
			cnameRouter, ok := r.(router.CNameRouter)
			if !ok {
				continue
			}
			for _, cname := range cnames {
				err = cnameRouter.SetCName(ctx.Context, cname, app)
				if err != nil {
					return nil, err
				}
			}
		}
		return cnames, nil
	},
	Backward: func(ctx action.BWContext) {
		cnames := ctx.Params[1].([]string)
		app := ctx.Params[0].(*App)
		err := setUnsetCnames(ctx.Context, app, cnames, false)
		if err != nil {
			log.Errorf("BACKWARD set cnames - unable to remove cnames from routers: %s", err)
		}
	},
}

var saveCNames = action.Action{
	Name: "add-cname-save-in-database",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(*App)
		cnames := ctx.Params[1].([]string)
		var conn *db.Storage
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		var cnamesDone []string
		for _, cname := range cnames {
			err = conn.Apps().Update(
				bson.M{"name": app.Name},
				bson.M{"$push": bson.M{"cname": cname}},
			)
			if err != nil {
				for _, c := range cnamesDone {
					conn.Apps().Update(
						bson.M{"name": app.Name},
						bson.M{"$pull": bson.M{"cname": c}},
					)
				}
				return nil, err
			}
			cnamesDone = append(cnamesDone, cname)
		}
		return cnamesDone, nil
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.Params[0].(*App)
		cnames := ctx.Params[1].([]string)
		conn, err := db.Conn()
		if err != nil {
			log.Errorf("BACKWARD add cnames db - unable to connect: %s", err)
			return
		}
		defer conn.Close()
		for _, c := range cnames {
			err := conn.Apps().Update(
				bson.M{"name": app.Name},
				bson.M{"$pull": bson.M{"cname": c}},
			)
			if err != nil {
				log.Errorf("BACKWARD add cnames db - unable to update: %s", err)
			}
		}
	},
}

var updateApp = action.Action{
	Name: "add-cname-update-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(*App)
		app.CName = append(app.CName, ctx.Params[1].([]string)...)
		return app.CName, nil
	},
}

var checkCNameExists = action.Action{
	Name: "cname-exists",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		cnames := ctx.Params[1].([]string)
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		for _, cname := range cnames {
			cs, err := conn.Apps().Find(bson.M{"cname": cname}).Count()
			if err != nil {
				return nil, err
			}
			if cs == 0 {
				return nil, errors.New(fmt.Sprintf("cname %s not exists in app", cname))
			}
		}
		return cnames, nil
	},
}

var unsetCNameFromProvisioner = action.Action{
	Name: "unset-cname-from-provisioner",
	Forward: func(ctx action.FWContext) (result action.Result, err error) {
		stdCtx := ctx.Context
		app := ctx.Params[0].(*App)
		cnames := ctx.Params[1].([]string)
		defer func() {
			if err != nil {
				setUnsetCnames(stdCtx, app, cnames, true)
			}
		}()
		for _, appRouter := range app.GetRouters() {
			var r router.Router
			r, err = router.Get(stdCtx, appRouter.Name)
			if err != nil {
				return nil, err
			}
			cnameRouter, ok := r.(router.CNameRouter)
			if !ok {
				continue
			}
			for _, cname := range cnames {
				err = cnameRouter.UnsetCName(ctx.Context, cname, app)
				if err != nil {
					return nil, err
				}
			}
		}
		return cnames, nil
	},
	Backward: func(ctx action.BWContext) {
		cnames := ctx.Params[1].([]string)
		app := ctx.Params[0].(*App)
		err := setUnsetCnames(ctx.Context, app, cnames, true)
		if err != nil {
			log.Errorf("BACKWARD unset cname - unable to set cnames in routers: %s", err)
		}
	},
}

var removeCNameFromDatabase = action.Action{
	Name: "remove-cname-from-database",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(*App)
		cnames := ctx.Params[1].([]string)
		var conn *db.Storage
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		var cnamesDone []string
		for _, cname := range cnames {
			err = conn.Apps().Update(
				bson.M{"name": app.Name},
				bson.M{"$pull": bson.M{"cname": cname}},
			)
			if err != nil {
				for _, c := range cnamesDone {
					conn.Apps().Update(
						bson.M{"name": app.Name},
						bson.M{"$push": bson.M{"cname": c}},
					)
				}
				return nil, err
			}
			cnamesDone = append(cnamesDone, cname)
		}
		return cnamesDone, nil
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.Params[0].(*App)
		cnames := ctx.Params[1].([]string)
		conn, err := db.Conn()
		if err != nil {
			log.Errorf("BACKWARD remove cname db - unable to connect to db: %s", err)
			return
		}
		defer conn.Close()
		for _, c := range cnames {
			err := conn.Apps().Update(
				bson.M{"name": app.Name},
				bson.M{"$push": bson.M{"cname": c}},
			)
			if err != nil {
				log.Errorf("BACKWARD remove cname db - unable to update: %s", err)
			}
		}
	},
}

var removeCNameFromApp = action.Action{
	Name: "remove-cname-from-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(*App)
		var conn *db.Storage
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		return nil, conn.Apps().Find(bson.M{"name": app.Name}).One(app)
	},
}

var swapCNamesInDatabaseAction = action.Action{
	Name: "swap-cnames-in-database",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app1 := ctx.Params[0].(*App)
		app2 := ctx.Params[1].(*App)

		return nil, swapCNamesInDatabase(app1, app2)
	},
}

var swapReEnsureBackendsAction = action.Action{
	Name: "re-ensure-backends",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app1 := ctx.Params[0].(*App)
		app2 := ctx.Params[1].(*App)

		return nil, swapRebuildRoutes(ctx.Context, app1, app2)
	},
	Backward: func(ctx action.BWContext) {
		app1 := ctx.Params[0].(*App)
		app2 := ctx.Params[1].(*App)

		err := swapCNamesInDatabase(app1, app2)
		if err != nil {
			return
		}

		_ = swapRebuildRoutes(ctx.Context, app1, app2)
	},
}

func swapCNamesInDatabase(app1 *App, app2 *App) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	app1.CName, app2.CName = app2.CName, app1.CName
	updateCName := func(app *App) error {
		return conn.Apps().Update(
			bson.M{"name": app.Name},
			bson.M{"$set": bson.M{"cname": app.CName}},
		)
	}
	err = updateCName(app1)
	if err != nil {
		return err
	}
	err = updateCName(app2)
	if err != nil {
		return err
	}

	return nil
}

func swapRebuildRoutes(ctx context.Context, app1 *App, app2 *App) error {
	// preserveOldCnames makes the operation without downtime, however when using diffent pools we cant do that
	preserveOldCNames := app1.Pool == app2.Pool
	_, err := rebuild.RebuildRoutes(ctx, rebuild.RebuildRoutesOpts{
		App:               app1,
		Wait:              true,
		PreserveOldCNames: preserveOldCNames,
	})
	if err != nil {
		return err
	}
	_, err = rebuild.RebuildRoutes(ctx, rebuild.RebuildRoutesOpts{
		App:               app2,
		Wait:              true,
		PreserveOldCNames: preserveOldCNames,
	})
	if err != nil {
		return err
	}

	app1.GetRoutersWithAddr()
	app2.GetRoutersWithAddr()

	return nil
}
