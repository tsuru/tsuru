// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"io"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/dockercommon"
)

func (p *dockerProvisioner) buildImage(app provision.App, archiveFile io.ReadCloser) (string, string, error) {
	user, _ := dockercommon.UserForContainer()
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

func (p *dockerProvisioner) Deploy(app provision.App, buildImageID string, evt *event.Event) (string, error) {
	deployImageID, err := image.AppVersionedImageName(app.GetName())
	if err != nil {
		return "", log.WrapError(errors.Errorf("error getting new image name for app %s", app.GetName()))
	}
	imageID, err := p.deployPipeline(app, buildImageID, deployImageID, nil, evt)
	err = p.deployAndClean(app, imageID, evt)
	if err != nil {
		return "", err
	}
	return imageID, nil
}

func (p *dockerProvisioner) GetDockerClient(app provision.App) (*docker.Client, error) {
	cluster := p.Cluster()
	nodes, err := cluster.NodesForMetadata(map[string]string{"pool": app.GetPool()})
	if err != nil {
		return nil, err
	}
	nodeAddr, _, err := p.scheduler.minMaxNodes(nodes, app.GetName(), "")
	if err != nil {
		return nil, err
	}
	node, err := cluster.GetNode(nodeAddr)
	if err != nil {
		return nil, err
	}
	client, err := node.Client()
	if err != nil {
		return nil, err
	}
	return client, nil
}
