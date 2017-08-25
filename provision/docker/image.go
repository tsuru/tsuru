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
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"gopkg.in/mgo.v2/bson"
)

func MigrateImages() error {
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
			err = dcluster.PushImage(pushOpts, dockercommon.RegistryAuthConfig())
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

func (p *dockerProvisioner) CleanImage(appName, imgName string) {
	shouldRemove := true
	err := p.Cluster().RemoveImage(imgName)
	if err != nil && err != docker.ErrNoSuchImage {
		shouldRemove = false
		log.Errorf("Ignored error removing old image %q: %s. Image kept on list to retry later.",
			imgName, err.Error())
	}
	err = p.Cluster().RemoveFromRegistry(imgName)
	if err != nil {
		shouldRemove = false
		log.Errorf("Ignored error removing old image from registry %q: %s. Image kept on list to retry later.",
			imgName, err.Error())
	}
	if shouldRemove {
		err = image.PullAppImageNames(appName, []string{imgName})
		if err != nil {
			log.Errorf("Ignored error pulling old images from database: %s", err)
		}
	}
}
