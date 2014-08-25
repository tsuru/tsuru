// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"io"
	"time"

	"github.com/tsuru/go-gandalfclient"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/mgo.v2/bson"
)

type deploy struct {
	ID        bson.ObjectId `bson:"_id,omitempty"`
	App       string
	Timestamp time.Time
	Duration  time.Duration
	Commit    string
	Error     string
}

func (app *App) ListDeploys() ([]deploy, error) {
	return listDeploys(app, nil)
}

func ListDeploys(s *service.Service) ([]deploy, error) {
	return listDeploys(nil, s)
}

func listDeploys(app *App, s *service.Service) ([]deploy, error) {
	var list []deploy
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var qr bson.M
	if app != nil {
		qr = bson.M{"app": app.Name}
	}
	if s != nil {
		var instances []service.ServiceInstance
		q := bson.M{"service_name": s.Name}
		err = conn.ServiceInstances().Find(q).All(&instances)
		if err != nil {
			return nil, err
		}
		var appNames []string
		for _, instance := range instances {
			for _, apps := range instance.Apps {
				appNames = append(appNames, apps)
			}
		}
		qr = bson.M{"app": bson.M{"$in": appNames}}
	}
	if err := conn.Deploys().Find(qr).Sort("-timestamp").All(&list); err != nil {
		return nil, err
	}
	return list, err
}

func GetDeploy(id string) (*deploy, error) {
	var dep deploy
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.Deploys().FindId(bson.ObjectIdHex(id)).One(&dep); err != nil {
		return nil, err
	}
	return &dep, nil
}

func GetDiffInDeploys(d *deploy) (string, error) {
	var list []deploy
	conn, err := db.Conn()
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if err := conn.Deploys().Find(bson.M{"app": d.App}).Sort("-timestamp").All(&list); err != nil {
		return "", err
	}
	if len(list) < 2 {
		return "", fmt.Errorf("The deployment must have at least two commits for the diff.")
	}
	gandalfClient := gandalf.Client{Endpoint: repository.ServerURL()}
	diffOutput, err := gandalfClient.GetDiff(d.App, list[1].Commit, list[0].Commit)
	if err != nil {
		return "", fmt.Errorf("Caught error getting repository metadata: %s", err.Error())
	}
	return diffOutput, nil
}

type DeployOptions struct {
	App          *App
	Version      string
	Commit       string
	ArchiveURL   string
	File         io.ReadCloser
	OutputStream io.Writer
}

// Deploy runs a deployment of an application. It will first try to run an
// archive based deploy (if opts.ArchiveURL is not empty), and then fallback to
// the Git based deployment.
func Deploy(opts DeployOptions) error {
	var pipeline *action.Pipeline
	start := time.Now()
	if cprovisioner, ok := Provisioner.(provision.CustomizedDeployPipelineProvisioner); ok {
		pipeline = cprovisioner.DeployPipeline()
	} else {
		actions := []*action.Action{&ProvisionerDeploy, &IncrementDeploy}
		pipeline = action.NewPipeline(actions...)
	}
	logWriter := LogWriter{App: opts.App, Writer: opts.OutputStream}
	err := pipeline.Execute(opts, &logWriter)
	elapsed := time.Since(start)
	if err != nil {
		saveDeployData(opts.App.Name, opts.Commit, elapsed, err)
		return err
	}
	if opts.App.UpdatePlatform == true {
		opts.App.SetUpdatePlatform(false)
	}
	return saveDeployData(opts.App.Name, opts.Commit, elapsed, nil)
}

func saveDeployData(appName, commit string, duration time.Duration, deployError error) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	deploy := deploy{
		App:       appName,
		Timestamp: time.Now(),
		Duration:  duration,
		Commit:    commit,
	}
	if deployError != nil {
		deploy.Error = deployError.Error()
	}
	return conn.Deploys().Insert(deploy)
}

func incrementDeploy(app *App) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Apps().Update(
		bson.M{"name": app.Name},
		bson.M{"$inc": bson.M{"deploys": 1}},
	)
}
