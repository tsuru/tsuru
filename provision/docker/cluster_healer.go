// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
)

type Healer struct {
	cluster               *cluster.Cluster
	disabledTime          time.Duration
	waitTimeNewMachine    time.Duration
	failuresBeforeHealing int
}

func (h *Healer) healNode(node cluster.Node) (bool, error) {
	failingAddr := node.Address
	nodeMetadata := node.CleanMetadata()
	failingHost := urlToHost(failingAddr)
	failures := node.FailureCount()
	iaasName, hasIaas := nodeMetadata["iaas"]
	if !hasIaas {
		return false, fmt.Errorf("Can't auto-heal after %d failures for node %s: no IaaS information.", failures, failingHost)
	}
	machine, err := iaas.CreateMachineForIaaS(iaasName, nodeMetadata)
	if err != nil {
		return false, fmt.Errorf("Can't auto-heal after %d failures for node %s: error creating new machine: %s", failures, failingHost, err.Error())
	}
	newAddr, err := machine.FormatNodeAddress()
	if err != nil {
		machine.Destroy()
		return false, fmt.Errorf("Can't auto-heal after %d failures for node %s: error formatting address: %s", failures, failingHost, err.Error())
	}
	err = h.cluster.Unregister(failingAddr)
	if err != nil {
		machine.Destroy()
		return false, fmt.Errorf("Can't auto-heal after %d failures for node %s: error unregistering old node: %s", failures, failingHost, err.Error())
	}
	log.Debugf("New machine created during healing process: %s - Waiting for docker to start...", newAddr)
	err = h.cluster.WaitAndRegister(newAddr, nodeMetadata, h.waitTimeNewMachine)
	if err != nil {
		machine.Destroy()
		h.cluster.Register(failingAddr, nodeMetadata)
		return false, fmt.Errorf("Can't auto-heal after %d failures for node %s: error registering new node: %s", failures, failingHost, err.Error())
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	err = moveContainers(failingHost, machine.Address, encoder)
	if err != nil {
		return true, fmt.Errorf("Unable to move containers %q -> %q: %s: %s", failingHost, machine.Address, err.Error(), buf.String())
	}
	failingMachine, err := iaas.FindMachineByAddress(failingHost)
	if err != nil {
		return true, fmt.Errorf("Unable to find failing machine %s in IaaS: %s", failingHost, err.Error())
	}
	err = failingMachine.Destroy()
	if err != nil {
		return true, fmt.Errorf("Unable to destroy machine %s from IaaS: %s", failingHost, err.Error())
	}
	log.Debugf("Done auto-healing node %q, node %q created in its place.", failingHost, machine.Address)
	return true, nil
}

func (h *Healer) HandleError(node cluster.Node) time.Duration {
	failures := node.FailureCount()
	if failures < h.failuresBeforeHealing {
		log.Debugf("%d failures detected in node %q, waiting for more failures before healing.", failures, node.Address)
		return h.disabledTime
	}
	containers, err := listContainersByHost(urlToHost(node.Address))
	if err != nil {
		log.Errorf("Error in cluster healer, trying to list containers: %s", err.Error())
		return h.disabledTime
	}
	if len(containers) == 0 {
		log.Debugf("No containers in node %q, no need for healing to run.", node.Address)
		return h.disabledTime
	}
	log.Errorf("Initiating healing process for node %q after %d failures: %d containers to move", node.Address, failures, len(containers))
	created, err := h.healNode(node)
	if err != nil {
		log.Errorf("Error healing: %s", err.Error())
	}
	if created {
		return 0
	}
	return h.disabledTime
}
