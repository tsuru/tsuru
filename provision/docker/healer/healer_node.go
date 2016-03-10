// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/bs"
	"github.com/tsuru/tsuru/queue"
	"gopkg.in/mgo.v2/bson"
)

const (
	HealerConfigEnabled             = "enabled"
	HealerConfigMaxTimeSinceSuccess = "maxtimesincesuccess"
	HealerConfigMaxUnresponsiveTime = "maxunresponsivetime"

	nodeHealerConfigEntry = "node-healer"
)

type NodeHealer struct {
	sync.Mutex
	locks                 map[string]*sync.Mutex
	provisioner           DockerProvisioner
	disabledTime          time.Duration
	waitTimeNewMachine    time.Duration
	failuresBeforeHealing int
	quit                  chan bool
}

type NodeHealerArgs struct {
	Provisioner           DockerProvisioner
	DisabledTime          time.Duration
	WaitTimeNewMachine    time.Duration
	FailuresBeforeHealing int
}

type nodeStatusData struct {
	Address     string       `bson:"_id,omitempty"`
	Checks      []nodeChecks `bson:",omitempty"`
	LastSuccess time.Time    `bson:",omitempty"`
	LastUpdate  time.Time
}

func NewNodeHealer(args NodeHealerArgs) *NodeHealer {
	healer := &NodeHealer{
		quit:                  make(chan bool),
		locks:                 make(map[string]*sync.Mutex),
		provisioner:           args.Provisioner,
		disabledTime:          args.DisabledTime,
		waitTimeNewMachine:    args.WaitTimeNewMachine,
		failuresBeforeHealing: args.FailuresBeforeHealing,
	}
	go func() {
		defer close(healer.quit)
		select {
		case <-healer.quit:
			return
		case <-time.After(30 * time.Second):
		}
		healer.checkActiveHealing()
	}()
	return healer
}

func (h *NodeHealer) healNode(node *cluster.Node) (cluster.Node, error) {
	emptyNode := cluster.Node{}
	failingAddr := node.Address
	nodeMetadata := node.CleanMetadata()
	failingHost := net.URLToHost(failingAddr)
	failures := node.FailureCount()
	machine, err := iaas.CreateMachineForIaaS(nodeMetadata["iaas"], nodeMetadata)
	if err != nil {
		node.ResetFailures()
		return emptyNode, fmt.Errorf("Can't auto-heal after %d failures for node %s: error creating new machine: %s", failures, failingHost, err.Error())
	}
	err = h.provisioner.Cluster().Unregister(failingAddr)
	if err != nil {
		machine.Destroy()
		return emptyNode, fmt.Errorf("Can't auto-heal after %d failures for node %s: error unregistering old node: %s", failures, failingHost, err.Error())
	}
	newAddr := machine.FormatNodeAddress()
	log.Debugf("New machine created during healing process: %s - Waiting for docker to start...", newAddr)
	createdNode := cluster.Node{
		Address:        newAddr,
		Metadata:       nodeMetadata,
		CreationStatus: cluster.NodeCreationStatusPending,
	}
	err = h.provisioner.Cluster().Register(createdNode)
	if err != nil {
		node.ResetFailures()
		h.provisioner.Cluster().Register(cluster.Node{Address: failingAddr, Metadata: nodeMetadata})
		machine.Destroy()
		return emptyNode, fmt.Errorf("Can't auto-heal after %d failures for node %s: error registering new node: %s", failures, failingHost, err.Error())
	}
	q, err := queue.Queue()
	if err != nil {
		return emptyNode, err
	}
	jobParams := monsterqueue.JobParams{
		"endpoint": createdNode.Address,
		"machine":  machine.Id,
		"metadata": createdNode.Metadata,
	}
	job, err := q.EnqueueWait(bs.QueueTaskName, jobParams, h.waitTimeNewMachine)
	if err == nil {
		_, err = job.Result()
	}
	if err != nil {
		node.ResetFailures()
		h.provisioner.Cluster().Register(cluster.Node{Address: failingAddr, Metadata: nodeMetadata})
		return emptyNode, fmt.Errorf("Can't auto-heal after %d failures for node %s: error waiting for the bs task: %s", failures, failingHost, err.Error())
	}
	var buf bytes.Buffer
	err = h.provisioner.MoveContainers(failingHost, "", &buf)
	if err != nil {
		log.Errorf("Unable to move containers, skipping containers healing %q -> %q: %s: %s", failingHost, machine.Address, err.Error(), buf.String())
	}
	failingMachine, err := iaas.FindMachineByIdOrAddress(node.Metadata["iaas-id"], failingHost)
	if err != nil {
		return createdNode, fmt.Errorf("Unable to find failing machine %s in IaaS: %s", failingHost, err.Error())
	}
	err = failingMachine.Destroy()
	if err != nil {
		return createdNode, fmt.Errorf("Unable to destroy machine %s from IaaS: %s", failingHost, err.Error())
	}
	log.Debugf("Done auto-healing node %q, node %q created in its place.", failingHost, machine.Address)
	return createdNode, nil
}

func (h *NodeHealer) HandleError(node *cluster.Node) time.Duration {
	h.Lock()
	if h.locks[node.Address] == nil {
		h.locks[node.Address] = &sync.Mutex{}
	}
	h.Unlock()
	h.locks[node.Address].Lock()
	defer h.locks[node.Address].Unlock()
	failures := node.FailureCount()
	if failures < h.failuresBeforeHealing {
		log.Debugf("%d failures detected in node %q, waiting for more failures before healing.", failures, node.Address)
		return h.disabledTime
	}
	if !node.HasSuccess() {
		log.Debugf("Node %q has never been successfully reached, healing won't run on it.", node.Address)
		return h.disabledTime
	}
	_, hasIaas := node.Metadata["iaas"]
	if !hasIaas {
		log.Debugf("Node %q doesn't have IaaS information, healing won't run on it.", node.Address)
		return h.disabledTime
	}
	healingCounter, err := healingCountFor("node", node.Address, consecutiveHealingsTimeframe)
	if err != nil {
		log.Errorf("Node healing: couldn't verify number of previous healings for %s: %s", node.Address, err.Error())
		return h.disabledTime
	}
	if healingCounter > consecutiveHealingsLimitInTimeframe {
		log.Errorf("Node healing: number of healings for node %s in the last %d minutes exceeds limit of %d: %d",
			node.Address, consecutiveHealingsTimeframe/time.Minute, consecutiveHealingsLimitInTimeframe, healingCounter)
		return h.disabledTime
	}
	log.Errorf("Initiating healing process for node %q after %d failures.", node.Address, failures)
	evt, err := NewHealingEvent(*node)
	if err != nil {
		log.Errorf("Error trying to insert healing event: %s", err.Error())
		return h.disabledTime
	}
	createdNode, err := h.healNode(node)
	if err != nil {
		log.Errorf("Error healing: %s", err.Error())
	}
	err = evt.Update(createdNode, err)
	if err != nil {
		log.Errorf("Error trying to update healing event: %s", err.Error())
	}
	if createdNode.Address != "" {
		return 0
	}
	return h.disabledTime
}

func (h *NodeHealer) Shutdown() {
	h.Lock()
	for _, lock := range h.locks {
		lock.Lock()
	}
	h.quit <- true
	<-h.quit
}

func (h *NodeHealer) String() string {
	return "node healer"
}

type nodeChecks struct {
	Time   time.Time
	Checks []provision.NodeCheckResult
}

func (h *NodeHealer) findNodeForNodeData(nodeData provision.NodeStatusData) (*cluster.Node, error) {
	nodes, err := h.provisioner.Cluster().UnfilteredNodes()
	if err != nil {
		return nil, err
	}
	nodeSet := map[string]*cluster.Node{}
	for i := range nodes {
		nodeSet[net.URLToHost(nodes[i].Address)] = &nodes[i]
	}
	containerIDs := make([]string, 0, len(nodeData.Units))
	containerNames := make([]string, 0, len(nodeData.Units))
	for _, u := range nodeData.Units {
		if u.ID != "" {
			containerIDs = append(containerIDs, u.ID)
		}
		if u.Name != "" {
			containerNames = append(containerNames, u.Name)
		}
	}
	containersForNode, err := h.provisioner.ListContainers(bson.M{
		"$or": []bson.M{
			{"name": bson.M{"$in": containerNames}},
			{"id": bson.M{"$in": containerIDs}},
		},
	})
	if err != nil {
		return nil, err
	}
	var node *cluster.Node
	for _, c := range containersForNode {
		n := nodeSet[c.HostAddr]
		if n != nil {
			if node != nil && node.Address != n.Address {
				return nil, fmt.Errorf("containers match multiple nodes: %s and %s", node.Address, n.Address)
			}
			node = n
		}
	}
	if node != nil {
		return node, nil
	}
	// Node not found through containers, try finding using addrs.
	for _, addr := range nodeData.Addrs {
		n := nodeSet[addr]
		if n != nil {
			if node != nil {
				return nil, fmt.Errorf("addrs match multiple nodes: %v", nodeData.Addrs)
			}
			node = n
		}
	}
	if node == nil {
		return nil, fmt.Errorf("node not found for addrs: %v", nodeData.Addrs)
	}
	return node, nil
}

func (h *NodeHealer) UpdateNodeData(nodeData provision.NodeStatusData) error {
	node, err := h.findNodeForNodeData(nodeData)
	if err != nil {
		return fmt.Errorf("[node healer update] %s", err)
	}
	isSuccess := true
	for _, c := range nodeData.Checks {
		isSuccess = c.Successful
		if isSuccess == false {
			break
		}
	}
	now := time.Now().UTC()
	toInsert := nodeStatusData{
		LastUpdate: now,
	}
	if isSuccess {
		toInsert.LastSuccess = now
	}
	coll, err := nodeDataCollection()
	if err != nil {
		return err
	}
	_, err = coll.UpsertId(node.Address, bson.M{
		"$set": toInsert,
		"$push": bson.M{
			"checks": bson.D([]bson.DocElem{
				{Name: "$each", Value: []nodeChecks{{Time: now, Checks: nodeData.Checks}}},
				{Name: "$slice", Value: -10},
			}),
		},
	})
	return err
}

func queryPartForConfig(nodes []*cluster.Node, entries provision.EntryMap) bson.M {
	now := time.Now().UTC()
	if enabled, ok := entries[HealerConfigEnabled].Value.(bool); !ok || !enabled {
		return nil
	}
	var orParts []bson.M
	if maxTime, ok := entries[HealerConfigMaxTimeSinceSuccess].Value.(int); ok && maxTime > 0 {
		lastSuccess := time.Duration(maxTime) * time.Second
		orParts = append(orParts, bson.M{
			"lastsuccess": bson.M{"$lt": now.Add(-lastSuccess)},
		})
	}
	if maxTime, ok := entries[HealerConfigMaxUnresponsiveTime].Value.(int); ok && maxTime > 0 {
		lastUpdate := time.Duration(maxTime) * time.Second
		orParts = append(orParts, bson.M{
			"lastupdate": bson.M{"$lt": now.Add(-lastUpdate)},
		})
	}
	if len(orParts) == 0 {
		return nil
	}
	nodeAddresses := make([]string, len(nodes))
	for i := range nodes {
		nodeAddresses[i] = nodes[i].Address
	}
	return bson.M{
		"_id": bson.M{"$in": nodeAddresses},
		"$or": orParts,
	}
}

func (h *NodeHealer) findNodesForHealing() ([]nodeStatusData, error) {
	conf, err := provision.FindScopedConfig(nodeHealerConfigEntry)
	if err != nil {
		return nil, fmt.Errorf("unable to find config: %s", err)
	}
	nodes, err := h.provisioner.Cluster().UnfilteredNodes()
	if err != nil {
		return nil, fmt.Errorf("unable to get cluster nodes: %s", err)
	}
	nodesPoolMap := map[string][]*cluster.Node{}
	for i, n := range nodes {
		pool := n.Metadata["pool"]
		nodesPoolMap[pool] = append(nodesPoolMap[pool], &nodes[i])
	}
	baseEntries, poolEntries := conf.AllEntries()
	query := []bson.M{}
	for poolName, entries := range poolEntries {
		q := queryPartForConfig(nodesPoolMap[poolName], entries)
		if q != nil {
			query = append(query, q)
		}
		delete(nodesPoolMap, poolName)
	}
	var remainingNodes []*cluster.Node
	for _, poolNodes := range nodesPoolMap {
		remainingNodes = append(remainingNodes, poolNodes...)
	}
	q := queryPartForConfig(remainingNodes, baseEntries)
	if q != nil {
		query = append(query, q)
	}
	if len(query) == 0 {
		return nil, nil
	}
	coll, err := nodeDataCollection()
	if err != nil {
		return nil, fmt.Errorf("unable to get node data collection: %s", err)
	}
	defer coll.Close()
	var nodesStatus []nodeStatusData
	err = coll.Find(bson.M{"$or": query}).All(&nodesStatus)
	if err != nil {
		return nil, fmt.Errorf("unable to find nodes to heal: %s", err)
	}
	return nodesStatus, nil
}

func (h *NodeHealer) checkActiveHealing() {
	_, err := h.findNodesForHealing()
	if err != nil {
		log.Errorf("[node healer check] %s", err)
	}

}

func nodeDataCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	return conn.Collection("node_status"), nil
}
