// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"strconv"

	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
)

type memoryScaler struct {
	*autoScaleConfig
	rule *autoScaleRule
}

type nodeMemoryData struct {
	node             *cluster.Node
	maxMemory        int64
	reserved         int64
	available        int64
	containersMemory map[string]int64
}

func (a *memoryScaler) nodesMemoryData(nodes []*cluster.Node) (map[string]*nodeMemoryData, error) {
	nodesMemoryData := make(map[string]*nodeMemoryData)
	containersMap, err := a.provisioner.runningContainersByNode(nodes)
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		totalMemory, _ := strconv.ParseFloat(node.Metadata[a.TotalMemoryMetadata], 64)
		if totalMemory == 0.0 {
			return nil, fmt.Errorf("no value found for memory metadata (%s) in node %s", a.TotalMemoryMetadata, node.Address)
		}
		maxMemory := int64(float64(a.rule.MaxMemoryRatio) * totalMemory)
		data := &nodeMemoryData{
			containersMemory: make(map[string]int64),
			node:             node,
			maxMemory:        maxMemory,
		}
		nodesMemoryData[node.Address] = data
		for _, cont := range containersMap[node.Address] {
			a, err := app.GetByName(cont.AppName)
			if err != nil {
				return nil, fmt.Errorf("couldn't find container app (%s): %s", cont.AppName, err)
			}
			data.containersMemory[cont.ID] = a.Plan.Memory
			data.reserved += a.Plan.Memory
		}
		data.available = data.maxMemory - data.reserved
	}
	return nodesMemoryData, nil
}

func (a *memoryScaler) chooseNodeForRemoval(maxPlanMemory int64, groupMetadata string, nodes []*cluster.Node) ([]cluster.Node, error) {
	memoryData, err := a.nodesMemoryData(nodes)
	if err != nil {
		return nil, err
	}
	var totalReserved, totalMem int64
	for _, node := range nodes {
		data := memoryData[node.Address]
		totalReserved += data.reserved
		totalMem += data.maxMemory
	}
	memPerNode := totalMem / int64(len(nodes))
	scaledMaxPlan := int64(float32(maxPlanMemory) * a.rule.ScaleDownRatio)
	toRemoveCount := len(nodes) - int(((totalReserved+scaledMaxPlan)/memPerNode)+1)
	if toRemoveCount <= 0 {
		return nil, nil
	}
	chosenNodes := chooseNodeForRemoval(nodes, toRemoveCount)
	if len(chosenNodes) == 0 {
		return nil, nil
	}
	return chosenNodes, nil
}

func (a *memoryScaler) scale(groupMetadata string, nodes []*cluster.Node) (*scalerResult, error) {
	plans, err := app.PlansList()
	if err != nil {
		return nil, fmt.Errorf("couldn't list plans: %s", err)
	}
	var maxPlanMemory int64
	for _, plan := range plans {
		if plan.Memory > maxPlanMemory {
			maxPlanMemory = plan.Memory
		}
	}
	if maxPlanMemory == 0 {
		var defaultPlan *app.Plan
		defaultPlan, err = app.DefaultPlan()
		if err != nil {
			return nil, fmt.Errorf("couldn't get default plan: %s", err)
		}
		maxPlanMemory = defaultPlan.Memory
	}
	chosenNodes, err := a.chooseNodeForRemoval(maxPlanMemory, groupMetadata, nodes)
	if err != nil {
		return nil, fmt.Errorf("unable to choose node for removal: %s", err)
	}
	if chosenNodes != nil {
		return &scalerResult{
			toRemove: chosenNodes,
			reason:   fmt.Sprintf("containers can be distributed in only %d nodes", len(nodes)-len(chosenNodes)),
		}, nil
	}
	memoryData, err := a.nodesMemoryData(nodes)
	if err != nil {
		return nil, err
	}
	canFitMax := false
	var totalReserved, totalMem int64
	for _, node := range nodes {
		data := memoryData[node.Address]
		if maxPlanMemory > data.maxMemory {
			return nil, fmt.Errorf("aborting, impossible to fit max plan memory of %d bytes, node max available memory is %d", maxPlanMemory, data.maxMemory)
		}
		totalReserved += data.reserved
		totalMem += data.maxMemory
		if data.available >= maxPlanMemory {
			canFitMax = true
			break
		}
	}
	if canFitMax {
		return nil, nil
	}
	nodesToAdd := int((totalReserved + maxPlanMemory) / totalMem)
	if nodesToAdd == 0 {
		return nil, nil
	}
	return &scalerResult{
		toAdd:  nodesToAdd,
		reason: fmt.Sprintf("can't add %d bytes to an existing node", maxPlanMemory),
	}, nil
}
