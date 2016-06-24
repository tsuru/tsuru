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
	clusterStorage "github.com/tsuru/docker-cluster/storage"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/nodecontainer"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/scopedconfig"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	nodeHealerConfigCollection = "node-healer"
)

type NodeHealer struct {
	wg                    sync.WaitGroup
	provisioner           DockerProvisioner
	disabledTime          time.Duration
	waitTimeNewMachine    time.Duration
	failuresBeforeHealing int
	quit                  chan bool
	started               time.Time
}

type NodeHealerArgs struct {
	Provisioner           DockerProvisioner
	DisabledTime          time.Duration
	WaitTimeNewMachine    time.Duration
	FailuresBeforeHealing int
}

type NodeHealerConfig struct {
	Enabled                      *bool
	MaxTimeSinceSuccess          *int
	MaxUnresponsiveTime          *int
	EnabledInherited             bool
	MaxTimeSinceSuccessInherited bool
	MaxUnresponsiveTimeInherited bool
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
		provisioner:           args.Provisioner,
		disabledTime:          args.DisabledTime,
		waitTimeNewMachine:    args.WaitTimeNewMachine,
		failuresBeforeHealing: args.FailuresBeforeHealing,
		started:               time.Now().UTC(),
	}
	healer.wg.Add(1)
	go func() {
		defer close(healer.quit)
		for {
			healer.runActiveHealing()
			select {
			case <-healer.quit:
				return
			case <-time.After(30 * time.Second):
			}
		}
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
	job, err := q.EnqueueWait(nodecontainer.QueueTaskName, jobParams, h.waitTimeNewMachine)
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

func (h *NodeHealer) tryHealingNode(node *cluster.Node, reason string, lastCheck *nodeChecks) error {
	_, hasIaas := node.Metadata["iaas"]
	if !hasIaas {
		log.Debugf("node %q doesn't have IaaS information, healing (%s) won't run on it.", node.Address, reason)
		return nil
	}
	evt, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Name: "node", Value: node.Address},
		InternalKind: "healer",
		CustomData: map[string]interface{}{
			"node":      *node,
			"reason":    reason,
			"lastCheck": lastCheck,
		},
	})
	if err != nil {
		if _, ok := err.(event.ErrEventLocked); ok {
			// Healing in progress.
			return nil
		}
		return fmt.Errorf("Error trying to insert nodee healing event, healing aborted: %s", err.Error())
	}
	var createdNode cluster.Node
	var evtErr error
	defer func() {
		var updateErr error
		if createdNode.Address == "" {
			updateErr = evt.Abort()
		} else {
			updateErr = evt.DoneCustomData(evtErr, createdNode)
		}
		if updateErr != nil {
			log.Errorf("error trying to update healing event: %s", updateErr.Error())
		}
	}()
	_, err = h.provisioner.Cluster().GetNode(node.Address)
	if err != nil {
		if err == clusterStorage.ErrNoSuchNode {
			return nil
		}
		evtErr = fmt.Errorf("unable to check if node still exists: %s", err)
		return evtErr
	}
	shouldHeal, err := h.shouldHealNode(node)
	if err != nil {
		evtErr = fmt.Errorf("unable to check if node should be healed: %s", err)
		return evtErr
	}
	if !shouldHeal {
		return nil
	}
	log.Errorf("initiating healing process for node %q due to: %s", node.Address, reason)
	createdNode, evtErr = h.healNode(node)
	return evtErr
}

func (h *NodeHealer) HandleError(node *cluster.Node) time.Duration {
	h.wg.Add(1)
	defer h.wg.Done()
	failures := node.FailureCount()
	if failures < h.failuresBeforeHealing {
		log.Debugf("%d failures detected in node %q, waiting for more failures before healing.", failures, node.Address)
		return h.disabledTime
	}
	if !node.HasSuccess() {
		log.Debugf("Node %q has never been successfully reached, healing won't run on it.", node.Address)
		return h.disabledTime
	}
	err := h.tryHealingNode(node, fmt.Sprintf("%d consecutive failures", failures), nil)
	if err != nil {
		log.Errorf("[node healer handle error] %s", err)
	}
	return h.disabledTime
}

func (h *NodeHealer) Shutdown() {
	h.wg.Done()
	h.wg.Wait()
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
		if !isSuccess {
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
	defer coll.Close()
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

func (h *NodeHealer) RunClusterHook(evt cluster.HookEvent, node *cluster.Node) error {
	coll, err := nodeDataCollection()
	if err != nil {
		return fmt.Errorf("unable to get node data collection: %s", err)
	}
	defer coll.Close()
	err = coll.RemoveId(node.Address)
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	return nil
}

func healerConfig() *scopedconfig.ScopedConfig {
	conf := scopedconfig.FindScopedConfig(nodeHealerConfigCollection)
	conf.AllowEmpty = true
	return conf
}

func (h *NodeHealer) queryPartForConfig(nodes []*cluster.Node, config NodeHealerConfig) (bson.M, error) {
	now := time.Now().UTC()
	if config.Enabled == nil || !*config.Enabled {
		return nil, nil
	}
	var orParts []bson.M
	if config.MaxTimeSinceSuccess != nil && *config.MaxTimeSinceSuccess > 0 {
		lastSuccess := time.Duration(*config.MaxTimeSinceSuccess) * time.Second
		nowMinusLastSuccess := now.Add(-lastSuccess)
		if h.started.Add(lastSuccess).Before(nowMinusLastSuccess) {
			orParts = append(orParts, bson.M{
				"lastsuccess": bson.M{"$lt": nowMinusLastSuccess},
			})
		}
	}
	if config.MaxUnresponsiveTime != nil && *config.MaxUnresponsiveTime > 0 {
		lastUpdate := time.Duration(*config.MaxUnresponsiveTime) * time.Second
		nowMinusLastUpdate := now.Add(-lastUpdate)
		if h.started.Add(lastUpdate).Before(nowMinusLastUpdate) {
			orParts = append(orParts, bson.M{
				"lastupdate": bson.M{"$lt": nowMinusLastUpdate},
			})
		}
	}
	if len(orParts) == 0 {
		return nil, nil
	}
	nodeAddresses := make([]string, len(nodes))
	for i := range nodes {
		nodeAddresses[i] = nodes[i].Address
	}
	return bson.M{
		"_id": bson.M{"$in": nodeAddresses},
		"$or": orParts,
	}, nil
}

func (h *NodeHealer) shouldHealNode(node *cluster.Node) (bool, error) {
	conf := healerConfig()
	var configEntry NodeHealerConfig
	err := conf.Load(node.Metadata["pool"], &configEntry)
	if err != nil {
		return false, err
	}
	queryPart, err := h.queryPartForConfig([]*cluster.Node{node}, configEntry)
	if err != nil {
		return false, err
	}
	if queryPart == nil {
		return false, nil
	}
	coll, err := nodeDataCollection()
	if err != nil {
		return false, fmt.Errorf("unable to get node data collection: %s", err)
	}
	defer coll.Close()
	count, err := coll.Find(queryPart).Count()
	if err != nil {
		return false, fmt.Errorf("unable to find nodes to heal: %s", err)
	}
	return count > 0, nil
}

func (h *NodeHealer) findNodesForHealing() ([]nodeStatusData, map[string]*cluster.Node, error) {
	nodes, err := h.provisioner.Cluster().UnfilteredNodes()
	if err != nil {
		return nil, nil, fmt.Errorf("unable to retrieve nodes: %s", err)
	}
	nodesPoolMap := map[string][]*cluster.Node{}
	nodesAddrMap := map[string]*cluster.Node{}
	for i, n := range nodes {
		pool := n.Metadata["pool"]
		nodesPoolMap[pool] = append(nodesPoolMap[pool], &nodes[i])
		nodesAddrMap[n.Address] = &nodes[i]
	}
	conf := healerConfig()
	var entries map[string]NodeHealerConfig
	err = conf.LoadAll(&entries)
	if err != nil {
		return nil, nil, err
	}
	query := []bson.M{}
	for poolName, entry := range entries {
		if poolName == "" {
			continue
		}
		var q bson.M
		q, err = h.queryPartForConfig(nodesPoolMap[poolName], entry)
		if err != nil {
			return nil, nil, err
		}
		if q != nil {
			query = append(query, q)
		}
		delete(nodesPoolMap, poolName)
	}
	var remainingNodes []*cluster.Node
	for _, poolNodes := range nodesPoolMap {
		remainingNodes = append(remainingNodes, poolNodes...)
	}
	q, err := h.queryPartForConfig(remainingNodes, entries[""])
	if err != nil {
		return nil, nil, err
	}
	if q != nil {
		query = append(query, q)
	}
	if len(query) == 0 {
		return nil, nodesAddrMap, nil
	}
	coll, err := nodeDataCollection()
	if err != nil {
		return nil, nil, fmt.Errorf("unable to get node data collection: %s", err)
	}
	defer coll.Close()
	var nodesStatus []nodeStatusData
	err = coll.Find(bson.M{"$or": query}).All(&nodesStatus)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to find nodes to heal: %s", err)
	}
	return nodesStatus, nodesAddrMap, nil
}

func (h *NodeHealer) runActiveHealing() {
	nodesStatus, nodesAddrMap, err := h.findNodesForHealing()
	if err != nil {
		log.Errorf("[node healer active] %s", err)
		return
	}
	for _, n := range nodesStatus {
		sinceUpdate := time.Since(n.LastUpdate)
		sinceSuccess := time.Since(n.LastSuccess)
		err = h.tryHealingNode(nodesAddrMap[n.Address],
			fmt.Sprintf("last update %v ago, last success %v ago", sinceUpdate, sinceSuccess),
			&n.Checks[len(n.Checks)-1],
		)
		if err != nil {
			log.Errorf("[node healer active] %s", err)
		}
	}
}

func UpdateConfig(pool string, config NodeHealerConfig) error {
	conf := healerConfig()
	err := conf.SaveMerge(pool, config)
	if err != nil {
		return fmt.Errorf("unable to save config: %s", err)
	}
	return nil
}

func RemoveConfig(pool, name string) error {
	conf := healerConfig()
	var err error
	if name == "" {
		err = conf.Remove(pool)
	} else {
		err = conf.RemoveField(pool, name)
	}
	return err
}

func GetConfig() (map[string]NodeHealerConfig, error) {
	conf := healerConfig()
	var ret map[string]NodeHealerConfig
	err := conf.LoadAll(&ret)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal config: %s", err)
	}
	return ret, nil
}

func nodeDataCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	return conn.Collection("node_status"), nil
}
