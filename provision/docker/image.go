// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	docker "github.com/fsouza/go-dockerclient"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/docker-cluster/storage"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision/dockercommon"
)

func MigrateImages() error {
	registry, _ := config.GetString("docker:registry")
	if registry != "" {
		registry += "/"
	}
	repoNamespace, _ := config.GetString("docker:repository-namespace")
	if repoNamespace == "" {
		repoNamespace = "tsuru"
	}
	apps, err := app.List(nil)
	if err != nil {
		return err
	}
	dcluster := mainDockerProvisioner.Cluster()
	for _, app := range apps {
		oldImage := registry + repoNamespace + "/" + app.GetName()
		newImage := registry + repoNamespace + "/app-" + app.GetName()
		containers, _ := mainDockerProvisioner.ListContainers(bson.M{"image": newImage, "appname": app.GetName()})
		if len(containers) > 0 {
			continue
		}
		opts := docker.TagImageOptions{Repo: newImage, Force: true}
		err = dcluster.TagImage(oldImage, opts)
		var baseErr error
		if nodeErr, ok := err.(cluster.DockerNodeError); ok {
			baseErr = nodeErr.BaseError()
		}
		if err != nil {
			if err == storage.ErrNoSuchImage || baseErr == docker.ErrNoSuchImage {
				continue
			}
			return err
		}
		if registry != "" {
			pushOpts := docker.PushImageOptions{
				Name:              newImage,
				InactivityTimeout: net.StreamInactivityTimeout,
			}
			err = dcluster.PushImage(pushOpts, dockercommon.RegistryAuthConfig(newImage))
			if err != nil {
				return err
			}
		}
		err = mainDockerProvisioner.updateContainers(bson.M{"appname": app.GetName()}, bson.M{"$set": bson.M{"image": newImage}})
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *dockerProvisioner) CleanImage(appName, imgName string) error {
	err := p.Cluster().RemoveImage(imgName)
	if err != nil && err != docker.ErrNoSuchImage && err != storage.ErrNoSuchImage {
		return err
	}
	return nil
}
