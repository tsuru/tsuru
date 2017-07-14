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

	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/docker-cluster/storage/mongodb"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/dockercommon"
	"github.com/tsuru/tsuru/safe"
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

func (p *dockerProvisioner) GetNodeByHost(host string) (cluster.Node, error) {
	nodes, err := p.Cluster().UnfilteredNodes()
	if err != nil {
		return cluster.Node{}, err
	}
	for _, node := range nodes {
		if net.URLToHost(node.Address) == host {
			return node, nil
		}
	}
	return cluster.Node{}, errors.Errorf("node with host %q not found", host)
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
	err = pipeline.Execute(args)
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
	err = pipeline.Execute(args)
	if err != nil {
		return nil, err
	}
	c := pipeline.Result().(container.Container)
	err = c.SetImage(p, imageID)
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
		pushOpts := docker.PushImageOptions{
			Name:              name,
			Tag:               tag,
			OutputStream:      &buf,
			InactivityTimeout: net.StreamInactivityTimeout,
			RawJSONStream:     true,
		}
		err = p.Cluster().PushImage(pushOpts, dockercommon.RegistryAuthConfig())
		if err != nil {
			log.Errorf("[docker] Failed to push image %q (%s): %s", name, err, buf.String())
			return err
		}
	}
	return nil
}

func (p *dockerProvisioner) GetDockerClient(app provision.App) (*docker.Client, error) {
	cluster := p.Cluster()
	nodeAddr := ""
	if app == nil {
		nodes, err := cluster.Nodes()
		if err != nil {
			return nil, err
		}
		if len(nodes) < 1 {
			return nil, errors.New("There is no Docker node. Add one with `tsuru node-add`")
		}
		nodeAddr, _, err = p.scheduler.minMaxNodes(nodes, "", "")
		if err != nil {
			return nil, err
		}
	} else {
		nodes, err := cluster.NodesForMetadata(map[string]string{"pool": app.GetPool()})
		if err != nil {
			return nil, err
		}
		nodeAddr, _, err = p.scheduler.minMaxNodes(nodes, app.GetName(), "")
		if err != nil {
			return nil, err
		}
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
