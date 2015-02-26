// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
)

type autoScaleConfig struct {
	provisioner         *dockerProvisioner
	matadataFilter      string
	groupByMetadata     string
	totalMemoryMetadata string
	maxMemoryRatio      float32
	maxContainerCount   int
	waitTimeNewMachine  time.Duration
	runInterval         time.Duration
	done                chan bool
}

type metaWithFrequency struct {
	metadata map[string]string
	freq     int
}

type metaWithFrequencyList []metaWithFrequency

func (l metaWithFrequencyList) Len() int           { return len(l) }
func (l metaWithFrequencyList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l metaWithFrequencyList) Less(i, j int) bool { return l[i].freq < l[j].freq }

func (a *autoScaleConfig) run() error {
	isMemoryBased := a.totalMemoryMetadata != "" && a.maxMemoryRatio != 0
	if !isMemoryBased && a.maxContainerCount == 0 {
		err := fmt.Errorf("[node autoscale] aborting node auto scale, either memory information or max container count must be informed in config")
		log.Error(err.Error())
		return err
	}
	oneMinute := 1 * time.Minute
	if a.runInterval < oneMinute {
		a.runInterval = oneMinute
	}
	if a.waitTimeNewMachine < oneMinute {
		a.waitTimeNewMachine = oneMinute
	}
	for {
		err := a.runOnce(isMemoryBased)
		if err != nil {
			err = fmt.Errorf("[node autoscale] %s", err.Error())
			log.Error(err.Error())
		}
		select {
		case <-a.done:
			return err
		case <-time.After(a.runInterval):
		}
	}
}

func (a *autoScaleConfig) stop() {
	a.done <- true
}

func (a *autoScaleConfig) runOnce(isMemoryBased bool) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("recovered panic, we can never stop! panic: %v", r)
		}
	}()
	nodes, err := a.provisioner.getCluster().Nodes()
	if err != nil {
		retErr = fmt.Errorf("error getting nodes: %s", err.Error())
		return
	}
	clusterMap := map[string][]*cluster.Node{}
	for i := range nodes {
		node := &nodes[i]
		if a.groupByMetadata == "" {
			clusterMap[""] = append(clusterMap[""], node)
			continue
		}
		groupMetadata := node.Metadata[a.groupByMetadata]
		if groupMetadata == "" {
			log.Debugf("[node autoscale] skipped node %s, no metadata value for %s.", node.Address, a.groupByMetadata)
			continue
		}
		if a.matadataFilter != "" && a.matadataFilter != groupMetadata {
			continue
		}
		clusterMap[groupMetadata] = append(clusterMap[groupMetadata], node)
	}
	for groupMetadata, nodes := range clusterMap {
		if !isMemoryBased {
			err = a.scaleGroupByCount(groupMetadata, nodes)
			if err != nil {
				retErr = fmt.Errorf("error scaling group %s: %s", groupMetadata, err.Error())
				return
			}
		}
	}
	return
}

func (a *autoScaleConfig) scaleGroupByCount(groupMetadata string, nodes []*cluster.Node) error {
	freeSlots := 0
	maxCount := 0
	minCount := 0
	for _, n := range nodes {
		contCount, err := a.provisioner.countContainersByHost(urlToHost(n.Address))
		if err != nil {
			return err
		}
		if contCount > maxCount {
			maxCount = contCount
		}
		if minCount == 0 || contCount < minCount {
			minCount = contCount
		}
		freeSlots += a.maxContainerCount - contCount
	}
	if freeSlots < 0 {
		minCount = 0
		err := a.addNode(nodes)
		if err != nil {
			return err
		}
	}
	gap := maxCount - minCount
	if gap >= 2 {
		var buf bytes.Buffer
		var rebalanceFilter map[string]string
		if a.groupByMetadata != "" {
			rebalanceFilter = map[string]string{a.groupByMetadata: groupMetadata}
		}
		err := a.provisioner.rebalanceContainersByFilter(&buf, nil, rebalanceFilter, false)
		if err != nil {
			log.Errorf("Unable to rebalance containers: %s - log: %s", err.Error(), buf.String())
		}
	}
	return nil
}

func splitMetadata(nodesMetadata []map[string]string) (metaWithFrequencyList, map[string]string, error) {
	common := make(map[string]string)
	exclusive := make([]map[string]string, len(nodesMetadata))
	for i := range nodesMetadata {
		metadata := nodesMetadata[i]
		for k, v := range metadata {
			isExclusive := false
			for j := range nodesMetadata {
				if i == j {
					continue
				}
				otherMetadata := nodesMetadata[j]
				if v != otherMetadata[k] {
					isExclusive = true
					break
				}
			}
			if isExclusive {
				if exclusive[i] == nil {
					exclusive[i] = make(map[string]string)
				}
				exclusive[i][k] = v
			} else {
				common[k] = v
			}
		}
	}
	var group metaWithFrequencyList
	sameMap := make(map[int]bool)
	for i := range exclusive {
		freq := 1
		for j := range exclusive {
			if i == j {
				continue
			}
			diffCount := 0
			for k, v := range exclusive[i] {
				if exclusive[j][k] != v {
					diffCount++
				}
			}
			if diffCount > 0 && diffCount < len(exclusive[i]) {
				return nil, nil, fmt.Errorf("unbalanced metadata for node group: %v vs %v", exclusive[i], exclusive[j])
			}
			if diffCount == 0 {
				sameMap[j] = true
				freq++
			}
		}
		if !sameMap[i] && exclusive[i] != nil {
			group = append(group, metaWithFrequency{metadata: exclusive[i], freq: freq})
		}
	}
	return group, common, nil
}

func chooseMetadataFromNodes(modelNodes []*cluster.Node) (map[string]string, error) {
	metadataList := make([]map[string]string, len(modelNodes))
	for i, n := range modelNodes {
		metadataList[i] = n.CleanMetadata()
	}
	exclusiveList, baseMetadata, err := splitMetadata(metadataList)
	if err != nil {
		return nil, err
	}
	var chosenExclusive map[string]string
	if exclusiveList != nil {
		sort.Sort(exclusiveList)
		chosenExclusive = exclusiveList[0].metadata
	}
	for k, v := range chosenExclusive {
		baseMetadata[k] = v
	}
	return baseMetadata, nil
}

func (a *autoScaleConfig) addNode(modelNodes []*cluster.Node) error {
	metadata, err := chooseMetadataFromNodes(modelNodes)
	if err != nil {
		return err
	}
	_, hasIaas := metadata["iaas"]
	if !hasIaas {
		return fmt.Errorf("no IaaS information in nodes metadata: %#v", metadata)
	}
	machine, err := iaas.CreateMachineForIaaS(metadata["iaas"], metadata)
	if err != nil {
		return fmt.Errorf("unable to create machine: %s", err.Error())
	}
	newAddr := machine.FormatNodeAddress()
	log.Debugf("New machine created during auto scaling: %s - Waiting for docker to start...", newAddr)
	_, err = a.provisioner.getCluster().WaitAndRegister(newAddr, metadata, a.waitTimeNewMachine)
	if err != nil {
		machine.Destroy()
		return fmt.Errorf("error registering new node %s: %s", newAddr, err.Error())
	}
	log.Debugf("New machine created during auto scaling: %s - started!", newAddr)
	return nil
}
