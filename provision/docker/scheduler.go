// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"github.com/dotcloud/docker"
	dcli "github.com/fsouza/go-dockerclient"
	"github.com/globocom/config"
	"github.com/globocom/docker-cluster/cluster"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"math/rand"
	"strings"
)

// errNoFallback is the error returned when no fallback hosts are configured in
// the segregated scheduler.
var errNoFallback = errors.New("No fallback configured in the scheduler")

var (
	ErrNodeAlreadyRegistered = errors.New("This node is already registered")
	ErrNodeNotFound          = errors.New("Node not found")
)

const schedulerCollection = "docker_scheduler"

type node struct {
	ID      string `bson:"_id"`
	Address string
	Team    string
}

type segregatedScheduler struct{}

func (s segregatedScheduler) Schedule(cfg *docker.Config) (string, *docker.Container, error) {
	image := cfg.Image
	namespace, err := config.GetString("docker:repository-namespace")
	if err != nil {
		return "", nil, err
	}
	conn, err := db.Conn()
	if err != nil {
		return "", nil, err
	}
	defer conn.Close()
	appname := strings.Replace(image, namespace+"/", "", -1)
	app := app.App{Name: appname}
	err = app.Get()
	if err != nil {
		return s.fallback(cfg)
	}
	if len(app.Teams) == 1 {
		var nodes []node
		err = conn.Collection(schedulerCollection).Find(bson.M{"team": app.Teams[0]}).All(&nodes)
		if err != nil || len(nodes) < 1 {
			return s.fallback(cfg)
		}
		return s.handle(cfg, nodes)
	}
	return s.fallback(cfg)
}

func (s segregatedScheduler) fallback(cfg *docker.Config) (string, *docker.Container, error) {
	conn, err := db.Conn()
	if err != nil {
		return "", nil, err
	}
	defer conn.Close()
	var nodes []node
	err = conn.Collection(schedulerCollection).Find(bson.M{"team": ""}).All(&nodes)
	if err != nil || len(nodes) < 1 {
		return "", nil, errNoFallback
	}
	return s.handle(cfg, nodes)
}

func (segregatedScheduler) handle(cfg *docker.Config, nodes []node) (string, *docker.Container, error) {
	node := nodes[rand.Intn(len(nodes))]
	client, err := dcli.NewClient(node.Address)
	if err != nil {
		return node.ID, nil, err
	}
	container, err := client.CreateContainer(cfg)
	return node.ID, container, err
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

// AddNodeToScheduler adds a new node to the scheduler, registering for use in
// the given team. The team parameter is optional, when set to "", the node
// will be used as a fallback node.
func addNodeToScheduler(n cluster.Node, team string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	node := node{ID: n.ID, Address: n.Address, Team: team}
	err = conn.Collection(schedulerCollection).Insert(node)
	if mgo.IsDup(err) {
		return ErrNodeAlreadyRegistered
	}
	return err
}

// RemoveNodeFromScheduler removes a node from the scheduler.
func removeNodeFromScheduler(n cluster.Node) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Collection(schedulerCollection).RemoveId(n.ID)
	if err != nil && err.Error() == "not found" {
		return ErrNodeNotFound
	}
	return err
}
