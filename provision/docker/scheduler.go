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
		return errNodeAlreadyRegister
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
	err := addNodeToScheduler(nd, team)
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
	nd := cluster.Node{ID: ctx.Args[0]}
	err := removeNodeFromScheduler(nd)
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
		t.AddRow(cmd.Row([]string{n.ID, n.Address, n.Team}))
	}
	t.Sort()
	ctx.Stdout.Write(t.Bytes())
	return nil
}
