// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"errors"
	"math/rand"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
)

var errNoSwarmNode = errors.New("no swarm nodes available")

type NodeAddr struct {
	DockerAddress string `bson:"_id"`
	SwarmAddress  string
}

func chooseDBSwarmNode() (*docker.Client, *NodeAddr, error) {
	coll, err := nodeAddrCollection()
	if err != nil {
		return nil, nil, err
	}
	var addrs []NodeAddr
	err = coll.Find(nil).All(&addrs)
	if err != nil {
		return nil, nil, err
	}
	if len(addrs) == 0 {
		return nil, nil, errNoSwarmNode
	}
	addr := addrs[rand.Intn(len(addrs))]
	// TODO(cezarsa): try ping. in case of failure, try another node and update
	// swarm node collection
	client, err := newClient(addr.DockerAddress)
	if err != nil {
		return nil, nil, err
	}
	return client, &addr, nil
}

func updateDBSwarmNodes(client *docker.Client) error {
	nodes, err := client.ListNodes(docker.ListNodesOptions{})
	if err != nil {
		return err
	}
	var docs []interface{}
	for _, n := range nodes {
		if n.ManagerStatus == nil {
			continue
		}
		addr := n.Spec.Annotations.Labels[labelDockerAddr]
		if addr == "" {
			continue
		}
		docs = append(docs, NodeAddr{
			DockerAddress: addr,
			SwarmAddress:  n.ManagerStatus.Addr,
		})
	}
	coll, err := nodeAddrCollection()
	if err != nil {
		return err
	}
	// TODO(cezarsa): safety and performance, do diff update instead of remove
	// all and add all.
	_, err = coll.RemoveAll(nil)
	if err != nil {
		return err
	}
	return coll.Insert(docs...)
}

func nodeAddrCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	return conn.Collection("swarmnodes"), nil
}
