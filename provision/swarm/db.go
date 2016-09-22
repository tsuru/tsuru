// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"math/rand"

	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type notFoundError struct{ error }

func (e notFoundError) NotFound() bool {
	return true
}

var errNoSwarmNode = notFoundError{errors.New("no swarm nodes available")}

const (
	uniqueDocumentID    = "swarm"
	swarmCollectionName = "swarmnodes"
	nodeRetryCount      = 3
)

type NodeAddrs struct {
	UniqueID  string `bson:"_id"`
	Addresses []string
}

func chooseDBSwarmNode() (*docker.Client, error) {
	coll, err := nodeAddrCollection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	var addrs NodeAddrs
	err = coll.FindId(uniqueDocumentID).One(&addrs)
	if err != nil && err != mgo.ErrNotFound {
		return nil, errors.Wrap(err, "")
	}
	if len(addrs.Addresses) == 0 {
		return nil, errors.Wrap(errNoSwarmNode, "")
	}
	var client *docker.Client
	initialIdx := rand.Intn(len(addrs.Addresses))
	var i int
	for ; i < nodeRetryCount; i++ {
		idx := (initialIdx + i) % len(addrs.Addresses)
		addr := addrs.Addresses[idx]
		client, err = newClient(addr)
		if err != nil {
			return nil, err
		}
		err = client.Ping()
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, errors.Wrap(err, "")
	}
	if i > 0 {
		updateDBSwarmNodes(client)
	}
	return client, nil
}

func updateDBSwarmNodes(client *docker.Client) error {
	nodes, err := client.ListNodes(docker.ListNodesOptions{})
	if err != nil {
		return errors.Wrap(err, "")
	}
	var addrs []string
	for _, n := range nodes {
		if n.ManagerStatus == nil {
			continue
		}
		addr := n.Spec.Annotations.Labels[labelDockerAddr]
		if addr == "" {
			continue
		}
		addrs = append(addrs, addr)
	}
	coll, err := nodeAddrCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	_, err = coll.UpsertId(uniqueDocumentID, bson.M{"$set": bson.M{"addresses": addrs}})
	if err != nil {
		return errors.Wrap(err, "")
	}
	return nil
}

func nodeAddrCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, errors.Wrap(err, "")
	}
	return conn.Collection(swarmCollectionName), nil
}
