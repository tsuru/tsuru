// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package autoscale

import (
	"fmt"

	"github.com/tsuru/tsuru/provision"
)

type countScaler struct {
	*Config
	rule *Rule
}

func (a *countScaler) scale(pool string, nodes []provision.Node) (*ScalerResult, error) {
	totalCount, _, err := unitsGapInNodes(pool, nodes)
	if err != nil {
		return nil, err
	}
	freeSlots := (len(nodes) * a.rule.MaxContainerCount) - totalCount
	reasonMsg := fmt.Sprintf("number of free slots is %d", freeSlots)
	scaledMaxCount := int(float32(a.rule.MaxContainerCount) * a.rule.ScaleDownRatio)
	if freeSlots > scaledMaxCount {
		toRemoveCount := freeSlots / scaledMaxCount
		chosenNodes := chooseNodeForRemoval(nodes, toRemoveCount)
		if len(chosenNodes) == 0 {
			a.logDebug("would remove any node but can't due to metadata restrictions")
			return &ScalerResult{}, nil
		}
		return &ScalerResult{
			ToRemove: nodesToSpec(chosenNodes),
			Reason:   reasonMsg,
		}, nil
	}
	if freeSlots >= 0 {
		return &ScalerResult{}, nil
	}
	nodesToAdd := -freeSlots / a.rule.MaxContainerCount
	if freeSlots%a.rule.MaxContainerCount != 0 {
		nodesToAdd++
	}
	return &ScalerResult{
		ToAdd:  nodesToAdd,
		Reason: reasonMsg,
	}, nil
}
