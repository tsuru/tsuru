// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"

	"github.com/tsuru/docker-cluster/cluster"
)

type countScaler struct {
	*autoScaleConfig
	rule *autoScaleRule
}

func (a *countScaler) scale(groupMetadata string, nodes []*cluster.Node) (*scalerResult, error) {
	totalCount, _, err := a.provisioner.containerGapInNodes(nodes)
	if err != nil {
		return nil, fmt.Errorf("couldn't find containers from nodes: %s", err)
	}
	freeSlots := (len(nodes) * a.rule.MaxContainerCount) - totalCount
	reasonMsg := fmt.Sprintf("number of free slots is %d", freeSlots)
	scaledMaxCount := int(float32(a.rule.MaxContainerCount) * a.rule.ScaleDownRatio)
	if freeSlots > scaledMaxCount {
		toRemoveCount := freeSlots / scaledMaxCount
		chosenNodes := chooseNodeForRemoval(nodes, toRemoveCount)
		if len(chosenNodes) == 0 {
			a.logDebug("would remove any node but can't due to metadata restrictions")
			return nil, nil
		}
		return &scalerResult{
			toRemove: chosenNodes,
			reason:   reasonMsg,
		}, nil
	}
	if freeSlots >= 0 {
		return nil, nil
	}
	nodesToAdd := -freeSlots / a.rule.MaxContainerCount
	if nodesToAdd == 0 {
		nodesToAdd = 1
	}
	return &scalerResult{
		toAdd:  nodesToAdd,
		reason: reasonMsg,
	}, nil
}
