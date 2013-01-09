// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/go-gandalfclient"
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/repository"
	"labix.org/v2/mgo/bson"
	"strconv"
)

// insertApp is an action that inserts an app in the database in Forward and
// removes it in the Backward.
//
// The first argument in the context must be an App or a pointer to an App.
var insertApp = action.Action{
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var app App
		switch ctx.Params[0].(type) {
		case App:
			app = ctx.Params[0].(App)
		case *App:
			app = *ctx.Params[0].(*App)
		default:
			return nil, errors.New("First parameter must be App or *App.")
		}
		app.State = "pending"
		err := db.Session.Apps().Insert(app)
		return &app, err
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.FWResult.(*App)
		db.Session.Apps().Remove(bson.M{"name": app.Name})
	},
	MinParams: 1,
}

type createBucketResult struct {
	app *App
	env *s3Env
}

// createBucketIam is an action that creates a bucket in S3, and a user,
// credentials and user policy in IAM.
//
// It does not receive any parameter. It expects an app to be in the Previous
// result, so this action cannot be the first in a pipeline.
//
// TODO(fss): break this action in smaller actions (createS3Bucket,
// createIAMUser, createIAMCredentials and createIAMUserPolicy).
var createBucketIam = action.Action{
	Forward: func(ctx action.FWContext) (action.Result, error) {
		app := ctx.Previous.(*App)
		env, err := createBucket(app)
		if err != nil {
			return nil, err
		}
		host, _ := config.GetString("host")
		envVars := []bind.EnvVar{
			{Name: "APPNAME", Value: app.Name},
			{Name: "TSURU_HOST", Value: host},
		}
		variables := map[string]string{
			"ENDPOINT":           env.endpoint,
			"LOCATIONCONSTRAINT": strconv.FormatBool(env.locationConstraint),
			"ACCESS_KEY_ID":      env.AccessKey,
			"SECRET_KEY":         env.SecretKey,
			"BUCKET":             env.bucket,
		}
		for name, value := range variables {
			envVars = append(envVars, bind.EnvVar{
				Name:         fmt.Sprintf("TSURU_S3_%s", name),
				Value:        value,
				InstanceName: s3InstanceName,
			})
		}
		err = app.SetEnvsToApp(envVars, false, true)
		if err != nil {
			return nil, err
		}
		return &createBucketResult{app: app, env: env}, nil
	},
	Backward: func(ctx action.BWContext) {
		result := ctx.FWResult.(*createBucketResult)
		destroyBucket(result.app)
	},
}

// createRepository creates a repository for the app in Gandalf.
var createRepository = action.Action{
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var app App
		switch ctx.Params[0].(type) {
		case App:
			app = ctx.Params[0].(App)
		case *App:
			app = *ctx.Params[0].(*App)
		default:
			return nil, errors.New("First parameter must be App or *App.")
		}
		gUrl := repository.GitServerUri()
		var users []string
		for _, t := range app.GetTeams() {
			users = append(users, t.Users...)
		}
		c := gandalf.Client{Endpoint: gUrl}
		_, err := c.NewRepository(app.Name, users, false)
		return &app, err
	},
	Backward: func(ctx action.BWContext) {
		app := ctx.FWResult.(*App)
		app.Get()
		gUrl := repository.GitServerUri()
		c := gandalf.Client{Endpoint: gUrl}
		c.RemoveRepository(app.Name)
	},
	MinParams: 1,
}

// provisionApp provisions the app in the provisioner. It takes two arguments:
// the app, and the number of units to create (an unsigned integer).
//
// TODO(fss): break this action in two small actions (provisionApp and
// addUnits).
var provisionApp = action.Action{
	Forward: func(ctx action.FWContext) (action.Result, error) {
		var (
			app   App
			units uint
		)
		switch ctx.Params[0].(type) {
		case App:
			app = ctx.Params[0].(App)
		case *App:
			app = *ctx.Params[0].(*App)
		default:
			return nil, errors.New("First parameter must be App or *App.")
		}
		switch ctx.Params[1].(type) {
		case int:
			units = uint(ctx.Params[1].(int))
		case int64:
			units = uint(ctx.Params[1].(int64))
		case uint:
			units = ctx.Params[1].(uint)
		case uint64:
			units = uint(ctx.Params[1].(uint64))
		default:
			units = 1
		}
		err := Provisioner.Provision(&app)
		if err != nil {
			return nil, err
		}
		if units > 1 {
			_, err = Provisioner.AddUnits(&app, units-1)
			return nil, err
		}
		return nil, nil
	},
	MinParams: 2,
}
