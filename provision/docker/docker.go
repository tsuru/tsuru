// Copyright 2013 tsuru authors. All rights reserved.
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
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/docker-cluster/storage/mongodb"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/clusterclient"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/dockercommon"
)

func buildClusterStorage() (cluster.Storage, error) {
	mongoURL, _ := config.GetString("docker:cluster:mongo-url")
	mongoDatabase, _ := config.GetString("docker:cluster:mongo-database")
	if mongoURL == "" || mongoDatabase == "" {
		return nil, errors.Errorf("Cluster Storage: docker:cluster:{mongo-url,mongo-database} must be set.")
	}
	storage, err := mongodb.Mongodb(mongoURL, mongoDatabase)
	if err != nil {
		return nil, errors.Errorf("Cluster Storage: Unable to connect to mongodb: %s (docker:cluster:mongo-url = %q; docker:cluster:mongo-database = %q)",
			err.Error(), mongoURL, mongoDatabase)
	}
	return storage, nil
}

func randomString() string {
	h := crypto.MD5.New()
	h.Write([]byte(time.Now().Format(time.RFC3339Nano)))
	io.CopyN(h, rand.Reader, 10)
	return fmt.Sprintf("%x", h.Sum(nil))[:20]
}

func (p *dockerProvisioner) deployPipeline(app provision.App, imageID string, commands []string, evt *event.Event) (string, error) {
	actions := []*action.Action{
		&insertEmptyContainerInDB,
		&createContainer,
		&setContainerID,
		&startContainer,
		&updateContainerInDB,
		&followLogsAndCommit,
	}
	pipeline := action.NewPipeline(actions...)
	deployImage, err := image.AppNewImageName(app.GetName())
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
		isDeploy:      true,
		buildingImage: deployImage,
		provisioner:   p,
		event:         evt,
	}
	err = container.RunPipelineWithRetry(pipeline, args)
	if err != nil {
		log.Errorf("error on execute deploy pipeline for app %s - %s", app.GetName(), err)
		return "", err
	}
	return deployImage, nil
}

func (p *dockerProvisioner) start(oldContainer *container.Container, app provision.App, imageID string, w io.Writer, exposedPort string, destinationHosts ...string) (*container.Container, error) {
	commands, processName, err := dockercommon.LeanContainerCmds(oldContainer.ProcessName, imageID, app)
	if err != nil {
		return nil, err
	}
	var actions []*action.Action
	if oldContainer != nil && oldContainer.Status == provision.StatusStopped.String() {
		actions = []*action.Action{
			&insertEmptyContainerInDB,
			&createContainer,
			&setContainerID,
			&stopContainer,
			&updateContainerInDB,
			&setNetworkInfo,
		}
	} else {
		actions = []*action.Action{
			&insertEmptyContainerInDB,
			&createContainer,
			&setContainerID,
			&startContainer,
			&updateContainerInDB,
			&setNetworkInfo,
		}
	}
	pipeline := action.NewPipeline(actions...)
	args := runContainerActionsArgs{
		app:              app,
		processName:      processName,
		imageID:          imageID,
		commands:         commands,
		destinationHosts: destinationHosts,
		provisioner:      p,
		exposedPort:      exposedPort,
	}
	err = container.RunPipelineWithRetry(pipeline, args)
	if err != nil {
		return nil, err
	}
	c := pipeline.Result().(*container.Container)
	err = c.SetImage(p.ClusterClient(), imageID)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (p *dockerProvisioner) ClusterClient() provision.BuilderDockerClient {
	return &clusterclient.ClusterClient{
		Cluster:    p.Cluster(),
		Collection: p.Collection,
		Limiter:    p.ActionLimiter(),
	}
}

func (p *dockerProvisioner) GetClient(app provision.App) (provision.BuilderDockerClient, error) {
	cli := &clusterclient.ClusterClient{
		Cluster:    p.Cluster(),
		Collection: p.Collection,
		Limiter:    p.ActionLimiter(),
	}
	if app != nil {
		appNodes, err := p.Nodes(app)
		if err != nil {
			return nil, err
		}
		for _, n := range appNodes {
			cli.PossibleNodes = append(cli.PossibleNodes, n.Address)
		}
	}
	return cli, nil
}
