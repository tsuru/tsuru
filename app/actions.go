// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"io"
	"reflect"
	"regexp"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/rebuild"
	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
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
		envVars := []bind.EnvVar{
			{Name: "TSURU_APPNAME", Value: app.Name},
			{Name: "TSURU_APPDIR", Value: defaultAppDir},
			{Name: "TSURU_APP_TOKEN", Value: t.GetValue()},
		}
		err = app.SetEnvs(bind.SetEnvArgs{
			Envs:          envVars,
			ShouldRestart: false,
		})
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

var addRouterBackend = action.Action{
	Name: "add-router-backend",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var app *App
		switch ctx.Params[0].(type) {
		case *App:
			app = ctx.Params[0].(*App)
		default:
			return nil, errors.New("First parameter must be *App.")
		}
		r, err := app.GetRouter()
		if err != nil {
			return nil, err
		}
		if optsRouter, ok := r.(router.OptsRouter); ok {
			err = optsRouter.AddBackendOpts(app.GetName(), app.GetRouterOpts())
		} else {
			err = r.AddBackend(app.GetName())
		}
		return app, err
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.FWResult.(*App)
		r, err := app.GetRouter()
		if err != nil {
			log.Errorf("[add-router-backend rollback] unable to get app router: %s", err)
			return
		}
		err = r.RemoveBackend(app.GetName())
		if err != nil {
			log.Errorf("[add-router-backend rollback] unable to remove router backend: %s", err)
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
		r, err := app.GetRouter()
		if err != nil {
			return nil, err
		}
		app.IP, err = r.Addr(app.Name)
		if err != nil {
			return nil, err
		}
		err = conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$set": bson.M{"ip": app.IP}})
		if err != nil {
			return nil, err
		}
		return app, nil
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.FWResult.(*App)
		app.IP = ""
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

type updateAppPipelineResult struct {
	changedRouter bool
	changedOpts   bool
	oldPlan       *appTypes.Plan
	oldIp         string
	oldRouter     string
	oldRouterOpts map[string]string
	app           *App
}

var moveRouterUnits = action.Action{
	Name: "update-app-move-router-units",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app, ok := ctx.Params[0].(*App)
		if !ok {
			return nil, errors.New("first parameter must be an *App")
		}
		oldPlan, ok := ctx.Params[1].(*appTypes.Plan)
		if !ok {
			return nil, errors.New("second parameter must be a *Plan")
		}
		oldRouter, ok := ctx.Params[2].(string)
		if !ok {
			return nil, errors.New("third parameter must be a string")
		}
		newRouter := app.Router
		result := updateAppPipelineResult{
			oldPlan:   oldPlan,
			oldRouter: oldRouter,
			app:       app,
			oldIp:     app.IP,
		}
		if newRouter != oldRouter {
			_, err := rebuild.RebuildRoutes(app, false)
			if err != nil {
				return nil, err
			}
			result.changedRouter = true
		}
		return &result, nil
	},
	Backward: func(ctx action.BWContext) {
		result := ctx.FWResult.(*updateAppPipelineResult)
		defer func() {
			result.app.Plan = *result.oldPlan
			result.app.Router = result.oldRouter
		}()
		if result.changedRouter {
			app := result.app
			app.IP = result.oldIp
			conn, err := db.Conn()
			if err != nil {
				log.Errorf("BACKWARD move router units - failed to connect to the database: %s", err)
				return
			}
			defer conn.Close()
			conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$set": bson.M{"ip": app.IP}})
			r, err := result.app.GetRouter()
			if err != nil {
				log.Errorf("BACKWARD move router units - failed to retrieve router: %s", err)
				return
			}
			err = r.RemoveBackend(result.app.Name)
			if err != nil {
				log.Errorf("BACKWARD move router units - failed to remove backend: %s", err)
			}
		}
	},
}

var updateRouterOpts = action.Action{
	Name: "update-app-update-router-opts",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		result, ok := ctx.Previous.(*updateAppPipelineResult)
		if !ok {
			return nil, errors.New("invalid previous result, should be changePlanPipelineResult")
		}
		oldRouterOpts, ok := ctx.Params[3].(map[string]string)
		if !ok {
			return nil, errors.New("forth parameter must be a map[string]string")
		}
		if result.changedRouter {
			return result, nil
		}
		app := result.app
		if !reflect.DeepEqual(app.RouterOpts, oldRouterOpts) {
			r, err := app.GetRouter()
			if err != nil {
				return nil, err
			}
			if optsRouter, ok := r.(router.OptsRouter); ok {
				err := optsRouter.UpdateBackendOpts(app.Name, app.RouterOpts)
				if err != nil {
					return nil, err
				}
				result.changedOpts = true
			} else {
				log.Errorf("FORWARD move router units - router %q does not support opts", app.Router)
				return nil, errors.Errorf("router %q does not support opts", app.Router)
			}
		}
		return result, nil
	},
	Backward: func(ctx action.BWContext) {
		result := ctx.FWResult.(*updateAppPipelineResult)
		defer func() {
			result.app.RouterOpts = result.oldRouterOpts
		}()
		if result.changedRouter {
			return
		}
		app := result.app
		if result.changedOpts {
			r, err := app.GetRouter()
			if err != nil {
				log.Errorf("BACKWARD move router units - failed to retrieve router: %s", err)
				return
			}
			if optsRouter, ok := r.(router.OptsRouter); ok {
				err := optsRouter.UpdateBackendOpts(app.Name, result.oldRouterOpts)
				if err != nil {
					log.Errorf("BACKWARD move router units - update backend opts: %s", err)
					return
				}
			}
		}
	},
}

var saveApp = action.Action{
	Name: "update-app-save-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		result, ok := ctx.Previous.(*updateAppPipelineResult)
		if !ok {
			return nil, errors.New("invalid previous result, should be changePlanPipelineResult")
		}
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		update := bson.M{"$set": bson.M{"plan": result.app.Plan, "routername": result.app.Router, "routeropts": result.app.RouterOpts}}
		err = conn.Apps().Update(bson.M{"name": result.app.Name}, update)
		if err != nil {
			return nil, err
		}
		return result, nil
	},
	Backward: func(ctx action.BWContext) {
		result := ctx.FWResult.(*updateAppPipelineResult)
		conn, err := db.Conn()
		if err != nil {
			log.Errorf("BACKWARD save app - failed to get database connection: %s", err)
			return
		}
		defer conn.Close()
		update := bson.M{"$set": bson.M{"plan": *result.oldPlan, "routername": result.oldRouter, "routeropts": result.oldRouterOpts}}
		err = conn.Apps().Update(bson.M{"name": result.app.Name}, update)
		if err != nil {
			log.Errorf("BACKWARD save app - failed to update app: %s", err)
		}
	},
}

var restartApp = action.Action{
	Name: "update-app-restart-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		w, ok := ctx.Params[4].(io.Writer)
		if !ok {
			return nil, errors.New("forth parameter must be an io.Writer")
		}
		result, ok := ctx.Previous.(*updateAppPipelineResult)
		if !ok {
			return nil, errors.New("invalid previous result, should be changePlanPipelineResult")
		}
		err := result.app.Restart("", w)
		if err != nil {
			return nil, err
		}
		return result, nil
	},
}

// removeOldBackend never fails because restartApp is not undoable.
var removeOldBackend = action.Action{
	Name: "update-app-remove-old-backend",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		result, ok := ctx.Previous.(*updateAppPipelineResult)
		if !ok {
			return nil, errors.New("invalid previous result, should be changePlanPipelineResult")
		}
		if result.changedRouter {
			r, err := router.Get(result.oldRouter)
			if err != nil {
				log.Errorf("[IGNORED ERROR] failed to remove old backend: %s", err)
				return nil, nil
			}
			err = r.RemoveBackend(result.app.Name)
			if err != nil {
				log.Errorf("[IGNORED ERROR] failed to remove old backend: %s", err)
			}
		}
		return result, nil
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

var setNewCNamesToProvisioner = action.Action{
	Name: "set-new-cnames-to-provisioner",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(*App)
		cnames := ctx.Params[1].([]string)
		r, err := app.GetRouter()
		if err != nil {
			return nil, err
		}
		cnameRouter, ok := r.(router.CNameRouter)
		if !ok {
			return nil, errors.New("router does not support cname change")
		}
		var cnamesDone []string
		for _, cname := range cnames {
			err := cnameRouter.SetCName(cname, app.Name)
			if err != nil {
				for _, c := range cnamesDone {
					cnameRouter.UnsetCName(c, app.Name)
				}
				return nil, err
			}
			cnamesDone = append(cnamesDone, cname)
		}
		return cnames, nil
	},
	Backward: func(ctx action.BWContext) {
		cnames := ctx.Params[1].([]string)
		app := ctx.Params[0].(*App)
		r, err := app.GetRouter()
		if err != nil {
			log.Errorf("BACKWARD set cnames - unable to retrieve router: %s", err)
			return
		}
		cnameRouter, ok := r.(router.CNameRouter)
		if !ok {
			log.Errorf("BACKWARD set cnames - router doesn't support cname change.")
			return
		}
		for _, cname := range cnames {
			err := cnameRouter.UnsetCName(cname, app.Name)
			if err != nil {
				log.Errorf("BACKWARD set cnames - unable to unset cname: %s", err)
			}
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
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(*App)
		cnames := ctx.Params[1].([]string)
		r, err := app.GetRouter()
		if err != nil {
			return nil, err
		}
		cnameRouter, ok := r.(router.CNameRouter)
		if !ok {
			return nil, errors.New("router does not support cname change")
		}
		var cnamesDone []string
		for _, cname := range cnames {
			err := cnameRouter.UnsetCName(cname, app.Name)
			if err != nil {
				for _, c := range cnamesDone {
					cnameRouter.SetCName(c, app.Name)
				}
				return nil, err
			}
			cnamesDone = append(cnamesDone, cname)
		}
		return cnames, nil
	},
	Backward: func(ctx action.BWContext) {
		cnames := ctx.Params[1].([]string)
		app := ctx.Params[0].(*App)
		r, err := app.GetRouter()
		if err != nil {
			log.Errorf("BACKWARD unset cname - unable to retrieve router: %s", err)
			return
		}
		cnameRouter, ok := r.(router.CNameRouter)
		if !ok {
			log.Errorf("BACKWARD unset cname - router doesn't support cname change.")
			return
		}
		for _, cname := range cnames {
			err := cnameRouter.SetCName(cname, app.Name)
			if err != nil {
				log.Errorf("BACKWARD unset cname - unable to set cname: %s", err)
			}
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
