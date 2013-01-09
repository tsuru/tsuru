// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
	"strconv"
)

// insertApp is an action that inserts an app in the database in Forward and
// removes it in the Backward.
//
// The first argument in the context must be an App or a pointer to an App.
var insertApp = &action.Action{
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
var createBucketIam = &action.Action{
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
		app.SetEnvsToApp(envVars, false, true)
		return &createBucketResult{app: app, env: env}, nil
	},
	Backward: func(ctx action.BWContext) {
		result := ctx.FWResult.(*createBucketResult)
		destroyBucket(result.app)
	},
	MinParams: 0,
}
