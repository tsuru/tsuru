// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	"time"
)

type Healer struct {
	cluster *cluster.Cluster
}

func (h *Healer) HandleError(node cluster.Node) time.Duration {
	defaultWait := 1 * time.Minute
	failures := node.FailureCount()
	if failures < 5 {
		return defaultWait
	}
	failingAddr := node.Address
	failingHost := urlToHost(failingAddr)
	containers, err := listContainersByHost(failingHost)
	if err != nil {
		log.Errorf("Error in cluster healer, trying to list containers: %s", err.Error())
		return defaultWait
	}
	// Empty host let's just try again in the future
	if len(containers) == 0 {
		return defaultWait
	}
	iaasName, hasIaas := node.Metadata["iaas"]
	if !hasIaas {
		log.Errorf("Can't auto-heal after %d failures for node %s: no IaaS information.", failures, failingHost)
		return defaultWait
	}
	machine, err := iaas.CreateMachineForIaaS(iaasName, node.Metadata)
	if err != nil {
		log.Errorf("Can't auto-heal after %d failures for node %s: error creating new machine: %s", failures, failingHost, err.Error())
		return defaultWait
	}
	newAddr, err := machine.FormatNodeAddress()
	if err != nil {
		log.Errorf("Can't auto-heal after %d failures for node %s: error formatting address: %s", failures, failingHost, err.Error())
		machine.Destroy()
		return defaultWait
	}
	cluster := dockerCluster()
	err = cluster.Unregister(failingAddr)
	if err != nil {
		log.Errorf("Can't auto-heal after %d failures for node %s: error unregistering old node: %s", failures, failingHost, err.Error())
		return defaultWait
	}
	err = cluster.WaitAndRegister(newAddr, node.Metadata, 2*time.Minute)
	if err != nil {
		log.Errorf("Can't auto-heal after %d failures for node %s: error registering new node: %s", failures, failingHost, err.Error())
		machine.Destroy()
		return defaultWait
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	err = moveContainers(failingHost, machine.Address, encoder)
	if err != nil {
		log.Errorf("Unable to move containers from: %s to: %s - %s", failingHost, machine.Address, err.Error())
		return 0
	}
	failingMachine, err := iaas.FindMachineByAddress(failingHost)
	if err != nil {
		log.Errorf("Unable to find failing machine %s in IaaS", failingHost)
		return 0
	}
	err = failingMachine.Destroy()
	if err != nil {
		log.Errorf("Unable to find destroy machine %s from IaaS", failingHost)
	}
	return 0
}
