// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"crypto"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/dockercommon"
)

const (
	archiveDirPath  = "/home/application"
	archiveFileName = "archive.tar.gz"
)

func (b *dockerBuilder) buildPipeline(p provision.BuilderDeployDockerClient, client provision.BuilderDockerClient, app provision.App, tarFile io.Reader, evt *event.Event, imageTag string) (string, error) {
	actions := []*action.Action{
		&createContainer,
		&uploadToContainer,
		&startContainer,
		&followLogsAndCommit,
		&updateAppBuilderImage,
	}
	pipeline := action.NewPipeline(actions...)
	imageName := image.GetBuildImage(app)
	buildingImage, err := image.AppNewBuilderImageName(app.GetName(), app.GetTeamOwner(), imageTag)
	if err != nil {
		return "", log.WrapError(errors.Errorf("error getting new image name for app %s", app.GetName()))
	}
	archiveFileURI := fmt.Sprintf("file://%s/%s", archiveDirPath, archiveFileName)
	cmds := dockercommon.ArchiveBuildCmds(app, archiveFileURI)
	var writer io.Writer = evt
	if evt == nil {
		writer = ioutil.Discard
	}
	args := runContainerActionsArgs{
		app:           app,
		imageID:       imageName,
		commands:      cmds,
		writer:        writer,
		buildingImage: buildingImage,
		client:        client,
		event:         evt,
		provisioner:   p,
		tarFile:       tarFile,
		isDeploy:      true,
	}
	err = container.RunPipelineWithRetry(pipeline, args)
	if err != nil {
		log.Errorf("error on execute build pipeline for app %s - %s", app.GetName(), err)
		return "", err
	}
	return buildingImage, nil
}

func randomString() string {
	h := crypto.MD5.New()
	h.Write([]byte(time.Now().Format(time.RFC3339Nano)))
	io.CopyN(h, rand.Reader, 10)
	return fmt.Sprintf("%x", h.Sum(nil))[:20]
}
