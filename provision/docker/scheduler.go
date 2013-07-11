// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"github.com/dotcloud/docker"
	dcli "github.com/fsouza/go-dockerclient"
	"github.com/globocom/docker-cluster/cluster"
	"github.com/globocom/tsuru/db"
)

const schedulerCollection = "docker_scheduler"

type node struct {
	ID      string `bson:"_id"`
	Address string
	Team    string
}

type segregatedScheduler struct{}

func (segregatedScheduler) Schedule(config *docker.Config) (string, *docker.Container, error) {
	return "", nil, nil
}

func (segregatedScheduler) Nodes() ([]cluster.Node, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var nodes []node
	err = conn.Collection(schedulerCollection).Find(nil).All(&nodes)
	if err != nil {
		return nil, err
	}
	result := make([]cluster.Node, len(nodes))
	for i, node := range nodes {
		result[i] = cluster.Node{ID: node.ID, Address: node.Address}
	}
	return result, nil
}
