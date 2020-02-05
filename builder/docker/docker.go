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
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
)

const (
	archiveDirPath  = "/home/application"
	archiveFileName = "archive.tar.gz"
)

func (b *dockerBuilder) buildPipeline(p provision.BuilderDeployDockerClient, client provision.BuilderDockerClient, app provision.App, tarFile io.Reader, evt *event.Event, opts *builder.BuildOpts) (appTypes.AppVersion, error) {
	actions := []*action.Action{
		&createContainer,
		&uploadToContainer,
		&startContainer,
		&followLogsAndCommit,
		&updateAppBuilderImage,
	}
	pipeline := action.NewPipeline(actions...)
	imageName, err := image.GetBuildImage(app)
	if err != nil {
		return nil, log.WrapError(errors.Errorf("error getting base image name for app %s", app.GetName()))
	}
	newVersion, err := servicemanager.AppVersion.NewAppVersion(appTypes.NewVersionArgs{
		App:            app,
		EventID:        evt.UniqueID.Hex(),
		CustomBuildTag: opts.Tag,
		Description:    opts.Message,
	})
	if err != nil {
		return nil, err
	}
	archiveFileURI := fmt.Sprintf("file://%s/%s", archiveDirPath, archiveFileName)
	cmds := dockercommon.ArchiveBuildCmds(app, archiveFileURI)
	var writer io.Writer = evt
	if evt == nil {
		writer = ioutil.Discard
	}
	args := runContainerActionsArgs{
		app:         app,
		imageID:     imageName,
		commands:    cmds,
		writer:      writer,
		version:     newVersion,
		client:      client,
		event:       evt,
		provisioner: p,
		tarFile:     tarFile,
		isDeploy:    true,
	}
	err = container.RunPipelineWithRetry(pipeline, args)
	if err != nil {
		log.Errorf("error on execute build pipeline for app %s - %s", app.GetName(), err)
		return nil, err
	}
	return newVersion, nil
}

func randomString() string {
	h := crypto.MD5.New()
	h.Write([]byte(time.Now().Format(time.RFC3339Nano)))
	io.CopyN(h, rand.Reader, 10)
	return fmt.Sprintf("%x", h.Sum(nil))[:20]
}
