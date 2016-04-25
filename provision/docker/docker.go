// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"crypto"
	"crypto/rand"
	"fmt"
	"io"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/docker-cluster/storage/mongodb"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/safe"
)

func buildClusterStorage() (cluster.Storage, error) {
	mongoUrl, _ := config.GetString("docker:cluster:mongo-url")
	mongoDatabase, _ := config.GetString("docker:cluster:mongo-database")
	if mongoUrl == "" || mongoDatabase == "" {
		return nil, fmt.Errorf("Cluster Storage: docker:cluster:{mongo-url,mongo-database} must be set.")
	}
	storage, err := mongodb.Mongodb(mongoUrl, mongoDatabase)
	if err != nil {
		return nil, fmt.Errorf("Cluster Storage: Unable to connect to mongodb: %s (docker:cluster:mongo-url = %q; docker:cluster:mongo-database = %q)",
			err.Error(), mongoUrl, mongoDatabase)
	}
	return storage, nil
}

func (p *dockerProvisioner) hostToNodeAddress(host string) (string, error) {
	node, err := p.getNodeByHost(host)
	if err != nil {
		return "", err
	}
	return node.Address, nil
}

func (p *dockerProvisioner) getNodeByHost(host string) (cluster.Node, error) {
	nodes, err := p.Cluster().Nodes()
	if err != nil {
		return cluster.Node{}, err
	}
	for _, node := range nodes {
		if net.URLToHost(node.Address) == host {
			return node, nil
		}
	}
	return cluster.Node{}, fmt.Errorf("Host `%s` not found", host)
}

func randomString() string {
	h := crypto.MD5.New()
	h.Write([]byte(time.Now().Format(time.RFC3339Nano)))
	io.CopyN(h, rand.Reader, 10)
	return fmt.Sprintf("%x", h.Sum(nil))[:20]
}

func (p *dockerProvisioner) archiveDeploy(app provision.App, image, archiveURL string, w io.Writer) (string, error) {
	commands, err := archiveDeployCmds(app, archiveURL)
	if err != nil {
		return "", err
	}
	return p.deployPipeline(app, image, commands, w)
}

func (p *dockerProvisioner) deployPipeline(app provision.App, imageId string, commands []string, w io.Writer) (string, error) {
	actions := []*action.Action{
		&insertEmptyContainerInDB,
		&createContainer,
		&setContainerID,
		&startContainer,
		&updateContainerInDB,
		&followLogsAndCommit,
	}
	pipeline := action.NewPipeline(actions...)
	buildingImage, err := appNewImageName(app.GetName())
	if err != nil {
		return "", log.WrapError(fmt.Errorf("error getting new image name for app %s", app.GetName()))
	}
	args := runContainerActionsArgs{
		app:           app,
		imageID:       imageId,
		commands:      commands,
		writer:        w,
		isDeploy:      true,
		buildingImage: buildingImage,
		provisioner:   p,
	}
	err = pipeline.Execute(args)
	if err != nil {
		log.Errorf("error on execute deploy pipeline for app %s - %s", app.GetName(), err)
		return "", err
	}
	return buildingImage, nil
}

func (p *dockerProvisioner) start(oldContainer *container.Container, app provision.App, imageId string, w io.Writer, destinationHosts ...string) (*container.Container, error) {
	commands, processName, err := runLeanContainerCmds(oldContainer.ProcessName, imageId, app)
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
		imageID:          imageId,
		commands:         commands,
		destinationHosts: destinationHosts,
		provisioner:      p,
	}
	err = pipeline.Execute(args)
	if err != nil {
		return nil, err
	}
	c := pipeline.Result().(container.Container)
	err = c.SetImage(p, imageId)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// PushImage sends the given image to the registry server defined in the
// configuration file.
func (p *dockerProvisioner) PushImage(name, tag string) error {
	if _, err := config.GetString("docker:registry"); err == nil {
		var buf safe.Buffer
		pushOpts := docker.PushImageOptions{Name: name, Tag: tag, OutputStream: &buf, InactivityTimeout: net.StreamInactivityTimeout}
		err = p.Cluster().PushImage(pushOpts, p.RegistryAuthConfig())
		if err != nil {
			log.Errorf("[docker] Failed to push image %q (%s): %s", name, err, buf.String())
			return err
		}
	}
	return nil
}

func (p *dockerProvisioner) RegistryAuthConfig() docker.AuthConfiguration {
	var authConfig docker.AuthConfiguration
	authConfig.Email, _ = config.GetString("docker:registry-auth:email")
	authConfig.Username, _ = config.GetString("docker:registry-auth:username")
	authConfig.Password, _ = config.GetString("docker:registry-auth:password")
	authConfig.ServerAddress, _ = config.GetString("docker:registry")
	return authConfig
}
