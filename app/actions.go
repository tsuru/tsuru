// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"regexp"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/router"
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
		err = app.setEnvsToApp(
			bind.SetEnvApp{
				Envs:          envVars,
				PublicOnly:    false,
				ShouldRestart: false,
			}, nil)
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
			app.UnsetEnvs(
				bind.UnsetEnvApp{
					VariableNames: vars,
					PublicOnly:    false,
					ShouldRestart: true,
				}, nil)
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
		w, _ := ctx.Params[2].(io.Writer)
		n := ctx.Previous.(int)
		process := ctx.Params[3].(string)
		units, err := Provisioner.AddUnits(app, uint(n), process, w)
		if err != nil {
			return nil, err
		}
		return units, nil
	},
	MinParams: 1,
}

type changePlanPipelineResult struct {
	changedRouter bool
	oldPlan       *Plan
	oldIp         string
	app           *App
}

var moveRouterUnits = action.Action{
	Name: "change-plan-move-router-units",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app, ok := ctx.Params[0].(*App)
		if !ok {
			return nil, errors.New("first parameter must be an *App")
		}
		oldPlan, ok := ctx.Params[1].(*Plan)
		if !ok {
			return nil, errors.New("second parameter must be a *Plan")
		}
		newRouter, err := app.GetRouter()
		if err != nil {
			return nil, err
		}
		oldRouter, err := oldPlan.getRouter()
		if err != nil {
			return nil, err
		}
		result := changePlanPipelineResult{oldPlan: oldPlan, app: app, oldIp: app.Ip}
		if newRouter != oldRouter {
			_, err = app.RebuildRoutes()
			if err != nil {
				return nil, err
			}
			result.changedRouter = true
		}
		return &result, nil
	},
	Backward: func(ctx action.BWContext) {
		result := ctx.FWResult.(*changePlanPipelineResult)
		defer func() {
			result.app.Plan = *result.oldPlan
		}()
		if result.changedRouter {
			app := result.app
			app.Ip = result.oldIp
			conn, err := db.Conn()
			if err != nil {
				log.Errorf("BACKWARD ABORTED - failed to connect to the database: %s", err)
				return
			}
			defer conn.Close()
			conn.Apps().Update(bson.M{"name": app.Name}, bson.M{"$set": bson.M{"ip": app.Ip}})
			routerName, err := result.app.GetRouter()
			if err != nil {
				log.Errorf("BACKWARD ABORTED - failed to get app router: %s", err)
				return
			}
			r, err := router.Get(routerName)
			if err != nil {
				log.Errorf("BACKWARD ABORTED - failed to retrieve router %q: %s", routerName, err)
				return
			}
			err = r.RemoveBackend(result.app.Name)
			if err != nil {
				log.Error(err.Error())
			}
		}
	},
}

var saveApp = action.Action{
	Name: "change-plan-save-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		result, ok := ctx.Previous.(*changePlanPipelineResult)
		if !ok {
			return nil, errors.New("invalid previous result, should be changePlanPipelineResult")
		}
		conn, err := db.Conn()
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		update := bson.M{"$set": bson.M{"plan": result.app.Plan}}
		err = conn.Apps().Update(bson.M{"name": result.app.Name}, update)
		if err != nil {
			return nil, err
		}
		return result, nil
	},
	Backward: func(ctx action.BWContext) {
		result := ctx.FWResult.(*changePlanPipelineResult)
		conn, err := db.Conn()
		if err != nil {
			log.Errorf("BACKWARD ABORTED - failed to get database connection: %s", err)
			return
		}
		defer conn.Close()
		update := bson.M{"$set": bson.M{"plan": *result.oldPlan}}
		err = conn.Apps().Update(bson.M{"name": result.app.Name}, update)
		if err != nil {
			log.Error(err.Error())
		}
	},
}

var restartApp = action.Action{
	Name: "change-plan-restart-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		w, ok := ctx.Params[2].(io.Writer)
		if !ok {
			log.Error("third parameter must be an io.Writer")
			w = ioutil.Discard
		}
		result, ok := ctx.Previous.(*changePlanPipelineResult)
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
	Name: "change-plan-remove-old-backend",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		result, ok := ctx.Previous.(*changePlanPipelineResult)
		if !ok {
			return nil, errors.New("invalid previous result, should be changePlanPipelineResult")
		}
		if result.changedRouter {
			routerName, err := result.oldPlan.getRouter()
			if err != nil {
				log.Errorf("[IGNORED ERROR] failed to remove old backend: %s", err)
				return nil, nil
			}
			r, err := router.Get(routerName)
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
		defer conn.Close()
		if err != nil {
			return nil, err
		}
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
		p, ok := Provisioner.(provision.CNameManager)
		if !ok {
			return nil, errors.New("Provisioner doesn't support cname change.")
		}
		app := ctx.Params[0].(*App)
		cnames := ctx.Params[1].([]string)
		var cnamesDone []string
		for _, cname := range cnames {
			err := p.SetCName(app, cname)
			if err != nil {
				for _, c := range cnamesDone {
					p.UnsetCName(app, c)
				}
				return nil, err
			}
			cnamesDone = append(cnamesDone, cname)
		}
		return cnames, nil
	},
	Backward: func(ctx action.BWContext) {
		p, ok := Provisioner.(provision.CNameManager)
		if !ok {
			log.Error("Provisioner doesn't support cname change.")
		}
		cnames := ctx.Params[1].([]string)
		app := ctx.Params[0].(*App)
		for _, cname := range cnames {
			err := p.UnsetCName(app, cname)
			if err != nil {
				log.Error(err.Error())
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
			log.Error(err.Error())
			return
		}
		defer conn.Close()
		for _, c := range cnames {
			err := conn.Apps().Update(
				bson.M{"name": app.Name},
				bson.M{"$pull": bson.M{"cname": c}},
			)
			if err != nil {
				log.Error(err.Error())
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
		defer conn.Close()
		if err != nil {
			return nil, err
		}
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
		p, ok := Provisioner.(provision.CNameManager)
		if !ok {
			return nil, errors.New("Provisioner doesn't support cname change.")
		}
		app := ctx.Params[0].(*App)
		cnames := ctx.Params[1].([]string)
		var cnamesDone []string
		for _, cname := range cnames {
			err := p.UnsetCName(app, cname)
			if err != nil {
				for _, c := range cnamesDone {
					p.SetCName(app, c)
				}
				return nil, err
			}
			cnamesDone = append(cnamesDone, cname)
		}
		return cnames, nil
	},
	Backward: func(ctx action.BWContext) {
		p, ok := Provisioner.(provision.CNameManager)
		if !ok {
			log.Error("Provisioner doesn't support cname change.")
		}
		cnames := ctx.Params[1].([]string)
		app := ctx.Params[0].(*App)
		for _, cname := range cnames {
			err := p.SetCName(app, cname)
			if err != nil {
				log.Error(err.Error())
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
			log.Error(err.Error())
			return
		}
		defer conn.Close()
		for _, c := range cnames {
			err := conn.Apps().Update(
				bson.M{"name": app.Name},
				bson.M{"$push": bson.M{"cname": c}},
			)
			if err != nil {
				log.Error(err.Error())
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
		err = conn.Apps().Find(bson.M{"name": app.Name}).One(app)
		if err != nil {
			return nil, err
		}
		return nil, nil
	},
}
