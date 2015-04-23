// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"time"

	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
)

type nodeHealer struct {
	provisioner           *dockerProvisioner
	disabledTime          time.Duration
	waitTimeNewMachine    time.Duration
	failuresBeforeHealing int
}

func (h *nodeHealer) healNode(node *cluster.Node) (cluster.Node, error) {
	emptyNode := cluster.Node{}
	failingAddr := node.Address
	nodeMetadata := node.CleanMetadata()
	failingHost := urlToHost(failingAddr)
	failures := node.FailureCount()
	machine, err := iaas.CreateMachineForIaaS(nodeMetadata["iaas"], nodeMetadata)
	if err != nil {
		node.ResetFailures()
		return emptyNode, fmt.Errorf("Can't auto-heal after %d failures for node %s: error creating new machine: %s", failures, failingHost, err.Error())
	}
	err = h.provisioner.getCluster().Unregister(failingAddr)
	if err != nil {
		machine.Destroy()
		return emptyNode, fmt.Errorf("Can't auto-heal after %d failures for node %s: error unregistering old node: %s", failures, failingHost, err.Error())
	}
	newAddr := machine.FormatNodeAddress()
	log.Debugf("New machine created during healing process: %s - Waiting for docker to start...", newAddr)
	createdNode, err := h.provisioner.getCluster().WaitAndRegister(newAddr, nodeMetadata, h.waitTimeNewMachine)
	if err != nil {
		node.ResetFailures()
		h.provisioner.getCluster().Register(failingAddr, nodeMetadata)
		machine.Destroy()
		return emptyNode, fmt.Errorf("Can't auto-heal after %d failures for node %s: error registering new node: %s", failures, failingHost, err.Error())
	}
	var buf bytes.Buffer
	err = h.provisioner.moveContainers(failingHost, "", &buf)
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

func (h *nodeHealer) HandleError(node *cluster.Node) time.Duration {
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
	evt, err := newHealingEvent(*node)
	if err != nil {
		log.Errorf("Error trying to insert healing event: %s", err.Error())
		return h.disabledTime
	}
	createdNode, err := h.healNode(node)
	if err != nil {
		log.Errorf("Error healing: %s", err.Error())
	}
	err = evt.update(createdNode, err)
	if err != nil {
		log.Errorf("Error trying to update healing event: %s", err.Error())
	}
	if createdNode.Address != "" {
		return 0
	}
	return h.disabledTime
}
