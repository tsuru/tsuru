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
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	eventTypes "github.com/tsuru/tsuru/types/event"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/quota"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// mockDeployFunc can be set in tests to mock the Deploy function.
var mockDeployFunc func(ctx context.Context, opts DeployOptions) (string, error)

var (
	ErrAppAlreadyExists                      = errors.New("there is already an app with this name")
	ErrCNameDoesNotExist                     = errors.New("cname does not exist in app")
	ErrCertIssuerNotAllowedByPoolConstraints = errors.New("cert issuer not allowed by constraints of this pool")
)

var reserveTeamApp = action.Action{
	Name: "reserve-team-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var app *appTypes.App
		switch ctx.Params[0].(type) {
		case *appTypes.App:
			app = ctx.Params[0].(*appTypes.App)
		default:
			return nil, errors.New("first parameter must be *App.")
		}
		if err := servicemanager.TeamQuota.Inc(ctx.Context, &authTypes.Team{Name: app.TeamOwner}, 1); err != nil {
			return nil, err
		}
		return map[string]string{"app": app.Name, "team": app.TeamOwner}, nil
	},
	Backward: func(ctx action.BWContext) {
		m := ctx.FWResult.(map[string]string)
		if teamStr, ok := m["team"]; ok {
			servicemanager.TeamQuota.Inc(ctx.Context, &authTypes.Team{Name: teamStr}, -1)
		}
	},
	MinParams: 2,
}

// reserveUserApp reserves the app for the user, only if the user has a quota
// of apps. If the user does not have a quota, meaning that it's unlimited,
// reserveUserApp.Forward just return nil.
var reserveUserApp = action.Action{
	Name: "reserve-user-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var app *appTypes.App
		switch ctx.Params[0].(type) {
		case *appTypes.App:
			app = ctx.Params[0].(*appTypes.App)
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
		if user.FromToken {
			// there's no quota to update as the user was generated from team token.
			return map[string]string{"app": app.Name}, nil
		}
		u := auth.User(user)
		if err := servicemanager.UserQuota.Inc(ctx.Context, &u, 1); err != nil {
			return nil, err
		}
		return map[string]string{"app": app.Name, "user": user.Email}, nil
	},
	Backward: func(ctx action.BWContext) {
		m, found := ctx.FWResult.(map[string]string)
		if !found {
			return
		}
		email, found := m["user"]
		if !found {
			return
		}
		if user, err := auth.GetUserByEmail(ctx.Context, email); err == nil {
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
		var app *appTypes.App
		switch ctx.Params[0].(type) {
		case *appTypes.App:
			app = ctx.Params[0].(*appTypes.App)
		default:
			return nil, errors.New("First parameter must be *App.")
		}
		err := createApp(ctx.Context, app)
		if err != nil {
			return nil, err
		}
		return app, nil
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.FWResult.(*appTypes.App)
		removeApp(ctx.Context, app)
	},
	MinParams: 1,
}

func createApp(ctx context.Context, app *appTypes.App) error {
	collection, err := storagev2.AppsCollection()
	if err != nil {
		return err
	}
	if app.Quota == (quota.Quota{}) {
		app.Quota = quota.UnlimitedQuota
	}
	var limit int
	if limit, err = config.GetInt("quota:units-per-app"); err == nil {
		app.Quota.Limit = limit
	}
	_, err = collection.InsertOne(ctx, app)
	if mongo.IsDuplicateKeyError(err) {
		return ErrAppAlreadyExists
	}

	return nil
}

func removeApp(ctx context.Context, app *appTypes.App) error {
	collection, err := storagev2.AppsCollection()
	if err != nil {
		return err
	}

	collection.DeleteOne(ctx, mongoBSON.M{"name": app.Name})
	return nil
}

// exportEnvironmentsAction exports tsuru's default environment variables in a
// new app. It requires a pointer to an App instance as the first parameter.
var exportEnvironmentsAction = action.Action{
	Name: "export-environments",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(*appTypes.App)
		app, err := GetByName(ctx.Context, app.Name)
		if err != nil {
			return nil, err
		}
		envVars := []bindTypes.EnvVar{
			{Name: "TSURU_APPNAME", Value: app.Name},
			{Name: "TSURU_APPDIR", Value: appTypes.DefaultAppDir},
		}

		err = SetEnvs(ctx.Context, app, bindTypes.SetEnvArgs{
			Envs:          envVars,
			ShouldRestart: false,
		})
		if err != nil {
			return nil, err
		}
		return app, nil
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.Params[0].(*appTypes.App)
		app, err := GetByName(ctx.Context, app.Name)
		if err == nil {
			vars := []string{"TSURU_APPNAME", "TSURU_APPDIR"}

			UnsetEnvs(ctx.Context, app, bindTypes.UnsetEnvArgs{
				VariableNames: vars,
				ShouldRestart: true,
			})
		}
	},
	MinParams: 1,
}

var provisionApp = action.Action{
	Name: "provision-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var app *appTypes.App
		switch ctx.Params[0].(type) {
		case *appTypes.App:
			app = ctx.Params[0].(*appTypes.App)
		default:
			return nil, errors.New("First parameter must be *App.")
		}
		prov, err := getProvisioner(ctx.Context, app)
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
		app := ctx.FWResult.(*appTypes.App)
		prov, err := getProvisioner(ctx.Context, app)
		if err == nil {
			prov.Destroy(ctx.Context, app)
		}
	},
	MinParams: 1,
}

var bootstrapDeployApp = action.Action{
	Name: "bootstrap-deploy-app",
	Forward: func(ctx action.FWContext) (result action.Result, err error) {
		var app *appTypes.App
		switch ctx.Params[0].(type) {
		case *appTypes.App:
			app = ctx.Params[0].(*appTypes.App)
		default:
			return nil, errors.New("First parameter must be *App.")
		}

		bootstrapDeployImage, _ := config.GetString("apps:bootstrap:image")

		if bootstrapDeployImage == "" {
			return app, nil
		}

		var evt *event.Event
		evt, err = event.NewInternal(ctx.Context, &event.Opts{
			Target:       eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: app.Name},
			InternalKind: "bootstrap deploy",
			DisableLock:  true,
			Allowed:      event.Allowed(permission.PermAppReadEvents, permission.Context(permTypes.CtxApp, app.Name)),
		})
		if err != nil {
			return nil, errors.Wrap(err, "unable to create event")
		}
		defer evt.Done(ctx.Context, err)

		opts := &DeployOptions{
			App:          app,
			Image:        bootstrapDeployImage,
			Event:        evt,
			Message:      "bootstrap deploy",
			OutputStream: io.Discard,
		}

		opts.GetKind()

		deployFn := Deploy
		if mockDeployFunc != nil {
			deployFn = mockDeployFunc
		}

		_, err = deployFn(ctx.Context, *opts)
		if err != nil {
			return nil, err
		}

		return app, nil
	},
	Backward: func(ctx action.BWContext) {
		// no rollback needed
	},
	MinParams: 1,
}

var reserveUnitsToAdd = action.Action{
	Name: "reserve-units-to-add",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var app *appTypes.App
		switch ctx.Params[0].(type) {
		case *appTypes.App:
			app = ctx.Params[0].(*appTypes.App)
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
		app, err := GetByName(ctx.Context, app.Name)
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
		var app *appTypes.App
		switch ctx.Params[0].(type) {
		case *appTypes.App:
			app = ctx.Params[0].(*appTypes.App)
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
		var app *appTypes.App
		switch ctx.Params[0].(type) {
		case *appTypes.App:
			app = ctx.Params[0].(*appTypes.App)
		default:
			return nil, errors.New("First parameter must be *App.")
		}
		w, _ := ctx.Params[2].(io.Writer)
		n := ctx.Previous.(int)
		process := ctx.Params[3].(string)
		version := ctx.Params[4].(appTypes.AppVersion)
		prov, err := getProvisioner(ctx.Context, app)
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
		app, ok := ctx.Params[0].(*appTypes.App)
		if !ok {
			return nil, errors.New("expected app ptr as first arg")
		}
		collection, err := storagev2.AppsCollection()
		if err != nil {
			return nil, err
		}

		_, err = collection.ReplaceOne(ctx.Context, mongoBSON.M{"name": app.Name}, app)
		return nil, err
	},
	Backward: func(ctx action.BWContext) {
		oldApp := ctx.Params[1].(*appTypes.App)
		collection, err := storagev2.AppsCollection()
		if err != nil {
			log.Errorf("BACKWARD save app - failed to get database connection: %s", err)
			return
		}

		_, err = collection.ReplaceOne(ctx.Context, mongoBSON.M{"name": oldApp.Name}, oldApp)
		if err != nil {
			log.Errorf("BACKWARD save app - failed to update app: %s", err)
		}
	},
}

var restartApp = action.Action{
	Name: "update-app-restart-app",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app, ok := ctx.Params[0].(*appTypes.App)
		if !ok {
			return nil, errors.New("expected app ptr as first arg")
		}
		w, _ := ctx.Params[2].(io.Writer)
		return nil, Restart(ctx.Context, app, "", "", w)
	},
	Backward: func(ctx action.BWContext) {
		oldApp := ctx.Params[1].(*appTypes.App)
		w, _ := ctx.Params[2].(io.Writer)
		err := Restart(ctx.Context, oldApp, "", "", w)
		if err != nil {
			log.Errorf("BACKWARD update app - failed to restart app: %s", err)
		}
	},
}

var provisionAppNewProvisioner = action.Action{
	Name: "provision-app-new-provisioner",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app, ok := ctx.Params[0].(*appTypes.App)
		if !ok {
			return nil, errors.New("expected app ptr as first arg")
		}
		prov, err := getProvisioner(ctx.Context, app)
		if err != nil {
			return nil, err
		}
		return nil, prov.Provision(ctx.Context, app)
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.Params[0].(*appTypes.App)
		prov, err := getProvisioner(ctx.Context, app)
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
		app, ok := ctx.Params[0].(*appTypes.App)
		if !ok {
			return nil, errors.New("expected app ptr as first arg")
		}
		oldApp, ok := ctx.Params[1].(*appTypes.App)
		if !ok {
			return nil, errors.New("expected app ptr as first arg")
		}
		w, _ := ctx.Params[2].(io.Writer)
		units, err := AppUnits(ctx.Context, oldApp)
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
				err = rebuild.RebuildRoutes(ctx.Context, rebuild.RebuildRoutesOpts{App: app})
			}
		}()
		for processData, count := range unitCount {
			var version string
			if processData.version > 0 {
				version = strconv.Itoa(processData.version)
			}
			err = AddUnits(ctx.Context, app, count, processData.process, version, w)
			if err != nil {
				return nil, err
			}
		}
		return nil, err
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.Params[0].(*appTypes.App)
		rebuild.RebuildRoutesWithAppName(app.Name, nil)
	},
}

var destroyAppOldProvisioner = action.Action{
	Name: "destroy-app-old-provisioner",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		oldApp, ok := ctx.Params[1].(*appTypes.App)
		if !ok {
			return nil, errors.New("expected app ptr as second arg")
		}
		oldProv, err := getProvisioner(ctx.Context, oldApp)
		if err != nil {
			return nil, err
		}
		return nil, oldProv.Destroy(ctx.Context, oldApp)
	},
}

var updateAppProvisioner = action.Action{
	Name: "update-app-provisioner",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app, ok := ctx.Params[0].(*appTypes.App)
		if !ok {
			return nil, errors.New("expected app ptr as first arg")
		}
		oldApp, ok := ctx.Params[1].(*appTypes.App)
		if !ok {
			return nil, errors.New("expected app ptr as second arg")
		}
		oldProv, err := getProvisioner(ctx.Context, oldApp)
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
		app := ctx.Params[0].(*appTypes.App)
		oldApp := ctx.Params[1].(*appTypes.App)
		newProv, err := getProvisioner(ctx.Context, app)
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
		app := ctx.Params[0].(*appTypes.App)
		cnameRegexp := regexp.MustCompile(`^(\*\.)?[a-zA-Z0-9][\w-.]+$`)
		cnames := ctx.Params[1].([]string)
		collection, err := storagev2.AppsCollection()
		if err != nil {
			return nil, err
		}
		appRouters := GetRouters(app)
		for _, cname := range cnames {
			if !cnameRegexp.MatchString(cname) {
				return nil, errors.New("Invalid cname")
			}
			cs, err := collection.CountDocuments(ctx.Context, mongoBSON.M{"cname": cname})
			if err != nil {
				return nil, err
			}
			if cs == 0 {
				continue
			}
			cs, err = collection.CountDocuments(ctx.Context, mongoBSON.M{"name": app.Name, "cname": cname})
			if err != nil {
				return nil, err
			}
			if cs > 0 {
				return nil, errors.New(fmt.Sprintf("cname %s already exists for this app", cname))
			}

			appCName := appTypes.App{}
			err = collection.FindOne(ctx.Context, mongoBSON.M{"cname": cname, "name": mongoBSON.M{"$ne": app.Name}, "routers": mongoBSON.M{"$in": appRouters}}).Decode(&appCName)
			if err != nil && err != mongo.ErrNoDocuments {
				return nil, err
			}
			if appCName.Name != "" {
				return nil, errors.New(fmt.Sprintf("cname %s already exists for app %s using same router", cname, appCName.Name))
			}
			err = collection.FindOne(ctx.Context, mongoBSON.M{"cname": cname, "name": mongoBSON.M{"$ne": app.Name}, "teamowner": mongoBSON.M{"$ne": app.TeamOwner}}).Decode(&appCName)
			if err != nil {
				if err == mongo.ErrNoDocuments {
					continue
				}
				return nil, err
			}
			return nil, errors.New(fmt.Sprintf("cname %s already exists for another app %s and belongs to a different team owner", cname, appCName.Name))
		}
		return cnames, nil
	},
}

var saveCNames = action.Action{
	Name: "add-cname-save-in-database",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(*appTypes.App)
		cnames := ctx.Params[1].([]string)
		collection, err := storagev2.AppsCollection()
		if err != nil {
			return nil, err
		}

		_, err = collection.UpdateOne(
			ctx.Context,
			mongoBSON.M{"name": app.Name},
			mongoBSON.M{"$addToSet": mongoBSON.M{"cname": mongoBSON.M{"$each": cnames}}},
		)

		if err != nil {
			return nil, err
		}
		return cnames, nil
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.Params[0].(*appTypes.App)
		cnames := ctx.Params[1].([]string)
		collection, err := storagev2.AppsCollection()
		if err != nil {
			log.Errorf("BACKWARD add cnames db - unable to connect: %s", err)
			return
		}

		// TODO: may use $pullAll instead of loop, but requires mongo > 5.0
		for _, c := range cnames {
			_, err := collection.UpdateOne(
				ctx.Context,
				mongoBSON.M{"name": app.Name},
				mongoBSON.M{"$pull": mongoBSON.M{"cname": c}},
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
		app := ctx.Params[0].(*appTypes.App)
		app.CName = append(app.CName, ctx.Params[1].([]string)...)
		return app.CName, nil
	},
}

var checkCNameExists = action.Action{
	Name: "cname-exists",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		cnames := ctx.Params[1].([]string)
		collection, err := storagev2.AppsCollection()
		if err != nil {
			return nil, err
		}
		for _, cname := range cnames {
			cs, err := collection.CountDocuments(ctx.Context, mongoBSON.M{"cname": cname})
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

var removeCNameFromDatabase = action.Action{
	Name: "remove-cname-from-database",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(*appTypes.App)
		cnames := ctx.Params[1].([]string)
		collection, err := storagev2.AppsCollection()
		if err != nil {
			return nil, err
		}
		var cnamesDone []string
		for _, cname := range cnames {
			// TODO: use $pullAll when uses mongo 5.0
			_, err = collection.UpdateOne(
				ctx.Context,
				mongoBSON.M{"name": app.Name},
				mongoBSON.M{"$pull": mongoBSON.M{"cname": cname}},
			)
			if err != nil {
				_, revertErr := collection.UpdateOne(
					ctx.Context,
					mongoBSON.M{"name": app.Name},
					mongoBSON.M{"$addToSet": mongoBSON.M{"cname": mongoBSON.M{"$each": cnamesDone}}},
				)
				if revertErr != nil {
					log.Errorf("BACKWARD remove cname db - unable to revert update: %s", revertErr)
				}

				return nil, err
			}
			cnamesDone = append(cnamesDone, cname)
		}
		return cnamesDone, nil
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.Params[0].(*appTypes.App)
		cnames := ctx.Params[1].([]string)
		collection, err := storagev2.AppsCollection()
		if err != nil {
			log.Errorf("BACKWARD remove cname db - unable to connect to db: %s", err)
			return
		}

		_, revertErr := collection.UpdateOne(
			ctx.Context,
			mongoBSON.M{"name": app.Name},
			mongoBSON.M{"$addToSet": mongoBSON.M{"cname": mongoBSON.M{"$each": cnames}}},
		)
		if revertErr != nil {
			log.Errorf("BACKWARD remove cname db - unable to revert update: %s", revertErr)
		}
	},
}

var rebuildRoutes = action.Action{
	Name: "rebuild-routes",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(*appTypes.App)

		err := rebuild.RebuildRoutesWithAppName(app.Name, nil)
		if err != nil {
			return nil, err
		}
		return nil, nil
	},
}

var checkSingleCNameExists = action.Action{
	Name: "cname-exists",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		cname := ctx.Params[1].(string)

		collection, err := storagev2.AppsCollection()
		if err != nil {
			return nil, err
		}

		cs, err := collection.CountDocuments(ctx.Context, mongoBSON.M{"cname": cname})
		if err != nil {
			return nil, err
		}
		if cs == 0 {
			return nil, ErrCNameDoesNotExist
		}

		return cname, nil
	},
}

var checkCertIssuerPoolConstraints = action.Action{
	Name: "validate-cert-issuer-constraint",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(*appTypes.App)
		issuer := ctx.Params[2].(string)

		appPool, err := pool.GetPoolByName(ctx.Context, app.Pool)
		if err != nil {
			return nil, err
		}
		certIssuerConstraint, err := appPool.GetCertIssuers(ctx.Context)
		if err != nil {
			if errors.Is(err, pool.ErrPoolHasNoCertIssuerConstraint) {
				return issuer, nil
			}
			return nil, err
		}

		issuerMatchValues := false
		for _, value := range certIssuerConstraint.Values {
			if value == issuer {
				issuerMatchValues = true
				break
			}
		}
		if certIssuerConstraint.Blacklist {
			if issuerMatchValues {
				return nil, fmt.Errorf("%w. not allowed values: %s", ErrCertIssuerNotAllowedByPoolConstraints, strings.Join(certIssuerConstraint.Values, ", "))
			}
		} else {
			if !issuerMatchValues {
				return nil, fmt.Errorf("%w. allowed values: %s", ErrCertIssuerNotAllowedByPoolConstraints, strings.Join(certIssuerConstraint.Values, ", "))
			}
		}
		return issuer, nil
	},
}

var saveCertIssuer = action.Action{
	Name: "save-cert-issuer",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(*appTypes.App)
		cname := ctx.Params[1].(string)
		issuer := ctx.Params[2].(string)

		collection, err := storagev2.AppsCollection()
		if err != nil {
			return nil, err
		}

		sanitizedCName := strings.ReplaceAll(cname, ".", appTypes.CertIssuerDotReplacement)
		certIssuerCName := fmt.Sprintf("certissuers.%s", sanitizedCName)

		_, err = collection.UpdateOne(
			ctx.Context,
			mongoBSON.M{"name": app.Name},
			mongoBSON.M{"$set": mongoBSON.M{certIssuerCName: issuer}},
		)
		return cname, err
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.Params[0].(*appTypes.App)
		cname := ctx.Params[1].(string)

		collection, err := storagev2.AppsCollection()
		if err != nil {
			log.Errorf("BACKWARD remove certissuer db. unable to connect: %s", err)
			return
		}

		sanitizedCName := strings.ReplaceAll(cname, ".", appTypes.CertIssuerDotReplacement)
		certIssuerCName := fmt.Sprintf("certissuers.%s", sanitizedCName)

		_, err = collection.UpdateOne(
			ctx.Context,
			mongoBSON.M{"name": app.Name},
			mongoBSON.M{"$unset": mongoBSON.M{certIssuerCName: ""}},
		)

		if err != nil {
			log.Errorf("BACKWARD remove certissuer db. failed to update: %s", err)
		}
	},
}

var removeCertIssuer = action.Action{
	Name: "remove-cert-issuer",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(*appTypes.App)
		cname := ctx.Params[1].(string)

		collection, err := storagev2.AppsCollection()
		if err != nil {
			return nil, err
		}

		sanitizedCName := strings.ReplaceAll(cname, ".", appTypes.CertIssuerDotReplacement)
		certIssuerCName := fmt.Sprintf("certissuers.%s", sanitizedCName)

		_, err = collection.UpdateOne(
			ctx.Context,
			mongoBSON.M{"name": app.Name},
			mongoBSON.M{"$unset": mongoBSON.M{certIssuerCName: ""}},
		)
		return cname, err
	},
}

var removeCertIssuersFromDatabase = action.Action{
	Name: "remove-cert-issuer",
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Params[0].(*appTypes.App)
		cname := ctx.Params[1].([]string)

		collection, err := storagev2.AppsCollection()
		if err != nil {
			return nil, err
		}

		for _, c := range cname {
			sanitizedCName := strings.ReplaceAll(c, ".", appTypes.CertIssuerDotReplacement)
			certIssuerCName := fmt.Sprintf("certissuers.%s", sanitizedCName)

			_, err = collection.UpdateOne(
				ctx.Context,
				mongoBSON.M{"name": app.Name},
				mongoBSON.M{"$unset": mongoBSON.M{certIssuerCName: ""}},
			)
		}
		return cname, err
	},
}
