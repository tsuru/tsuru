// Copyright 2015 tsuru authors. All rights reserved.
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
	"strconv"
	"strings"
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2/bson"
)

// errNoFallback is the error returned when no fallback hosts are configured in
// the segregated scheduler.
var errNoFallback = errors.New("No fallback configured in the scheduler: you should have a pool without any teams")

type segregatedScheduler struct {
	hostMutex           sync.Mutex
	maxMemoryRatio      float32
	totalMemoryMetadata string
	provisioner         *dockerProvisioner
	// ignored containers is only set in provisioner returned by
	// cloneProvisioner which will set this field to exclude some container
	// ids from balancing (containers being removed by rebalance usually).
	ignoredContainers []string
}

func (s *segregatedScheduler) Schedule(c *cluster.Cluster, opts docker.CreateContainerOptions, schedulerOpts cluster.SchedulerOptions) (cluster.Node, error) {
	appName, _ := schedulerOpts.(string)
	a, _ := app.GetByName(appName)
	nodes, err := s.provisioner.Nodes(a)
	if err != nil {
		return cluster.Node{}, err
	}
	nodes, err = s.filterByMemoryUsage(a, nodes, s.maxMemoryRatio, s.totalMemoryMetadata)
	if err != nil {
		return cluster.Node{}, err
	}
	node, err := s.chooseNode(nodes, opts.Name, appName)
	if err != nil {
		return cluster.Node{}, err
	}
	return cluster.Node{Address: node}, nil
}

func (s *segregatedScheduler) filterByMemoryUsage(a *app.App, nodes []cluster.Node, maxMemoryRatio float32, totalMemoryMetadata string) ([]cluster.Node, error) {
	if maxMemoryRatio == 0 || totalMemoryMetadata == "" {
		return nodes, nil
	}
	hosts := make([]string, len(nodes))
	for i := range nodes {
		hosts[i] = urlToHost(nodes[i].Address)
	}
	containers, err := s.provisioner.listContainersBy(bson.M{"hostaddr": bson.M{"$in": hosts}})
	if err != nil {
		return nil, err
	}
	hostReserved := make(map[string]int64)
	for _, cont := range containers {
		a, err := app.GetByName(cont.AppName)
		if err != nil {
			return nil, err
		}
		hostReserved[cont.HostAddr] += a.Plan.Memory
	}
	megabyte := float64(1024 * 1024)
	nodeList := make([]cluster.Node, 0, len(nodes))
	for _, node := range nodes {
		totalMemory, _ := strconv.ParseFloat(node.Metadata[totalMemoryMetadata], 64)
		shouldAdd := true
		if totalMemory != 0 {
			maxMemory := totalMemory * float64(maxMemoryRatio)
			host := urlToHost(node.Address)
			nodeReserved := hostReserved[host] + a.Plan.Memory
			if nodeReserved > int64(maxMemory) {
				shouldAdd = false
				tryingToReserveMB := float64(a.Plan.Memory) / megabyte
				reservedMB := float64(hostReserved[host]) / megabyte
				limitMB := maxMemory / megabyte
				log.Errorf("Node %q has reached its memory limit. "+
					"Limit %0.4fMB. Reserved: %0.4fMB. Needed additional %0.4fMB",
					host, limitMB, reservedMB, tryingToReserveMB)
			}
		}
		if shouldAdd {
			nodeList = append(nodeList, node)
		}
	}
	if len(nodeList) == 0 {
		return nil, fmt.Errorf("No nodes found with enough memory for container of %q: %0.4fMB.",
			a.Name, float64(a.Plan.Memory)/megabyte)
	}
	return nodeList, nil
}

type nodeAggregate struct {
	HostAddr string `bson:"_id"`
	Count    int
}

// aggregateContainersBy aggregates and counts how many containers
// exist each node that matches received filters
func (s *segregatedScheduler) aggregateContainersBy(matcher bson.M) (map[string]int, error) {
	coll := s.provisioner.collection()
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

func (s *segregatedScheduler) aggregateContainersByHost(hosts []string) (map[string]int, error) {
	return s.aggregateContainersBy(bson.M{"$match": bson.M{"hostaddr": bson.M{"$in": hosts}, "id": bson.M{"$nin": s.ignoredContainers}}})
}

func (s *segregatedScheduler) aggregateContainersByHostApp(hosts []string, appName string) (map[string]int, error) {
	return s.aggregateContainersBy(bson.M{"$match": bson.M{"appname": appName, "hostaddr": bson.M{"$in": hosts}, "id": bson.M{"$nin": s.ignoredContainers}}})
}

func (s *segregatedScheduler) GetRemovableContainer(appName string, c *cluster.Cluster) (string, error) {
	a, _ := app.GetByName(appName)
	nodes, err := s.provisioner.Nodes(a)
	if err != nil {
		return "", err
	}
	return s.chooseContainerFromMaxContainersCountInNode(nodes, appName)
}

// chooseNodeWithMaxContainersCount finds which is the node with maximum number
// of containers and returns it
func (s *segregatedScheduler) chooseContainerFromMaxContainersCountInNode(nodes []cluster.Node, appName string) (string, error) {
	var chosenNode string
	hosts, hostsMap := s.nodesToHosts(nodes)
	log.Debugf("[scheduler] Possible nodes for remove a container: %#v", hosts)
	s.hostMutex.Lock()
	defer s.hostMutex.Unlock()
	hostCountMap, err := s.aggregateContainersByHost(hosts)
	if err != nil {
		return chosenNode, err
	}
	appCountMap, err := s.aggregateContainersByHostApp(hosts, appName)
	if err != nil {
		return chosenNode, err
	}
	// Finally finding the host with the maximum value for
	// the pair [appCount, hostCount]
	var maxHost string
	maxCount := 0
	for _, host := range hosts {
		adjCount := appCountMap[host] + hostCountMap[host]
		if adjCount > maxCount {
			maxCount = adjCount
			maxHost = host
		}
	}
	chosenNode = hostsMap[maxHost]
	log.Debugf("[scheduler] Chosen node for remove a container: %#v Count: %d", chosenNode, hostCountMap[maxHost])
	containerID, err := s.getContainerFromHost(maxHost, appName)
	if err != nil {
		return "", err
	}
	return containerID, err
}

func (s *segregatedScheduler) getContainerFromHost(host string, appName string) (string, error) {
	coll := s.provisioner.collection()
	defer coll.Close()
	var c container
	err := coll.Find(bson.M{"hostaddr": host, "appname": appName}).Select(bson.M{"id": 1}).One(&c)
	return c.ID, err
}

func (s *segregatedScheduler) nodesToHosts(nodes []cluster.Node) ([]string, map[string]string) {
	hosts := make([]string, len(nodes))
	hostsMap := make(map[string]string)
	// Only hostname is saved in the docker containers collection
	// so we need to extract and map then to the original node.
	for i, node := range nodes {
		host := urlToHost(node.Address)
		hosts[i] = host
		hostsMap[host] = node.Address
	}
	return hosts, hostsMap
}

// chooseNode finds which is the node with the minimum number
// of containers and returns it
func (s *segregatedScheduler) chooseNode(nodes []cluster.Node, contName string, appName string) (string, error) {
	var chosenNode string
	hosts, hostsMap := s.nodesToHosts(nodes)
	log.Debugf("[scheduler] Possible nodes for container %s: %#v", contName, hosts)
	s.hostMutex.Lock()
	defer s.hostMutex.Unlock()
	hostCountMap, err := s.aggregateContainersByHost(hosts)
	if err != nil {
		return chosenNode, err
	}
	appCountMap, err := s.aggregateContainersByHostApp(hosts, appName)
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
		coll := s.provisioner.collection()
		defer coll.Close()
		err = coll.Update(bson.M{"name": contName}, bson.M{"$set": bson.M{"hostaddr": minHost}})
	}
	return chosenNode, err
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
	cmd.ConfirmationCommand
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
	var pools []provision.Pool
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
