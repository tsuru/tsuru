// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/log"
	"io/ioutil"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"math"
	"net/http"
	"strings"
	"sync"
)

// errNoFallback is the error returned when no fallback hosts are configured in
// the segregated scheduler.
var errNoFallback = errors.New("No fallback configured in the scheduler")

var (
	errNodeAlreadyRegister = errors.New("This node is already registered")
	errNodeNotFound        = errors.New("Node not found")
)

const schedulerCollection = "docker_scheduler"

type Pool struct {
	Name  string `bson:"_id"`
	Nodes []string
	Teams []string
}

type segregatedScheduler struct{}

func (s segregatedScheduler) Schedule(opts docker.CreateContainerOptions, schedulerOpts cluster.SchedulerOptions) (cluster.Node, error) {
	appName, _ := schedulerOpts.(string)
	a, _ := app.GetByName(appName)
	nodes, err := s.nodesForApp(a)
	if err != nil {
		return cluster.Node{}, err
	}
	node, err := s.chooseNode(nodes, opts.Name, appName)
	if err != nil {
		return cluster.Node{}, err
	}
	return cluster.Node{ID: node, Address: node}, nil
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
func (segregatedScheduler) chooseNode(nodes []string, contName string, appName string) (string, error) {
	var chosenNode string
	hosts := make([]string, len(nodes))
	hostsMap := make(map[string]string)
	// Only hostname is saved in the docker containers collection
	// so we need to extract and map then to the original node.
	for i, node := range nodes {
		host := urlToHost(node)
		hosts[i] = host
		hostsMap[host] = node
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
	coll := collection()
	defer coll.Close()
	err = coll.Update(bson.M{"name": contName}, bson.M{"$set": bson.M{"hostaddr": minHost}})
	return chosenNode, err
}

func (segregatedScheduler) nodesForApp(app *app.App) ([]string, error) {
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
		err = conn.Collection(schedulerCollection).Find(query).All(&pools)
		if err == nil {
			for _, pool := range pools {
				if len(pool.Nodes) > 0 {
					return pool.Nodes, nil
				}
			}
		}
	}
	query = bson.M{"$or": []bson.M{{"teams": bson.M{"$exists": false}}, {"teams": bson.M{"$size": 0}}}}
	err = conn.Collection(schedulerCollection).Find(query).All(&pools)
	if err != nil {
		return nil, errNoFallback
	}
	for _, pool := range pools {
		if len(pool.Nodes) > 0 {
			return pool.Nodes, nil
		}
	}
	return nil, errNoFallback
}

func (segregatedScheduler) Nodes() ([]cluster.Node, error) {
	nodes, err := listNodesInTheScheduler()
	if err != nil {
		return nil, err
	}
	result := make([]cluster.Node, len(nodes))
	for i, node := range nodes {
		result[i] = cluster.Node{ID: node, Address: node}
	}
	return result, nil
}

func (s segregatedScheduler) NodesForOptions(schedulerOpts cluster.SchedulerOptions) ([]cluster.Node, error) {
	appName, _ := schedulerOpts.(string)
	a, _ := app.GetByName(appName)
	nodes, err := s.nodesForApp(a)
	if err != nil {
		return nil, err
	}
	result := make([]cluster.Node, len(nodes))
	for i, node := range nodes {
		result[i] = cluster.Node{ID: node, Address: node}
	}
	return result, nil
}

func (segregatedScheduler) GetNode(pool, address string) (string, error) {
	conn, err := db.Conn()
	if err != nil {
		return "", err
	}
	defer conn.Close()
	var p Pool
	err = conn.Collection(schedulerCollection).FindId(pool).One(&p)
	if err != nil {
		return "", err
	}
	for _, n := range p.Nodes {
		if n == address {
			return n, nil
		}
	}
	return "", errNodeNotFound
}

// Register adds a new node to the scheduler, registering for use in
// the given team. The team parameter is optional, when set to "", the node
// will be used as a fallback node.
func (seg *segregatedScheduler) Register(params map[string]string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	nodes, _ := seg.Nodes()
	for _, node := range nodes {
		if params["address"] == node.Address || params["ID"] == node.ID {
			return errNodeAlreadyRegister
		}
	}
	if params["pool"] == "" {
		return errors.New("Pool name is required.")
	}
	err = conn.Collection(schedulerCollection).Update(bson.M{"_id": params["pool"]}, bson.M{"$push": bson.M{"nodes": params["address"]}})
	return err
}

// Unregister removes a node from the scheduler.
func (segregatedScheduler) Unregister(params map[string]string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	pools, err := conn.Collection(schedulerCollection).Find(bson.M{"nodes": params["address"]}).Count()
	if err != nil {
		return err
	}
	if pools == 0 {
		return errNodeNotFound
	}
	err = conn.Collection(schedulerCollection).UpdateId(params["pool"], bson.M{"$pull": bson.M{"nodes": params["address"]}})
	if err == mgo.ErrNotFound {
		return errNodeNotFound
	}
	return err
}

func (segregatedScheduler) addPool(pool_name string) error {
	if pool_name == "" {
		return errors.New("Pool name is required.")
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	pool := Pool{Name: pool_name}
	return conn.Collection(schedulerCollection).Insert(pool)
}

func (segregatedScheduler) removePool(pool_name string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Collection(schedulerCollection).Remove(bson.M{"_id": pool_name, "nodes": bson.M{"$size": 0}})
}

func (segregatedScheduler) addTeamsToPool(pool_name string, teams []string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Collection(schedulerCollection).UpdateId(pool_name, bson.M{"$push": bson.M{"teams": bson.M{"$each": teams}}})
}

func (segregatedScheduler) removeTeamsFromPool(pool_name string, teams []string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Collection(schedulerCollection).UpdateId(pool_name, bson.M{"$pullAll": bson.M{"teams": teams}})
}

func listNodesInTheScheduler() ([]string, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var pools []Pool
	err = conn.Collection(schedulerCollection).Find(nil).All(&pools)
	if err != nil {
		return nil, err
	}
	var nodes []string
	for _, pool := range pools {
		nodes = append(nodes, pool.Nodes...)
	}
	return nodes, nil
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

type removePoolFromSchedulerCmd struct{}

func (removePoolFromSchedulerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "docker-pool-remove",
		Usage:   "docker-pool-remove <pool>",
		Desc:    "Remove a pool to cluster",
		MinArgs: 1,
	}
}

func (removePoolFromSchedulerCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
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
	t := cmd.Table{Headers: cmd.Row([]string{"Pools", "Nodes", "Teams"})}
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
		t.AddRow(cmd.Row([]string{p.Name, strings.Join(p.Nodes, ", "), strings.Join(p.Teams, ", ")}))
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
