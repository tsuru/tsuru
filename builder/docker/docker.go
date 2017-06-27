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

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
)

func (b *dockerBuilder) buildPipeline(p provision.BuilderDeploy, client *docker.Client, app provision.App, imageID string, commands []string, evt *event.Event) (string, error) {
	actions := []*action.Action{
		&createContainer,
		&commitContainer,
		&updateAppBuilderImage,
	}
	pipeline := action.NewPipeline(actions...)
	buildingImage, err := image.AppNewBuilderImageName(app.GetName())
	if err != nil {
		return "", log.WrapError(errors.Errorf("error getting new image name for app %s", app.GetName()))
	}
	var writer io.Writer = evt
	if evt == nil {
		writer = ioutil.Discard
	}
	args := runContainerActionsArgs{
		app:           app,
		imageID:       imageID,
		commands:      commands,
		writer:        writer,
		buildingImage: buildingImage,
		client:        client,
		event:         evt,
		provisioner:   p,
	}
	err = pipeline.Execute(args)
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
