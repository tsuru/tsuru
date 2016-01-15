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
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision/docker/bs"
	"github.com/tsuru/tsuru/queue"
)

type NodeHealer struct {
	sync.Mutex
	locks                 map[string]*sync.Mutex
	provisioner           DockerProvisioner
	disabledTime          time.Duration
	waitTimeNewMachine    time.Duration
	failuresBeforeHealing int
}

type NodeHealerArgs struct {
	Provisioner           DockerProvisioner
	DisabledTime          time.Duration
	WaitTimeNewMachine    time.Duration
	FailuresBeforeHealing int
}

func NewNodeHealer(args NodeHealerArgs) *NodeHealer {
	return &NodeHealer{
		locks:                 make(map[string]*sync.Mutex),
		provisioner:           args.Provisioner,
		disabledTime:          args.DisabledTime,
		waitTimeNewMachine:    args.WaitTimeNewMachine,
		failuresBeforeHealing: args.FailuresBeforeHealing,
	}
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
}

func (h *NodeHealer) String() string {
	return "node healer"
}
