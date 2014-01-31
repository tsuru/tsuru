// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"github.com/fsouza/go-dockerclient"
	"github.com/globocom/docker-cluster/cluster"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/cmd"
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
	errNodeAlreadyRegister = errors.New("This node is already registered")
	errNodeNotFound        = errors.New("Node not found")
)

const schedulerCollection = "docker_scheduler"

type node struct {
	ID      string `bson:"_id"`
	Address string
	Teams   []string
}

type segregatedScheduler struct{}

func (s segregatedScheduler) Schedule(opts docker.CreateContainerOptions) (string, *docker.Container, error) {
	conn, err := db.Conn()
	if err != nil {
		return "", nil, err
	}
	defer conn.Close()
	var cont container
	coll := collection()
	defer coll.Close()
	err = coll.Find(bson.M{"name": opts.Name}).One(&cont)
	if err != nil {
		return "", nil, err
	}
	app, err := app.GetByName(cont.AppName)
	if err != nil {
		return s.fallback(opts)
	}
	var nodes []node
	query := bson.M{"teams": bson.M{"$in": app.Teams}}
	err = conn.Collection(schedulerCollection).Find(query).All(&nodes)
	if err != nil || len(nodes) < 1 {
		return s.fallback(opts)
	}
	return s.handle(opts, nodes)
}

func (s segregatedScheduler) fallback(opts docker.CreateContainerOptions) (string, *docker.Container, error) {
	conn, err := db.Conn()
	if err != nil {
		return "", nil, err
	}
	defer conn.Close()
	var nodes []node
	err = conn.Collection(schedulerCollection).Find(bson.M{"$or": []bson.M{{"teams": bson.M{"$exists": false}}, {"teams": bson.M{"$size": 0}}}}).All(&nodes)
	if err != nil || len(nodes) < 1 {
		return "", nil, errNoFallback
	}
	return s.handle(opts, nodes)
}

func (segregatedScheduler) handle(opts docker.CreateContainerOptions, nodes []node) (string, *docker.Container, error) {
	node := nodes[rand.Intn(len(nodes))]
	client, err := docker.NewClient(node.Address)
	if err != nil {
		return node.ID, nil, err
	}
	container, err := client.CreateContainer(opts)
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

func (segregatedScheduler) GetNode(id string) (node, error) {
	conn, err := db.Conn()
	if err != nil {
		return node{}, err
	}
	defer conn.Close()
	var n node
	err = conn.Collection(schedulerCollection).FindId(id).One(&n)
	if err == mgo.ErrNotFound {
		return node{}, errNodeNotFound
	}
	return n, nil
}

// Register adds a new node to the scheduler, registering for use in
// the given team. The team parameter is optional, when set to "", the node
// will be used as a fallback node.
func (segregatedScheduler) Register(params map[string]string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	node := node{ID: params["ID"], Address: params["address"], Teams: []string{params["team"]}}
	err = conn.Collection(schedulerCollection).Insert(node)
	if mgo.IsDup(err) {
		return errNodeAlreadyRegister
	}
	return err
}

// Unregister removes a node from the scheduler.
func (segregatedScheduler) Unregister(params map[string]string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Collection(schedulerCollection).RemoveId(params["ID"])
	if err == mgo.ErrNotFound {
		return errNodeNotFound
	}
	return err
}

func listNodesInTheScheduler() ([]node, error) {
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
	return nodes, nil
}

type addNodeToSchedulerCmd struct{}

func (addNodeToSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "docker-add-node",
		Usage:   "docker-add-node <id> <address> [team]",
		Desc:    "Registers a new node in the cluster, optionally assigning it to a team",
		MinArgs: 2,
	}
}

func (addNodeToSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	var team string
	nd := cluster.Node{ID: ctx.Args[0], Address: ctx.Args[1]}
	if len(ctx.Args) > 2 {
		team = ctx.Args[2]
	}
	var scheduler segregatedScheduler
	err := scheduler.Register(map[string]string{"ID": nd.ID, "address": nd.Address, "team": team})
	if err != nil {
		return err
	}
	ctx.Stdout.Write([]byte("Node successfully registered.\n"))
	return nil
}

type removeNodeFromSchedulerCmd struct{}

func (removeNodeFromSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "docker-rm-node",
		Usage:   "docker-rm-node <id>",
		Desc:    "Removes a node from the cluster",
		MinArgs: 1,
	}
}

func (removeNodeFromSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	var scheduler segregatedScheduler
	err := scheduler.Unregister(map[string]string{"ID": ctx.Args[0]})
	if err != nil {
		return err
	}
	ctx.Stdout.Write([]byte("Node successfully removed.\n"))
	return nil
}

type listNodesInTheSchedulerCmd struct{}

func (listNodesInTheSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-list-nodes",
		Usage: "docker-list-nodes",
		Desc:  "List available nodes in the cluster",
	}
}

func (listNodesInTheSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	t := cmd.Table{Headers: cmd.Row([]string{"ID", "Address", "Team"})}
	nodes, err := listNodesInTheScheduler()
	if err != nil {
		return err
	}
	for _, n := range nodes {
		t.AddRow(cmd.Row([]string{n.ID, n.Address, strings.Join(n.Teams, ", ")}))
	}
	t.Sort()
	ctx.Stdout.Write(t.Bytes())
	return nil
}
