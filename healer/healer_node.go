// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/scopedconfig"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	nodeHealerConfigCollection = "node-healer"
	poolMetadataName           = "pool"
)

type NodeHealer struct {
	wg                    sync.WaitGroup
	disabledTime          time.Duration
	waitTimeNewMachine    time.Duration
	failuresBeforeHealing int
	quit                  chan bool
	started               time.Time
}

type nodeHealerArgs struct {
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

type NodeStatusData struct {
	Address     string       `bson:"_id,omitempty"`
	Checks      []NodeChecks `bson:",omitempty"`
	LastSuccess time.Time    `bson:",omitempty"`
	LastUpdate  time.Time
}

type NodeChecks struct {
	Time   time.Time
	Checks []provision.NodeCheckResult
}

type NodeHealerCustomData struct {
	Node      provision.NodeSpec
	Reason    string
	LastCheck *NodeChecks
}

func newNodeHealer(args nodeHealerArgs) *NodeHealer {
	healer := &NodeHealer{
		quit:                  make(chan bool),
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

func (h *NodeHealer) healNode(node provision.Node) (*provision.NodeSpec, error) {
	failingAddr := node.Address()
	// Copy metadata to ensure underlying data structure is not modified.
	newNodeMetadata := map[string]string{}
	for k, v := range node.Metadata() {
		newNodeMetadata[k] = v
	}
	failingHost := net.URLToHost(failingAddr)
	healthNode, isHealthNode := node.(provision.NodeHealthChecker)
	failures := 0
	if isHealthNode {
		failures = healthNode.FailureCount()
	}
	machine, err := iaas.CreateMachineForIaaS(newNodeMetadata["iaas"], newNodeMetadata)
	if err != nil {
		if isHealthNode {
			healthNode.ResetFailures()
		}
		return nil, errors.Wrapf(err, "Can't auto-heal after %d failures for node %s: error creating new machine", failures, failingHost)
	}
	err = node.Provisioner().UpdateNode(provision.UpdateNodeOptions{
		Address: failingAddr,
		Disable: true,
	})
	if err != nil {
		machine.Destroy()
		return nil, errors.Wrapf(err, "Can't auto-heal after %d failures for node %s: error unregistering old node", failures, failingHost)
	}
	newAddr := machine.FormatNodeAddress()
	log.Debugf("New machine created during healing process: %s - Waiting for docker to start...", newAddr)
	createOpts := provision.AddNodeOptions{
		Address:    newAddr,
		Metadata:   newNodeMetadata,
		WaitTO:     h.waitTimeNewMachine,
		CaCert:     machine.CaCert,
		ClientCert: machine.ClientCert,
		ClientKey:  machine.ClientKey,
	}
	err = node.Provisioner().AddNode(createOpts)
	if err != nil {
		if isHealthNode {
			healthNode.ResetFailures()
		}
		node.Provisioner().UpdateNode(provision.UpdateNodeOptions{Address: failingAddr, Enable: true})
		machine.Destroy()
		return nil, errors.Wrapf(err, "Can't auto-heal after %d failures for node %s: error registering new node", failures, failingHost)
	}
	nodeSpec := provision.NodeToSpec(node)
	nodeSpec.Address = newAddr
	nodeSpec.Metadata = newNodeMetadata
	var buf bytes.Buffer
	err = node.Provisioner().RemoveNode(provision.RemoveNodeOptions{
		Address:   failingAddr,
		Rebalance: true,
		Writer:    &buf,
	})
	if err != nil {
		log.Errorf("Unable to move containers, skipping containers healing %q -> %q: %s: %s", failingHost, machine.Address, err, buf.String())
	}
	err = h.RemoveNode(node)
	if err != nil {
		log.Errorf("Unable to remove node %s status from healer: %s", node.Address(), err)
	}
	failingMachine, err := iaas.FindMachineByIdOrAddress(node.Metadata()["iaas-id"], failingHost)
	if err != nil {
		return &nodeSpec, errors.Wrapf(err, "Unable to find failing machine %s in IaaS", failingHost)
	}
	err = failingMachine.Destroy()
	if err != nil {
		return &nodeSpec, errors.Wrapf(err, "Unable to destroy machine %s from IaaS", failingHost)
	}
	err = node.Provisioner().RemoveNode(provision.RemoveNodeOptions{
		Address: failingAddr,
	})
	if err != nil && err != provision.ErrNodeNotFound {
		return &nodeSpec, errors.Wrapf(err, "Unable to remove node %s from provisioner", failingHost)
	}
	log.Debugf("Done auto-healing node %q, node %q created in its place.", failingHost, machine.Address)
	return &nodeSpec, nil
}

func (h *NodeHealer) tryHealingNode(node provision.Node, reason string, lastCheck *NodeChecks) error {
	_, hasIaas := node.Metadata()["iaas"]
	if !hasIaas {
		log.Debugf("node %q doesn't have IaaS information, healing (%s) won't run on it.", node.Address(), reason)
		return nil
	}
	poolName := node.Metadata()[poolMetadataName]
	evt, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Type: event.TargetTypeNode, Value: node.Address()},
		InternalKind: "healer",
		CustomData: NodeHealerCustomData{
			Node:      provision.NodeToSpec(node),
			Reason:    reason,
			LastCheck: lastCheck,
		},
		Allowed: event.Allowed(permission.PermPoolReadEvents, permission.Context(permission.CtxPool, poolName)),
	})
	if err != nil {
		if _, ok := err.(event.ErrEventLocked); ok {
			// Healing in progress.
			return nil
		}
		return errors.Wrap(err, "Error trying to insert node healing event, healing aborted")
	}
	var createdNode *provision.NodeSpec
	var evtErr error
	defer func() {
		var updateErr error
		if evtErr == nil && createdNode == nil {
			updateErr = evt.Abort()
		} else {
			updateErr = evt.DoneCustomData(evtErr, createdNode)
		}
		if updateErr != nil {
			log.Errorf("error trying to update healing event: %s", updateErr)
		}
	}()
	_, err = node.Provisioner().GetNode(node.Address())
	if err != nil {
		if err == provision.ErrNodeNotFound {
			return nil
		}
		evtErr = errors.Wrap(err, "unable to check if node still exists")
		return evtErr
	}
	shouldHeal, err := h.shouldHealNode(node)
	if err != nil {
		evtErr = errors.Wrap(err, "unable to check if node should be healed")
		return evtErr
	}
	if !shouldHeal {
		return nil
	}
	log.Errorf("initiating healing process for node %q due to: %s", node.Address(), reason)
	createdNode, evtErr = h.healNode(node)
	return evtErr
}

func (h *NodeHealer) HandleError(node provision.NodeHealthChecker) time.Duration {
	h.wg.Add(1)
	defer h.wg.Done()
	failures := node.FailureCount()
	if failures < h.failuresBeforeHealing {
		log.Debugf("%d failures detected in node %q, waiting for more failures before healing.", failures, node.Address())
		return h.disabledTime
	}
	if !node.HasSuccess() {
		log.Debugf("Node %q has never been successfully reached, healing won't run on it.", node.Address())
		return h.disabledTime
	}
	err := h.tryHealingNode(node, fmt.Sprintf("%d consecutive failures", failures), nil)
	if err != nil {
		log.Errorf("[node healer handle error] %s", err)
	}
	return h.disabledTime
}

func (h *NodeHealer) Shutdown(ctx context.Context) error {
	h.wg.Done()
	h.wg.Wait()
	h.quit <- true
	<-h.quit
	return nil
}

func (h *NodeHealer) String() string {
	return "node healer"
}

func allNodes() ([]provision.Node, error) {
	provs, err := provision.Registry()
	if err != nil {
		return nil, err
	}
	var nodes []provision.Node
	for _, p := range provs {
		if nodeProv, ok := p.(provision.NodeProvisioner); ok {
			var provNodes []provision.Node
			provNodes, err = nodeProv.ListNodes(nil)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, provNodes...)
		}
	}
	return nodes, nil
}

func (h *NodeHealer) UpdateNodeData(node provision.Node, checks []provision.NodeCheckResult) error {
	isSuccess := true
	for _, c := range checks {
		isSuccess = c.Successful
		if !isSuccess {
			break
		}
	}
	now := time.Now().UTC()
	toInsert := NodeStatusData{
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
	_, err = coll.UpsertId(node.Address(), bson.M{
		"$set": toInsert,
		"$push": bson.M{
			"checks": bson.D([]bson.DocElem{
				{Name: "$each", Value: []NodeChecks{{Time: now, Checks: checks}}},
				{Name: "$slice", Value: -10},
			}),
		},
	})
	return err
}

func (h *NodeHealer) RemoveNode(node provision.Node) error {
	coll, err := nodeDataCollection()
	if err != nil {
		return errors.Wrap(err, "unable to get node data collection")
	}
	defer coll.Close()
	err = coll.RemoveId(node.Address())
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

func (h *NodeHealer) queryPartForConfig(nodes []provision.Node, config NodeHealerConfig) (bson.M, error) {
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
		nodeAddresses[i] = nodes[i].Address()
	}
	return bson.M{
		"_id": bson.M{"$in": nodeAddresses},
		"$or": orParts,
	}, nil
}

func (h *NodeHealer) shouldHealNode(node provision.Node) (bool, error) {
	conf := healerConfig()
	var configEntry NodeHealerConfig
	err := conf.Load(node.Metadata()[poolMetadataName], &configEntry)
	if err != nil {
		return false, err
	}
	queryPart, err := h.queryPartForConfig([]provision.Node{node}, configEntry)
	if err != nil {
		return false, err
	}
	if queryPart == nil {
		return false, nil
	}
	coll, err := nodeDataCollection()
	if err != nil {
		return false, errors.Wrap(err, "unable to get node data collection")
	}
	defer coll.Close()
	count, err := coll.Find(queryPart).Count()
	if err != nil {
		return false, errors.Wrap(err, "unable to find nodes to heal")
	}
	return count > 0, nil
}

func (h *NodeHealer) findNodesForHealing() ([]NodeStatusData, map[string]provision.Node, error) {
	nodes, err := allNodes()
	if err != nil {
		return nil, nil, errors.Wrap(err, "unable to retrieve nodes")
	}
	nodesPoolMap := map[string][]provision.Node{}
	nodesAddrMap := map[string]provision.Node{}
	for i, n := range nodes {
		if _, ok := n.Provisioner().(provision.NodeContainerProvisioner); !ok {
			continue
		}
		pool := n.Metadata()[poolMetadataName]
		nodesPoolMap[pool] = append(nodesPoolMap[pool], nodes[i])
		nodesAddrMap[n.Address()] = nodes[i]
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
	var remainingNodes []provision.Node
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
		return nil, nil, errors.Wrap(err, "unable to get node data collection")
	}
	defer coll.Close()
	var nodesStatus []NodeStatusData
	err = coll.Find(bson.M{"$or": query}).All(&nodesStatus)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unable to find nodes to heal")
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
		return errors.Wrap(err, "unable to save config")
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
		return nil, errors.Wrap(err, "unable to unmarshal config")
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
