// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"gopkg.in/mgo.v2/bson"
)

func migrateImages() error {
	registry, _ := config.GetString("config:registry")
	if registry != "" {
		registry += "/"
	}
	repoNamespace, err := config.GetString("docker:repository-namespace")
	if err != nil {
		return err
	}
	apps, err := app.List(nil)
	if err != nil {
		return err
	}
	cluster := dockerCluster()
	for _, app := range apps {
		oldImage := registry + repoNamespace + "/" + app.Name
		newImage := registry + repoNamespace + "/app-" + app.Name
		opts := docker.TagImageOptions{Repo: newImage, Force: true}
		err = cluster.TagImage(oldImage, opts)
		if err != nil && err != docker.ErrNoSuchImage {
			return err
		}
		err = updateContainers(bson.M{"appname": app.Name}, bson.M{"$set": bson.M{"image": newImage}})
		if err != nil {
			return err
		}
		cluster.RemoveImage(oldImage)
	}
	return nil
}
