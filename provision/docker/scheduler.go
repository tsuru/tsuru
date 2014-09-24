// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strings"
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/tsuru-base"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"gopkg.in/mgo.v2/bson"
)

// errNoFallback is the error returned when no fallback hosts are configured in
// the segregated scheduler.
var errNoFallback = errors.New("No fallback configured in the scheduler: you should have a pool without any teams")

const schedulerCollection = "docker_scheduler"

type Pool struct {
	Name  string `bson:"_id"`
	Teams []string
}

type segregatedScheduler struct{}

func (s segregatedScheduler) Schedule(c *cluster.Cluster, opts docker.CreateContainerOptions, schedulerOpts cluster.SchedulerOptions) (cluster.Node, error) {
	appName, _ := schedulerOpts.(string)
	a, _ := app.GetByName(appName)
	nodes, err := nodesForApp(c, a)
	if err != nil {
		return cluster.Node{}, err
	}
	node, err := s.chooseNode(nodes, opts.Name, appName)
	if err != nil {
		return cluster.Node{}, err
	}
	return cluster.Node{Address: node}, nil
}

type nodeAggregate struct {
	HostAddr string `bson:"_id"`
	Count    int
}

var hostMutex sync.Mutex

// aggregateNodesBy aggregates and counts how many containers
// exist each node that matches received filters
func aggregateNodesBy(matcher bson.M) (map[string]int, error) {
	coll := collection()
	defer coll.Close()
	pipe := coll.Pipe([]bson.M{
		matcher,
		{"$group": bson.M{"_id": "$hostaddr", "count": bson.M{"$sum": 1}}},
	})
	var results []nodeAggregate
	err := pipe.All(&results)
	if err != nil {
		return nil, err
	}
	countMap := make(map[string]int)
	for _, result := range results {
		countMap[result.HostAddr] = result.Count
	}
	return countMap, nil
}

func aggregateNodesByHost(hosts []string) (map[string]int, error) {
	return aggregateNodesBy(bson.M{"$match": bson.M{"hostaddr": bson.M{"$in": hosts}}})
}

func aggregateNodesByHostApp(hosts []string, appName string) (map[string]int, error) {
	return aggregateNodesBy(bson.M{"$match": bson.M{"appname": appName, "hostaddr": bson.M{"$in": hosts}}})
}

// chooseNode finds which is the node with the minimum number
// of containers and returns it
func (segregatedScheduler) chooseNode(nodes []cluster.Node, contName string, appName string) (string, error) {
	var chosenNode string
	hosts := make([]string, len(nodes))
	hostsMap := make(map[string]string)
	// Only hostname is saved in the docker containers collection
	// so we need to extract and map then to the original node.
	for i, node := range nodes {
		host := urlToHost(node.Address)
		hosts[i] = host
		hostsMap[host] = node.Address
	}
	log.Debugf("[scheduler] Possible nodes for container %s: %#v", contName, hosts)
	hostMutex.Lock()
	defer hostMutex.Unlock()
	hostCountMap, err := aggregateNodesByHost(hosts)
	if err != nil {
		return chosenNode, err
	}
	appCountMap, err := aggregateNodesByHostApp(hosts, appName)
	if err != nil {
		return chosenNode, err
	}
	// Finally finding the host with the minimum value for
	// the pair [appCount, hostCount]
	var minHost string
	minCount := math.MaxInt32
	for _, host := range hosts {
		adjCount := appCountMap[host]*10000 + hostCountMap[host]
		if adjCount < minCount {
			minCount = adjCount
			minHost = host
		}
	}
	chosenNode = hostsMap[minHost]
	log.Debugf("[scheduler] Chosen node for container %s: %#v Count: %d", contName, chosenNode, minCount)
	if contName != "" {
		coll := collection()
		defer coll.Close()
		err = coll.Update(bson.M{"name": contName}, bson.M{"$set": bson.M{"hostaddr": minHost}})
	}
	return chosenNode, err
}

func poolsForApp(app *app.App) ([]Pool, error) {
	var pools []Pool
	var query bson.M
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if app != nil {
		if app.TeamOwner != "" {
			query = bson.M{"teams": app.TeamOwner}
		} else {
			query = bson.M{"teams": bson.M{"$in": app.Teams}}
		}
		conn.Collection(schedulerCollection).Find(query).All(&pools)
	}
	if len(pools) == 0 {
		query = bson.M{"$or": []bson.M{{"teams": bson.M{"$exists": false}}, {"teams": bson.M{"$size": 0}}}}
		err = conn.Collection(schedulerCollection).Find(query).All(&pools)
		if err != nil {
			return nil, err
		}
	}
	if len(pools) == 0 {
		return nil, errNoFallback
	}
	return pools, nil
}

func nodesForApp(c *cluster.Cluster, app *app.App) ([]cluster.Node, error) {
	pools, err := poolsForApp(app)
	if err != nil {
		return nil, err
	}
	for _, pool := range pools {
		nodes, err := c.NodesForMetadata(map[string]string{"pool": pool.Name})
		if err != nil {
			return nil, err
		}
		if len(nodes) > 0 {
			return nodes, nil
		}
	}
	return nil, fmt.Errorf("No nodes found in pools %v", pools)
}

func nodesForAppName(c *cluster.Cluster, appName string) ([]cluster.Node, error) {
	a, err := app.GetByName(appName)
	if err != nil {
		return nil, err
	}
	return nodesForApp(c, a)
}

func (segregatedScheduler) addPool(poolName string) error {
	if poolName == "" {
		return errors.New("Pool name is required.")
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	pool := Pool{Name: poolName}
	return conn.Collection(schedulerCollection).Insert(pool)
}

func (segregatedScheduler) removePool(poolName string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Collection(schedulerCollection).Remove(bson.M{"_id": poolName})
}

func (segregatedScheduler) addTeamsToPool(poolName string, teams []string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var pool Pool
	err = conn.Collection(schedulerCollection).Find(bson.M{"_id": poolName}).One(&pool)
	if err != nil {
		return err
	}
	for _, newTeam := range teams {
		for _, team := range pool.Teams {
			if newTeam == team {
				return errors.New("Team already exists in pool.")
			}
		}
	}
	return conn.Collection(schedulerCollection).UpdateId(poolName, bson.M{"$push": bson.M{"teams": bson.M{"$each": teams}}})
}

func (segregatedScheduler) removeTeamsFromPool(poolName string, teams []string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Collection(schedulerCollection).UpdateId(poolName, bson.M{"$pullAll": bson.M{"teams": teams}})
}

type addPoolToSchedulerCmd struct{}

func (addPoolToSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "docker-pool-add",
		Usage:   "docker-pool-add <pool>",
		Desc:    "Add a pool to cluster",
		MinArgs: 1,
	}
}

func (addPoolToSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	b, err := json.Marshal(map[string]string{"pool": ctx.Args[0]})
	if err != nil {
		return err
	}
	url, err := cmd.GetURL("/docker/pool")
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	_, err = client.Do(req)
	if err != nil {
		return err
	}
	ctx.Stdout.Write([]byte("Pool successfully registered.\n"))
	return nil
}

type removePoolFromSchedulerCmd struct {
	tsuru.ConfirmationCommand
}

func (c *removePoolFromSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "docker-pool-remove",
		Usage:   "docker-pool-remove <pool> [-y]",
		Desc:    "Remove a pool to cluster",
		MinArgs: 1,
	}
}

func (c *removePoolFromSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	if !c.Confirm(ctx, fmt.Sprintf("Are you sure you want to remove \"%s\" pool?", ctx.Args[0])) {
		return nil
	}
	b, err := json.Marshal(map[string]string{"pool": ctx.Args[0]})
	if err != nil {
		return err
	}
	url, err := cmd.GetURL("/docker/pool")
	if err != nil {
		return err
	}
	req, err := http.NewRequest("DELETE", url, bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	_, err = client.Do(req)
	if err != nil {
		return err
	}
	ctx.Stdout.Write([]byte("Pool successfully removed.\n"))
	return nil
}

type listPoolsInTheSchedulerCmd struct{}

func (listPoolsInTheSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-pool-list",
		Usage: "docker-pool-list",
		Desc:  "List available pools in the cluster",
	}
}

func (listPoolsInTheSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	t := cmd.Table{Headers: cmd.Row([]string{"Pools", "Teams"})}
	url, err := cmd.GetURL("/docker/pool")
	if err != nil {
		return err
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	var pools []Pool
	err = json.Unmarshal(body, &pools)
	for _, p := range pools {
		t.AddRow(cmd.Row([]string{p.Name, strings.Join(p.Teams, ", ")}))
	}
	t.Sort()
	ctx.Stdout.Write(t.Bytes())
	return nil
}

type addTeamsToPoolCmd struct{}

func (addTeamsToPoolCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "docker-pool-teams-add",
		Usage:   "docker-pool-teams-add <pool> <teams>",
		Desc:    "Add team to a pool",
		MinArgs: 2,
	}
}

func (addTeamsToPoolCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	body, err := json.Marshal(map[string]interface{}{"pool": ctx.Args[0], "teams": ctx.Args[1:]})
	if err != nil {
		return err
	}
	url, err := cmd.GetURL("/docker/pool/team")
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	_, err = client.Do(req)
	if err != nil {
		return err
	}
	ctx.Stdout.Write([]byte("Teams successfully registered.\n"))
	return nil
}

type removeTeamsFromPoolCmd struct{}

func (removeTeamsFromPoolCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "docker-pool-teams-remove",
		Usage:   "docker-pool-teams-remove <pool> <teams>",
		Desc:    "Remove team from pool",
		MinArgs: 2,
	}
}

func (removeTeamsFromPoolCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	body, err := json.Marshal(map[string]interface{}{"pool": ctx.Args[0], "teams": ctx.Args[1:]})
	if err != nil {
		return err
	}
	url, err := cmd.GetURL("/docker/pool/team")
	if err != nil {
		return err
	}
	req, err := http.NewRequest("DELETE", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	_, err = client.Do(req)
	if err != nil {
		return err
	}
	ctx.Stdout.Write([]byte("Teams successfully removed.\n"))
	return nil
}
