// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/docker-cluster/storage"
	"github.com/tsuru/tsuru/app"
	"gopkg.in/mgo.v2/bson"
)

func migrateImages() error {
	registry, _ := config.GetString("docker:registry")
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
	dcluster := dockerCluster()
	for _, app := range apps {
		oldImage := registry + repoNamespace + "/" + app.Name
		newImage := registry + repoNamespace + "/app-" + app.Name
		opts := docker.TagImageOptions{Repo: newImage, Force: true}
		err = dcluster.TagImage(oldImage, opts)
		var baseErr error
		if nodeErr, ok := err.(cluster.DockerNodeError); ok {
			baseErr = nodeErr.BaseError()
		}
		if err != nil && err != storage.ErrNoSuchImage && baseErr != docker.ErrNoSuchImage {
			return err
		}
		if registry != "" {
			pushOpts := docker.PushImageOptions{Name: newImage}
			err = dcluster.PushImage(pushOpts, docker.AuthConfiguration{})
			if err != nil {
				return err
			}
		}
		err = updateContainers(bson.M{"appname": app.Name}, bson.M{"$set": bson.M{"image": newImage}})
		if err != nil {
			return err
		}
	}
	return nil
}
