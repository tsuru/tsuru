// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/autoscale"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/node"
)

type segregatedScheduler struct {
	hostMutex           sync.Mutex
	maxMemoryRatio      float32
	TotalMemoryMetadata string
	provisioner         *dockerProvisioner
	// ignored containers is only set in provisioner returned by
	// cloneProvisioner which will set this field to exclude some container
	// ids from balancing (containers being removed by rebalance usually).
	ignoredContainers []string
}

func (s *segregatedScheduler) Schedule(c *cluster.Cluster, opts *docker.CreateContainerOptions, schedulerOpts cluster.SchedulerOptions) (cluster.Node, error) {
	schedOpts, ok := schedulerOpts.(*container.SchedulerOpts)
	if !ok {
		return cluster.Node{}, &container.SchedulerError{
			Base: errors.Errorf("invalid scheduler opts: %#v", schedulerOpts),
		}
	}
	var appName string
	filterNodesMap := map[string]struct{}{}
	if schedOpts != nil {
		appName = schedOpts.AppName
		for _, n := range schedOpts.FilterNodes {
			filterNodesMap[n] = struct{}{}
		}
	}
	if appName == "" {
		return s.scheduleAnyNode(c, filterNodesMap)
	}
	a, _ := app.GetByName(context.TODO(), schedOpts.AppName)
	nodes, err := s.provisioner.Nodes(a)
	if err != nil {
		return cluster.Node{}, &container.SchedulerError{Base: err}
	}
	nodes = filterNodes(nodes, filterNodesMap)
	nodes, err = s.filterByMemoryUsage(a, nodes, s.maxMemoryRatio, s.TotalMemoryMetadata)
	if err != nil {
		return cluster.Node{}, &container.SchedulerError{Base: err}
	}
	node, err := s.chooseNodeToAdd(nodes, opts.Name, schedOpts.AppName, schedOpts.ProcessName)
	if err != nil {
		return cluster.Node{}, &container.SchedulerError{Base: err}
	}
	if schedOpts.ActionLimiter != nil {
		schedOpts.LimiterDone = schedOpts.ActionLimiter.Start(net.URLToHost(node))
	}
	if schedOpts.UpdateName {
		err = s.updateContainerName(opts, schedOpts.AppName)
		if err != nil {
			return cluster.Node{}, &container.SchedulerError{Base: err}
		}
	}
	return cluster.Node{Address: node}, nil
}

func (s *segregatedScheduler) scheduleAnyNode(c *cluster.Cluster, filter map[string]struct{}) (cluster.Node, error) {
	nodes, err := c.Nodes()
	if err != nil {
		return cluster.Node{}, err
	}
	nodes = filterNodes(nodes, filter)
	if len(nodes) < 1 {
		return cluster.Node{}, errors.New("There is no Docker node. Add one with `tsuru node-add`")
	}
	log.Debugf("[scheduler] Schedule any node with filter %#v possible nodes: %#v", filter, nodes)
	nodeAddr, _, err := s.minMaxNodes(nodes, "", "")
	if err != nil {
		return cluster.Node{}, err
	}
	log.Debugf("[scheduler] Schedule any node with filter %#v chosen node: %q", filter, nodeAddr)
	return cluster.Node{Address: nodeAddr}, nil
}

func (s *segregatedScheduler) updateContainerName(opts *docker.CreateContainerOptions, appName string) error {
	if opts.Name == "" {
		return nil
	}
	newName := generateContainerName(appName)
	coll := s.provisioner.Collection()
	defer coll.Close()
	err := coll.Update(bson.M{"name": opts.Name}, bson.M{"$set": bson.M{"name": newName}})
	if err != nil {
		return err
	}
	opts.Name = newName
	return nil
}

func (s *segregatedScheduler) filterByMemoryUsage(a *app.App, nodes []cluster.Node, maxMemoryRatio float32, TotalMemoryMetadata string) ([]cluster.Node, error) {
	ctx := context.TODO()
	if maxMemoryRatio == 0 || TotalMemoryMetadata == "" {
		return nodes, nil
	}
	hosts := make([]string, len(nodes))
	for i := range nodes {
		hosts[i] = net.URLToHost(nodes[i].Address)
	}
	containers, err := s.provisioner.ListContainers(bson.M{"hostaddr": bson.M{"$in": hosts}, "id": bson.M{"$nin": s.ignoredContainers}})
	if err != nil {
		return nil, err
	}
	hostReserved := make(map[string]int64)
	for _, cont := range containers {
		contApp, err := app.GetByName(ctx, cont.AppName)
		if err != nil {
			return nil, err
		}
		hostReserved[cont.HostAddr] += contApp.Plan.Memory
	}
	megabyte := float64(1024 * 1024)
	nodeList := make([]cluster.Node, 0, len(nodes))
	for _, node := range nodes {
		totalMemory, _ := strconv.ParseFloat(node.Metadata[TotalMemoryMetadata], 64)
		shouldAdd := true
		if totalMemory != 0 {
			maxMemory := totalMemory * float64(maxMemoryRatio)
			host := net.URLToHost(node.Address)
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
		var autoScaleEnabled bool
		rule, _ := autoscale.AutoScaleRuleForMetadata(a.Pool)
		if rule != nil {
			autoScaleEnabled = rule.Enabled
		}
		errMsg := fmt.Sprintf("no nodes found with enough memory for container of %q: %0.4fMB",
			a.Name, float64(a.Plan.Memory)/megabyte)
		if autoScaleEnabled {
			// Allow going over quota temporarily because auto-scale will be
			// able to detect this and automatically add a new nodes.
			log.Errorf("WARNING: %s. Will ignore memory restrictions.", errMsg)
			return nodes, nil
		}
		return nil, errors.New(errMsg)
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
	coll := s.provisioner.Collection()
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
		"hostaddr": bson.M{"$in": hosts},
		"id":       bson.M{"$nin": s.ignoredContainers},
	}
	if appName != "" {
		matcher["appname"] = appName
	}
	if process == "" {
		matcher["$or"] = []bson.M{{"processname": bson.M{"$exists": false}}, {"processname": ""}}
	} else {
		matcher["processname"] = process
	}
	return s.aggregateContainersBy(bson.M{"$match": matcher})
}

func (s *segregatedScheduler) GetRemovableContainer(appName string, process string) (string, error) {
	ctx := context.TODO()
	a, err := app.GetByName(ctx, appName)
	if err != nil {
		return "", err
	}
	nodes, err := s.provisioner.Nodes(a)
	if err != nil {
		return "", err
	}
	return s.chooseContainerToRemove(nodes, appName, process)
}

type errContainerNotFound struct {
	AppName     string
	ProcessName string
}

func (m *errContainerNotFound) Error() string {
	return fmt.Sprintf("Container of app %q with process %q was not found in any servers", m.AppName, m.ProcessName)
}

func (s *segregatedScheduler) getContainerPreferablyFromHost(host string, appName, process string) (string, error) {
	coll := s.provisioner.Collection()
	defer coll.Close()
	var c container.Container
	query := bson.M{
		"id":       bson.M{"$nin": s.ignoredContainers},
		"appname":  appName,
		"hostaddr": net.URLToHost(host),
	}
	if process == "" {
		query["$or"] = []bson.M{{"processname": bson.M{"$exists": false}}, {"processname": ""}}
	} else {
		query["processname"] = process
	}
	err := coll.Find(query).Select(bson.M{"id": 1}).One(&c)
	if err == mgo.ErrNotFound {
		delete(query, "hostaddr")
		err = coll.Find(query).Select(bson.M{"id": 1}).One(&c)
	}
	if err == mgo.ErrNotFound {
		return "", &errContainerNotFound{AppName: appName, ProcessName: process}
	}
	return c.ID, err
}

func (s *segregatedScheduler) nodesToHosts(nodes []cluster.Node) ([]string, map[string]string) {
	hosts := make([]string, len(nodes))
	hostsMap := make(map[string]string)
	// Only hostname is saved in the docker containers collection
	// so we need to extract and map then to the original node.
	for i, node := range nodes {
		host := net.URLToHost(node.Address)
		hosts[i] = host
		hostsMap[host] = node.Address
	}
	return hosts, hostsMap
}

// chooseNodeToAdd finds which is the node with the minimum number of containers
// and returns it
func (s *segregatedScheduler) chooseNodeToAdd(nodes []cluster.Node, contName string, appName, process string) (string, error) {
	log.Debugf("[scheduler] Possible nodes for container %s: %#v", contName, nodes)
	s.hostMutex.Lock()
	defer s.hostMutex.Unlock()
	chosenNode, _, err := s.minMaxNodes(nodes, appName, process)
	if err != nil {
		return "", err
	}
	log.Debugf("[scheduler] Chosen node for container %s: %#v", contName, chosenNode)
	if contName != "" {
		coll := s.provisioner.Collection()
		defer coll.Close()
		err = coll.Update(bson.M{"name": contName}, bson.M{"$set": bson.M{"hostaddr": net.URLToHost(chosenNode)}})
	}
	return chosenNode, err
}

// chooseContainerToRemove finds a container from the the node with maximum
// number of containers and returns it
func (s *segregatedScheduler) chooseContainerToRemove(nodes []cluster.Node, appName, process string) (string, error) {
	_, chosenNode, err := s.minMaxNodes(nodes, appName, process)
	if err != nil {
		return "", err
	}
	log.Debugf("[scheduler] Chosen node for remove a container: %#v", chosenNode)
	containerID, err := s.getContainerPreferablyFromHost(chosenNode, appName, process)
	if err != nil {
		return "", err
	}
	return containerID, err
}

func appGroupCount(hostGroups map[string]int, appCountHost map[string]int) map[string]int {
	groupCounters := map[int]int{}
	for host, count := range appCountHost {
		groupCounters[hostGroups[host]] += count
	}
	result := map[string]int{}
	for host := range hostGroups {
		result[host] = groupCounters[hostGroups[host]]
	}
	return result
}

// Find the host with the minimum (good to add a new container) and maximum
// (good to remove a container) value for the pair [(number of containers for
// app-process), (number of containers in host)]
func (s *segregatedScheduler) minMaxNodes(nodes []cluster.Node, appName, process string) (string, string, error) {
	nodesList := make(node.NodeList, len(nodes))
	for i := range nodes {
		nodesList[i] = &clusterNodeWrapper{Node: &nodes[i], prov: s.provisioner}
	}
	metaFreqList, _, err := nodesList.SplitMetadata()
	if err != nil {
		log.Debugf("[scheduler] ignoring metadata diff when selecting node: %s", err)
	}
	hostGroupMap := map[string]int{}
	for i, m := range metaFreqList {
		for _, n := range m.Nodes {
			hostGroupMap[net.URLToHost(n.Address())] = i
		}
	}
	hosts, hostsMap := s.nodesToHosts(nodes)
	hostCountMap, err := s.aggregateContainersByHost(hosts)
	if err != nil {
		return "", "", err
	}
	appCountMap, err := s.aggregateContainersByHostAppProcess(hosts, appName, process)
	if err != nil {
		return "", "", err
	}
	priorityEntries := []map[string]int{appGroupCount(hostGroupMap, appCountMap), appCountMap, hostCountMap}
	var minHost, maxHost string
	var minScore uint64 = math.MaxUint64
	var maxScore uint64 = 0
	for _, host := range hosts {
		var score uint64
		for i, e := range priorityEntries {
			score += uint64(e[host]) << uint((len(priorityEntries)-i-1)*(64/len(priorityEntries)))
		}
		if score < minScore {
			minScore = score
			minHost = host
		}
		if score >= maxScore {
			maxScore = score
			maxHost = host
		}
	}
	return hostsMap[minHost], hostsMap[maxHost], nil
}

func filterNodes(nodes []cluster.Node, filter map[string]struct{}) []cluster.Node {
	if len(filter) == 0 {
		return nodes
	}
	for i := 0; i < len(nodes); i++ {
		if _, ok := filter[nodes[i].Address]; !ok {
			nodes[i] = nodes[len(nodes)-1]
			nodes = nodes[:len(nodes)-1]
			i--
		}
	}
	return nodes
}
