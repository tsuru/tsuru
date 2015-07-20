// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/log"
	"gopkg.in/mgo.v2/bson"
)

// errNoDefaultPool is the error returned when no default hosts are configured in
// the segregated scheduler.
var errNoDefaultPool = errors.New("No default pool configured in the scheduler: you should create a default pool.")

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
	schedOpts, _ := schedulerOpts.([]string)
	if len(schedOpts) != 2 {
		return cluster.Node{}, fmt.Errorf("invalid scheduler opts: %#v", schedulerOpts)
	}
	appName := schedOpts[0]
	processName := schedOpts[1]
	a, _ := app.GetByName(appName)
	nodes, err := s.provisioner.Nodes(a)
	if err != nil {
		return cluster.Node{}, err
	}
	nodes, err = s.filterByMemoryUsage(a, nodes, s.maxMemoryRatio, s.totalMemoryMetadata)
	if err != nil {
		return cluster.Node{}, err
	}
	node, err := s.chooseNode(nodes, opts.Name, appName, processName)
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
	containers, err := s.provisioner.listContainersBy(bson.M{"hostaddr": bson.M{"$in": hosts}, "id": bson.M{"$nin": s.ignoredContainers}})
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
		log.Errorf("WARNING: no nodes found with enough memory for container of %q: %0.4fMB. Will ignore memory restrictions.",
			a.Name, float64(a.Plan.Memory)/megabyte)
		return nodes, nil
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

func (s *segregatedScheduler) aggregateContainersByHostAppProcess(hosts []string, appName, process string) (map[string]int, error) {
	matcher := bson.M{
		"appname":  appName,
		"hostaddr": bson.M{"$in": hosts},
		"id":       bson.M{"$nin": s.ignoredContainers},
	}
	if process == "" {
		matcher["$or"] = []bson.M{{"processname": bson.M{"$exists": false}}, {"processname": ""}}
	} else {
		matcher["processname"] = process
	}
	return s.aggregateContainersBy(bson.M{"$match": matcher})
}

func (s *segregatedScheduler) GetRemovableContainer(appName string, process string) (string, error) {
	a, _ := app.GetByName(appName)
	nodes, err := s.provisioner.Nodes(a)
	if err != nil {
		return "", err
	}
	return s.chooseContainerFromMaxContainersCountInNode(nodes, appName, process)
}

// chooseNodeWithMaxContainersCount finds which is the node with maximum number
// of containers and returns it
func (s *segregatedScheduler) chooseContainerFromMaxContainersCountInNode(nodes []cluster.Node, appName, process string) (string, error) {
	hosts, hostsMap := s.nodesToHosts(nodes)
	log.Debugf("[scheduler] Possible nodes for remove a container: %#v", hosts)
	s.hostMutex.Lock()
	defer s.hostMutex.Unlock()
	hostCountMap, err := s.aggregateContainersByHost(hosts)
	if err != nil {
		return "", err
	}
	appCountMap, err := s.aggregateContainersByHostAppProcess(hosts, appName, process)
	if err != nil {
		return "", err
	}
	// Finally finding the host with the maximum value for
	// the pair [appCount, hostCount]
	var maxHost string
	maxCount := 0
	for _, host := range hosts {
		adjCount := appCountMap[host]*10000 + hostCountMap[host]
		if adjCount > maxCount {
			maxCount = adjCount
			maxHost = host
		}
	}
	chosenNode := hostsMap[maxHost]
	log.Debugf("[scheduler] Chosen node for remove a container: %#v Count: %d", chosenNode, hostCountMap[maxHost])
	containerID, err := s.getContainerFromHost(maxHost, appName, process)
	if err != nil {
		return "", err
	}
	return containerID, err
}

func (s *segregatedScheduler) getContainerFromHost(host string, appName, process string) (string, error) {
	coll := s.provisioner.collection()
	defer coll.Close()
	var c container
	query := bson.M{
		"hostaddr": host,
		"appname":  appName,
		"id":       bson.M{"$nin": s.ignoredContainers},
	}
	if process == "" {
		query["$or"] = []bson.M{{"processname": bson.M{"$exists": false}}, {"processname": ""}}
	} else {
		query["processname"] = process
	}
	err := coll.Find(query).Select(bson.M{"id": 1}).One(&c)
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
func (s *segregatedScheduler) chooseNode(nodes []cluster.Node, contName string, appName, process string) (string, error) {
	var chosenNode string
	hosts, hostsMap := s.nodesToHosts(nodes)
	log.Debugf("[scheduler] Possible nodes for container %s: %#v", contName, hosts)
	s.hostMutex.Lock()
	defer s.hostMutex.Unlock()
	hostCountMap, err := s.aggregateContainersByHost(hosts)
	if err != nil {
		return chosenNode, err
	}
	appCountMap, err := s.aggregateContainersByHostAppProcess(hosts, appName, process)
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
