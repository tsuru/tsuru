// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/tsuru/tsuru/db"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2/bson"
)

type DeployData struct {
	ID          bson.ObjectId `bson:"_id,omitempty"`
	App         string
	Timestamp   time.Time
	Duration    time.Duration
	Commit      string
	Error       string
	Image       string
	Log         string
	User        string
	Origin      string
	CanRollback bool
	RemoveDate  time.Time `bson:",omitempty"`
	Diff        string
}

// ListDeploys returns the list of deploy that match a given filter.
func ListDeploys(filter *Filter, skip, limit int) ([]DeployData, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	appsList, err := List(filter)
	if err != nil {
		return nil, err
	}
	apps := make([]string, len(appsList))
	for i, a := range appsList {
		apps[i] = a.Name
	}
	var list []DeployData
	f := bson.M{"app": bson.M{"$in": apps}, "removedate": bson.M{"$exists": false}}
	s := bson.M{
		"app":         1,
		"timestamp":   1,
		"duration":    1,
		"commit":      1,
		"error":       1,
		"image":       1,
		"user":        1,
		"origin":      1,
		"canrollback": 1,
		"removedate":  1,
	}
	query := conn.Deploys().Find(f).Select(s).Sort("-timestamp")
	if skip != 0 {
		query = query.Skip(skip)
	}
	if limit != 0 {
		query = query.Limit(limit)
	}
	if err = query.All(&list); err != nil {
		return nil, err
	}
	validImages := set{}
	for _, appName := range apps {
		var imgs []string
		imgs, err = Provisioner.ValidAppImages(appName)
		if err != nil {
			return nil, err
		}
		validImages.Add(imgs...)
	}
	for i := range list {
		list[i].CanRollback = validImages.Includes(list[i].Image)
		r := regexp.MustCompile("v[0-9]+$")
		if list[i].Image != "" && r.MatchString(list[i].Image) {
			parts := r.FindAllStringSubmatch(list[i].Image, -1)
			list[i].Image = parts[0][0]
		}
	}
	return list, err
}

func markDeploysAsRemoved(appName string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Deploys().UpdateAll(
		bson.M{"app": appName, "removedate": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{"removedate": time.Now()}},
	)
	return err
}

func GetDeploy(id string) (*DeployData, error) {
	var dep DeployData
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if !bson.IsObjectIdHex(id) {
		return nil, fmt.Errorf("id parameter is not ObjectId: %s", id)
	}
	if err := conn.Deploys().FindId(bson.ObjectIdHex(id)).One(&dep); err != nil {
		return nil, err
	}
	return &dep, nil
}

type DeployOptions struct {
	App          *App
	Version      string
	Commit       string
	ArchiveURL   string
	File         io.ReadCloser
	OutputStream io.Writer
	User         string
	Image        string
	Origin       string
	Rollback     bool
}

// Deploy runs a deployment of an application. It will first try to run an
// archive based deploy (if opts.ArchiveURL is not empty), and then fallback to
// the Git based deployment.
func Deploy(opts DeployOptions) error {
	var outBuffer bytes.Buffer
	start := time.Now()
	logWriter := LogWriter{App: opts.App}
	logWriter.Async()
	defer logWriter.Close()
	writer := io.MultiWriter(&tsuruIo.NoErrorWriter{Writer: opts.OutputStream}, &outBuffer, &logWriter)
	elapsed := time.Since(start)
	saveErr := saveDeployData(&opts, "diff", "", elapsed, nil)
	if saveErr != nil {
		log.Errorf("WARNING: couldn't save deploy data, deploy opts: %#v", opts)
	}
	imageId, err := deployToProvisioner(&opts, writer)
	elapsed = time.Since(start)
	saveErr = saveDeployData(&opts, imageId, outBuffer.String(), elapsed, err)
	if saveErr != nil {
		log.Errorf("WARNING: couldn't save deploy data, deploy opts: %#v", opts)
	}
	if err != nil {
		return err
	}
	err = incrementDeploy(opts.App)
	if err != nil {
		log.Errorf("WARNING: couldn't increment deploy count, deploy opts: %#v", opts)
	}
	if opts.App.UpdatePlatform == true {
		opts.App.SetUpdatePlatform(false)
	}
	return nil
}

func deployToProvisioner(opts *DeployOptions, writer io.Writer) (string, error) {
	if opts.Rollback {
		return Provisioner.Rollback(opts.App, opts.Image, writer)
	}
	if opts.Image != "" {
		if deployer, ok := Provisioner.(provision.ImageDeployer); ok {
			return deployer.ImageDeploy(opts.App, opts.Image, writer)
		}
	}
	if opts.File != nil {
		if deployer, ok := Provisioner.(provision.UploadDeployer); ok {
			return deployer.UploadDeploy(opts.App, opts.File, writer)
		}
	}
	if opts.ArchiveURL != "" {
		if deployer, ok := Provisioner.(provision.ArchiveDeployer); ok {
			return deployer.ArchiveDeploy(opts.App, opts.ArchiveURL, writer)
		}
	}
	return Provisioner.(provision.GitDeployer).GitDeploy(opts.App, opts.Version, writer)
}

func saveDeployData(opts *DeployOptions, imageId, log string, duration time.Duration, deployError error) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	deploy := DeployData{
		App:       opts.App.Name,
		Timestamp: time.Now(),
		Duration:  duration,
		Commit:    opts.Commit,
		Image:     imageId,
		Log:       log,
		User:      opts.User,
	}
	if opts.Origin != "" {
		deploy.Origin = opts.Origin
	} else if opts.Commit != "" {
		deploy.Origin = "git"
	}
	if deployError != nil {
		deploy.Error = deployError.Error()
	}
	var dep []DeployData
	err = conn.Deploys().Find(bson.M{"app": opts.App.Name, "image": "diff"}).All(&dep)
	if err != nil {
		return err
	}
	if len(dep) == 1 {
		deploy.Diff = dep[0].Diff
	}
	query := bson.M{"$set": deploy}
	_, err = conn.Deploys().Upsert(bson.M{"app": deploy.App, "image": "diff"}, query)
	return err
}

func SaveDiffData(diff string, appName string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	deploy := DeployData{
		App:   appName,
		Image: "diff",
		Diff:  diff,
	}
	query := bson.M{"$set": deploy}
	_, err = conn.Deploys().Upsert(bson.M{"app": deploy.App, "image": "diff"}, query)
	return err
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

func getImage(appName string, img string) (string, error) {
	conn, err := db.Conn()
	if err != nil {
		return "", err
	}
	defer conn.Close()
	var deploy DeployData
	query := bson.M{"app": appName, "image": bson.M{"$regex": ".*:" + img + "$"}}
	if err := conn.Deploys().Find(query).One(&deploy); err != nil {
		return "", err
	}
	return deploy.Image, nil
}

func Rollback(opts DeployOptions) error {
	if !regexp.MustCompile(":v[0-9]+$").MatchString(opts.Image) {
		img, err := getImage(opts.App.Name, opts.Image)
		// err is not handled here because it is handled by Deploy
		if err == nil {
			opts.Image = img
		}
		opts.Rollback = true
	}
	return Deploy(opts)
}
