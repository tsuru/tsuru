// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package autoscale

import (
	"context"
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
)

type memoryScaler struct {
	*Config
	rule *Rule
}

type nodeMemoryData struct {
	node             provision.Node
	maxMemory        int64
	reserved         int64
	available        int64
	containersMemory map[string]int64
}

func (a *memoryScaler) nodesMemoryData(pool string, nodes []provision.Node) (map[string]*nodeMemoryData, error) {
	ctx := context.TODO()
	nodesMemoryData := make(map[string]*nodeMemoryData)
	unitsMap, err := preciseUnitsByNode(pool, nodes)
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		metadata := node.MetadataNoPrefix()
		totalMemory, _ := strconv.ParseFloat(metadata[a.TotalMemoryMetadata], 64)
		if totalMemory == 0.0 {
			return nil, errors.Errorf("no value found for memory metadata (%s) in node %s", a.TotalMemoryMetadata, node.Address())
		}
		maxMemory := int64(float64(a.rule.MaxMemoryRatio) * totalMemory)
		data := &nodeMemoryData{
			containersMemory: make(map[string]int64),
			node:             node,
			maxMemory:        maxMemory,
		}
		nodesMemoryData[node.Address()] = data
		for _, unit := range unitsMap[node.Address()] {
			a, err := app.GetByName(ctx, unit.AppName)
			if err != nil {
				return nil, errors.Wrapf(err, "couldn't find container app (%s)", unit.AppName)
			}
			data.containersMemory[unit.ID] = a.Plan.Memory
			data.reserved += a.Plan.Memory
		}
		data.available = data.maxMemory - data.reserved
	}
	return nodesMemoryData, nil
}

func (a *memoryScaler) chooseNodeForRemoval(maxPlanMemory int64, pool string, nodes []provision.Node) ([]provision.Node, error) {
	memoryData, err := a.nodesMemoryData(pool, nodes)
	if err != nil {
		return nil, err
	}
	var totalReserved, totalMem int64
	for _, node := range nodes {
		data := memoryData[node.Address()]
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

func (a *memoryScaler) scale(pool string, nodes []provision.Node) (*ScalerResult, error) {
	ctx := context.TODO()
	plans, err := servicemanager.Plan.List(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't list plans")
	}
	var maxPlanMemory int64
	for _, plan := range plans {
		if plan.Memory > maxPlanMemory {
			maxPlanMemory = plan.Memory
		}
	}
	if maxPlanMemory == 0 {
		var defaultPlan *appTypes.Plan
		defaultPlan, err = servicemanager.Plan.DefaultPlan(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "couldn't get default plan")
		}
		maxPlanMemory = defaultPlan.Memory
	}
	chosenNodes, err := a.chooseNodeForRemoval(maxPlanMemory, pool, nodes)
	if err != nil {
		return nil, err
	}
	if chosenNodes != nil {
		return &ScalerResult{
			ToRemove: nodesToSpec(chosenNodes),
			Reason:   fmt.Sprintf("containers can be distributed in only %d nodes", len(nodes)-len(chosenNodes)),
		}, nil
	}
	memoryData, err := a.nodesMemoryData(pool, nodes)
	if err != nil {
		return nil, err
	}
	canFitMax := false
	var totalReserved, totalMem int64
	for _, node := range nodes {
		data := memoryData[node.Address()]
		if maxPlanMemory > data.maxMemory {
			return nil, errors.Errorf("aborting, impossible to fit max plan memory of %d bytes, node max available memory is %d", maxPlanMemory, data.maxMemory)
		}
		totalReserved += data.reserved
		totalMem += data.maxMemory
		if data.available >= maxPlanMemory {
			canFitMax = true
			break
		}
	}
	if canFitMax {
		return &ScalerResult{}, nil
	}
	nodesToAdd := int((totalReserved + maxPlanMemory) / totalMem)
	if nodesToAdd == 0 {
		return &ScalerResult{}, nil
	}
	return &ScalerResult{
		ToAdd:  nodesToAdd,
		Reason: fmt.Sprintf("can't add %d bytes to an existing node", maxPlanMemory),
	}, nil
}
