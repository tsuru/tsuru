// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"io"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/dockercommon"
)

func (p *dockerProvisioner) buildImage(app provision.App, archiveFile io.ReadCloser) (string, string, error) {
	user, err := config.GetString("docker:user")
	if err != nil {
		user, _ = config.GetString("docker:ssh:user")
	}
	imageName := image.GetBuildImage(app)
	options := docker.CreateContainerOptions{
		Config: &docker.Config{
			AttachStdout: true,
			AttachStderr: true,
			AttachStdin:  true,
			User:         user,
			Image:        imageName,
		},
	}
	cluster := p.Cluster()
	schedOpts := &container.SchedulerOpts{
		AppName:       app.GetName(),
		ActionLimiter: p.ActionLimiter(),
	}
	addr, cont, err := cluster.CreateContainerSchedulerOpts(options, schedOpts, net.StreamInactivityTimeout)
	hostAddr := net.URLToHost(addr)
	if schedOpts.LimiterDone != nil {
		schedOpts.LimiterDone()
	}
	if err != nil {
		return "", "", err
	}
	defer func() {
		done := p.ActionLimiter().Start(hostAddr)
		cluster.RemoveContainer(docker.RemoveContainerOptions{ID: cont.ID, Force: true})
		done()
	}()
	intermediateImageID, fileURI, err := dockercommon.UploadToContainer(cluster, cont.ID, archiveFile)
	if err != nil {
		return "", "", err
	}
	return intermediateImageID, fileURI, nil
}

func (p *dockerProvisioner) rebuildImage(app provision.App) (string, string, error) {
	filePath := "/home/application/archive.tar.gz"
	imageName, err := image.AppCurrentImageName(app.GetName())
	if err != nil {
		return "", "", errors.Errorf("App %s image not found", app.GetName())
	}
	createOptions := docker.CreateContainerOptions{
		Config: &docker.Config{
			AttachStdout: true,
			AttachStderr: true,
			Image:        imageName,
		},
	}
	cluster := p.Cluster()
	schedOpts := &container.SchedulerOpts{
		AppName:       app.GetName(),
		ActionLimiter: p.ActionLimiter(),
	}
	addr, cont, err := cluster.CreateContainerSchedulerOpts(createOptions, schedOpts, net.StreamInactivityTimeout)
	if schedOpts.LimiterDone != nil {
		schedOpts.LimiterDone()
	}
	hostAddr := net.URLToHost(addr)
	if err != nil {
		return "", "", err
	}
	defer func() {
		done := p.ActionLimiter().Start(hostAddr)
		cluster.RemoveContainer(docker.RemoveContainerOptions{ID: cont.ID, Force: true})
		done()
	}()
	archiveFile, err := dockercommon.DownloadFromContainer(cluster, cont.ID, filePath)
	if err != nil {
		return "", "", errors.Errorf("App %s raw image not found", app.GetName())
	}
	defer archiveFile.Close()
	return p.buildImage(app, archiveFile)
}
