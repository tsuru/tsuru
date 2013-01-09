// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/go-gandalfclient"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/repository"
	"labix.org/v2/mgo/bson"
	"strconv"
)

// oldInsertApp is an implementation for the action interface.
type oldInsertApp struct{}

// oldInsertApp forward stores the app with "pending" as your state.
func (a *oldInsertApp) forward(app *App, args ...interface{}) error {
	app.State = "pending"
	return db.Session.Apps().Insert(app)
}

// oldInsertApp backward removes the app from the database.
func (a *oldInsertApp) backward(app *App, args ...interface{}) {
	db.Session.Apps().Remove(bson.M{"name": app.Name})
}

func (a *oldInsertApp) rollbackItself() bool {
	return false
}

// oldCreateBucketIam is an implementation for the action interface.
type oldCreateBucketIam struct{}

// oldCreateBucketIam forward creates a bucket and exports
// the related info as environs in the app machine.
func (a *oldCreateBucketIam) forward(app *App, args ...interface{}) error {
	env, err := createBucket(app)
	if err != nil {
		return err
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
	return nil
}

// oldCreateBucketIam backward destroys the app bucket.
func (a *oldCreateBucketIam) backward(app *App, args ...interface{}) {
	destroyBucket(app)
}

func (a *oldCreateBucketIam) rollbackItself() bool {
	return true
}

// provisionApp is an implementation for the action interface.
type provisionApp struct{}

// provision forward provisions the app.
func (a *provisionApp) forward(app *App, args ...interface{}) error {
	var units uint
	if len(args) > 0 {
		switch args[0].(type) {
		case int:
			units = uint(args[0].(int))
		case int64:
			units = uint(args[0].(int64))
		case uint:
			units = args[0].(uint)
		case uint64:
			units = uint(args[0].(uint64))
		default:
			units = 1
		}
	}
	err := Provisioner.Provision(app)
	if err != nil {
		return err
	}
	if units > 1 {
		_, err = Provisioner.AddUnits(app, units-1)
		return err
	}
	return nil
}

// provision backward does nothing.
func (a *provisionApp) backward(app *App, args ...interface{}) {}

func (a *provisionApp) rollbackItself() bool {
	return false
}

// createRepository is an implementation for the action interface.
type createRepository struct{}

// createRepository forward creates a git repository using the
// gandalf client.
func (a *createRepository) forward(app *App, args ...interface{}) error {
	gUrl := repository.GitServerUri()
	var users []string
	for _, t := range app.GetTeams() {
		users = append(users, t.Users...)
	}
	c := gandalf.Client{Endpoint: gUrl}
	_, err := c.NewRepository(app.Name, users, false)
	return err
}

// createRepository backward remove the git repository
// using the gandalf client.
func (a *createRepository) backward(app *App, args ...interface{}) {
	gUrl := repository.GitServerUri()
	c := gandalf.Client{Endpoint: gUrl}
	c.RemoveRepository(app.Name)
}

func (a *createRepository) rollbackItself() bool {
	return false
}
