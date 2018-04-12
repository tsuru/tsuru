// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"io"
	"regexp"

	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"

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
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/router"
)

var (
	ErrAppAlreadyExists = errors.New("there is already an app with this name")
	ErrAppNotFound      = errors.New("App not found.")
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
		if err := servicemanager.AuthQuota.ReserveApp(usr, &usr.Quota); err != nil {
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
		app.Quota = appTypes.AppQuota{AppName: app.Name, Limit: -1, InUse: 0}
		var limit int
		if limit, err = config.GetInt("quota:units-per-app"); err == nil {
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

// createAppToken generates a token for the app and saves it in the database.
// It requires a pointer to an App instance as the first parameter.
var createAppToken = action.Action{
	Name: "create-app-token",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app, ok := ctx.Params[0].(*App)
		if !ok {
			return nil, errors.New("First parameter must be *App.")
		}
		app, err := GetByName(app.Name)
		if err != nil {
			return nil, err
		}
		t, err := AuthScheme.AppLogin(app.Name)
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
			AuthScheme.Logout(tokenValue)
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
		app, err := GetByName(app.Name)
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
		app, err := GetByName(app.Name)
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

// createRepository creates a repository for the app in the repository manager.
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
		allowedPerms := []permission.Permission{
			{
				Scheme:  permission.PermAppDeploy,
				Context: permission.Context(permission.CtxGlobal, ""),
			},
			{
				Scheme:  permission.PermAppDeploy,
				Context: permission.Context(permission.CtxPool, app.Pool),
			},
		}
		for _, t := range app.GetTeams() {
			allowedPerms = append(allowedPerms, permission.Permission{
				Scheme:  permission.PermAppDeploy,
				Context: permission.Context(permission.CtxTeam, t.Name),
			})
		}
		users, err := auth.ListUsersWithPermissions(allowedPerms...)
		if err != nil {
			return nil, err
		}
		userNames := make([]string, len(users))
		for i := range users {
			userNames[i] = users[i].Email
		}
		manager := repository.Manager()
		err = manager.CreateRepository(app.Name, userNames)
		if err != nil {
			return nil, err
		}
		return app, err
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.FWResult.(*App)
		repository.Manager().RemoveRepository(app.Name)
	},
	MinParams: 1,
}

func removeAllRoutersBackend(app *App) error {
	multi := tsuruErrors.NewMultiError()
	for _, appRouter := range app.GetRouters() {
		r, err := router.Get(appRouter.Name)
		if err != nil {
			multi.Add(err)
			continue
		}
		err = r.RemoveBackend(app.GetName())
		if err != nil && err != router.ErrBackendNotFound {
			multi.Add(err)
		}
	}
	return multi.ToError()
}

var addRouterBackend = action.Action{
	Name: "add-router-backend",
	Forward: func(ctx action.FWContext) (result action.Result, err error) {
		var app *App
		switch ctx.Params[0].(type) {
		case *App:
			app = ctx.Params[0].(*App)
		default:
			return nil, errors.New("First parameter must be *App.")
		}
		defer func() {
			if err != nil {
				removeAllRoutersBackend(app)
			}
		}()
		for _, appRouter := range app.GetRouters() {
			r, err := router.Get(appRouter.Name)
			if err != nil {
				return nil, err
			}
			if optsRouter, ok := r.(router.OptsRouter); ok {
				err = optsRouter.AddBackendOpts(app, appRouter.Opts)
			} else {
				err = r.AddBackend(app)
			}
			if err != nil {
				return nil, err
			}
		}
		return app, nil
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.FWResult.(*App)
		err := removeAllRoutersBackend(app)
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
		err = prov.Provision(app)
		if err != nil {
			return nil, err
		}
		return app, nil
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.FWResult.(*App)
		prov, err := app.getProvisioner()
		if err == nil {
			prov.Destroy(app)
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
		err = servicemanager.AppQuota.ReserveUnits(&app.Quota, n)
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
		err := servicemanager.AppQuota.ReleaseUnits(&app.Quota, qty)
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
		prov, err := app.getProvisioner()
		if err != nil {
			return nil, err
		}
		return nil, prov.AddUnits(app, uint(n), process, w)
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
		return nil, app.Restart("", w)
	},
	Backward: func(ctx action.BWContext) {
		oldApp := ctx.Params[1].(*App)
		w, _ := ctx.Params[2].(io.Writer)
		err := oldApp.Restart("", w)
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
		return nil, prov.Provision(app)
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.Params[0].(*App)
		prov, err := app.getProvisioner()
		if err != nil {
			log.Errorf("BACKWARD provision app - failed to get provisioner: %s", err)
		}
		err = prov.Destroy(app)
		if err != nil {
			log.Errorf("BACKWARD provision app - failed to destroy app in prov: %s", err)
		}
	},
}

var provisionAppAddUnits = action.Action{
	Name: "provision-app-add-unit",
	Forward: func(ctx action.FWContext) (action.Result, error) {
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
		unitCount := make(map[string]uint)
		for _, u := range units {
			unitCount[u.ProcessName]++
		}
		for process, count := range unitCount {
			err = app.AddUnits(count, process, w)
			if err != nil {
				return nil, err
			}
		}
		return nil, err
	},
	Backward: func(ctx action.BWContext) {
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
		return nil, oldProv.Destroy(oldApp)
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

func setUnsetCnames(app *App, cnames []string, toSet bool) error {
	multi := tsuruErrors.NewMultiError()
	for _, appRouter := range app.GetRouters() {
		r, err := router.Get(appRouter.Name)
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
				err = cnameRouter.SetCName(c, app.Name)
				if err == router.ErrCNameExists {
					err = nil
				}
			} else {
				err = cnameRouter.UnsetCName(c, app.Name)
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
		app := ctx.Params[0].(*App)
		cnames := ctx.Params[1].([]string)
		defer func() {
			if err != nil {
				setUnsetCnames(app, cnames, false)
			}
		}()
		for _, appRouter := range app.GetRouters() {
			var r router.Router
			r, err = router.Get(appRouter.Name)
			if err != nil {
				return nil, err
			}
			cnameRouter, ok := r.(router.CNameRouter)
			if !ok {
				continue
			}
			for _, cname := range cnames {
				err = cnameRouter.SetCName(cname, app.Name)
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
		err := setUnsetCnames(app, cnames, false)
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
		app := ctx.Params[0].(*App)
		cnames := ctx.Params[1].([]string)
		defer func() {
			if err != nil {
				setUnsetCnames(app, cnames, true)
			}
		}()
		for _, appRouter := range app.GetRouters() {
			var r router.Router
			r, err = router.Get(appRouter.Name)
			if err != nil {
				return nil, err
			}
			cnameRouter, ok := r.(router.CNameRouter)
			if !ok {
				continue
			}
			for _, cname := range cnames {
				err = cnameRouter.UnsetCName(cname, app.Name)
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
		err := setUnsetCnames(app, cnames, true)
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
