// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/safe"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type autoScaleEvent struct {
	ID            interface{} `bson:"_id"`
	MetadataValue string
	Action        string // "rebalance", "add" or "remove"
	Reason        string // dependend on scaler
	StartTime     time.Time
	EndTime       time.Time `bson:",omitempty"`
	Successful    bool
	Error         string `bson:",omitempty"`
}

func autoScaleCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	name, err := config.GetString("docker:collection")
	if err != nil {
		return nil, err
	}
	return conn.Collection(fmt.Sprintf("%s_auto_scale", name)), nil
}

var errAutoScaleRunning = errors.New("autoscale already running")

func newAutoScaleEvent(metadataValue, action, reason string) (*autoScaleEvent, error) {
	// Use metadataValue as ID to ensure only one auto scale process runs for
	// each metadataValue. (*autoScaleEvent).update() will generate a new
	// unique ID and remove this initial record.
	evt := autoScaleEvent{
		ID:            metadataValue,
		StartTime:     time.Now().UTC(),
		MetadataValue: metadataValue,
		Action:        action,
		Reason:        reason,
	}
	coll, err := autoScaleCollection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	err = coll.Insert(evt)
	if mgo.IsDup(err) {
		return nil, errAutoScaleRunning
	}
	return &evt, err
}

func (evt *autoScaleEvent) update(err error) error {
	if err != nil {
		evt.Error = err.Error()
	}
	evt.Successful = err == nil
	evt.EndTime = time.Now().UTC()
	coll, err := autoScaleCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	defer coll.RemoveId(evt.ID)
	evt.ID = bson.NewObjectId()
	return coll.Insert(evt)
}

func listAutoScaleEvents(skip, limit int) ([]autoScaleEvent, error) {
	coll, err := autoScaleCollection()
	if err != nil {
		return nil, err
	}
	query := coll.Find(nil).Sort("-_id")
	if skip != 0 {
		query = query.Skip(skip)
	}
	if limit != 0 {
		query = query.Limit(limit)
	}
	var list []autoScaleEvent
	err = query.All(&list)
	if err != nil {
		return nil, err
	}
	return list, nil
}

type autoScaleConfig struct {
	provisioner         *dockerProvisioner
	matadataFilter      string
	groupByMetadata     string
	totalMemoryMetadata string
	maxMemoryRatio      float32
	maxContainerCount   int
	scaleDownRatio      float32
	waitTimeNewMachine  time.Duration
	runInterval         time.Duration
	done                chan bool
}

type autoScaler interface {
	scale(groupMetadata string, nodes []*cluster.Node) (*autoScaleEvent, error)
}

type memoryScaler struct {
	*autoScaleConfig
}

type countScaler struct {
	*autoScaleConfig
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
	var scaler autoScaler
	if a.maxContainerCount > 0 {
		scaler = &countScaler{a}
	} else if a.totalMemoryMetadata != "" && a.maxMemoryRatio != 0 {
		scaler = &memoryScaler{a}
	} else {
		err := fmt.Errorf("[node autoscale] aborting node auto scale, either memory information or max container count must be informed in config")
		log.Error(err.Error())
		return err
	}
	if a.scaleDownRatio == 0.0 {
		a.scaleDownRatio = 1.333
	} else if a.scaleDownRatio < 1.0 {
		err := fmt.Errorf("[node autoscale] scale down ratio needs to be greater than 1.0, got %f", a.scaleDownRatio)
		log.Error(err.Error())
		return err
	}
	if a.runInterval < time.Minute {
		a.runInterval = time.Minute
	}
	if a.waitTimeNewMachine < time.Minute {
		a.waitTimeNewMachine = time.Minute
	}
	for {
		err := a.runOnce(scaler)
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

func (a *autoScaleConfig) runOnce(scaler autoScaler) (retErr error) {
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
		var rebalanceFilter map[string]string
		if a.groupByMetadata != "" {
			rebalanceFilter = map[string]string{a.groupByMetadata: groupMetadata}
		}
		event, err := scaler.scale(groupMetadata, nodes)
		if err != nil {
			if err == errAutoScaleRunning {
				log.Debugf("[node autoscale] skipping already running for: %s", groupMetadata)
				continue
			}
			retErr = fmt.Errorf("error scaling group %s: %s", groupMetadata, err.Error())
			return
		}
		if event == nil {
			event, err = a.createRebalanceEvent(groupMetadata, nodes, rebalanceFilter)
			if err != nil {
				log.Errorf("[node autoscale] unable to create rebalance event: %s", err.Error())
			}
		}
		if event != nil && event.Action != "remove" {
			buf := safe.NewBuffer(nil)
			_, err := a.provisioner.rebalanceContainersByFilter(buf, nil, rebalanceFilter, false)
			if err != nil {
				log.Errorf("[node autoscale] unable to rebalance containers: %s - log: %s", err.Error(), buf.String())
			}
		}
		if event != nil {
			event.update(err)
		}
	}
	return
}

type nodeMemoryData struct {
	node             *cluster.Node
	maxMemory        int64
	reserved         int64
	available        int64
	containersMemory map[string]int64
}

func (a *memoryScaler) nodesMemoryData(prov *dockerProvisioner, nodes []*cluster.Node) (map[string]*nodeMemoryData, error) {
	nodesMemoryData := make(map[string]*nodeMemoryData)
	for _, node := range nodes {
		totalMemory, _ := strconv.ParseFloat(node.Metadata[a.totalMemoryMetadata], 64)
		maxMemory := int64(float64(a.maxMemoryRatio) * totalMemory)
		data := &nodeMemoryData{
			containersMemory: make(map[string]int64),
			node:             node,
			maxMemory:        maxMemory,
		}
		nodesMemoryData[node.Address] = data
		containers, err := prov.listRunningContainersByHost(urlToHost(node.Address))
		if err != nil {
			return nil, fmt.Errorf("couldn't find containers: %s", err)
		}
		for _, cont := range containers {
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

func (a *memoryScaler) scale(groupMetadata string, nodes []*cluster.Node) (*autoScaleEvent, error) {
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
		defaultPlan, err := app.DefaultPlan()
		if err != nil {
			return nil, fmt.Errorf("couldn't get default plan: %s", err)
		}
		maxPlanMemory = defaultPlan.Memory
	}
	var containers []container
	for _, node := range nodes {
		conts, err := a.provisioner.listRunningContainersByHost(urlToHost(node.Address))
		if err != nil {
			return nil, err
		}
		containers = append(containers, conts...)
	}
	var maxAvailable int64
	var chosenNode *cluster.Node
	for _, node := range nodes {
		dryProv, err := a.provisioner.dryMode(nil)
		if err != nil {
			return nil, err
		}
		defer dryProv.stopDryMode()
		err = dryProv.getCluster().Unregister(node.Address)
		if err != nil {
			return nil, err
		}
		buf := safe.NewBuffer(nil)
		err = dryProv.moveContainerList(containers, "", buf)
		if err != nil {
			log.Errorf("[node autoscale] unable to rebalance containers without %s: %s - log: %s", node.Address, err, buf.String())
			continue
		}
		otherNodes, err := dryProv.getCluster().NodesForMetadata(map[string]string{a.groupByMetadata: groupMetadata})
		if err != nil {
			return nil, err
		}
		otherNodesPtr := make([]*cluster.Node, len(otherNodes))
		for i := range otherNodes {
			otherNodesPtr[i] = &otherNodes[i]
		}
		data, err := a.nodesMemoryData(dryProv, otherNodesPtr)
		if err != nil {
			return nil, err
		}
		var maxLocalAvailable int64
		for _, v := range data {
			if v.available > maxLocalAvailable {
				maxLocalAvailable = v.available
			}
		}
		if maxLocalAvailable > maxAvailable {
			maxAvailable = maxLocalAvailable
			chosenNode = node
		}
	}
	if chosenNode != nil && maxAvailable > int64(float32(maxPlanMemory)*a.scaleDownRatio) {
		event, err := newAutoScaleEvent(groupMetadata, "remove", fmt.Sprintf("containers from %s can be distributed in cluster", chosenNode.Address))
		if err != nil {
			return nil, err
		}
		err = a.removeNode(chosenNode)
		return event, err
	}
	memoryData, err := a.nodesMemoryData(a.provisioner, nodes)
	if err != nil {
		return nil, err
	}
	canFitMax := false
	for _, node := range nodes {
		data := memoryData[node.Address]
		if maxPlanMemory > data.maxMemory {
			return nil, fmt.Errorf("aborting, impossible to fit max plan memory of %d bytes, node max available memory is %d", maxPlanMemory, data.maxMemory)
		}
		if data.available >= maxPlanMemory {
			canFitMax = true
			break
		}
	}
	if canFitMax {
		return nil, nil
	}
	event, err := newAutoScaleEvent(groupMetadata, "add", fmt.Sprintf("can't add %d bytes to an existing node", maxPlanMemory))
	if err != nil {
		return nil, err
	}
	log.Debugf("[node autoscale] adding a new machine, metadata value: %s, didn't have %d bytes available", groupMetadata, maxPlanMemory)
	err = a.addNode(nodes)
	if err != nil {
		event.update(err)
		return nil, err
	}
	return event, nil
}

func (a *countScaler) scale(groupMetadata string, nodes []*cluster.Node) (*autoScaleEvent, error) {
	totalCount, _, err := a.provisioner.containerGapInNodes(nodes)
	if err != nil {
		return nil, fmt.Errorf("couldn't find containers from nodes: %s", err)
	}
	freeSlots := (len(nodes) * a.maxContainerCount) - totalCount
	baseReasonMsg := "number of free slots is %d"
	if freeSlots > int(float32(a.maxContainerCount)*a.scaleDownRatio) {
		event, err := newAutoScaleEvent(groupMetadata, "remove", fmt.Sprintf(baseReasonMsg, freeSlots))
		if err != nil {
			return nil, err
		}
		minCount := math.MaxInt32
		var chosenNode *cluster.Node
		for _, node := range nodes {
			contCount, err := a.provisioner.countRunningContainersByHost(urlToHost(node.Address))
			if err != nil {
				return nil, fmt.Errorf("unable to count running containers: %s", err)
			}
			if contCount < minCount {
				minCount = contCount
				chosenNode = node
			}
		}
		err = a.removeNode(chosenNode)
		return event, err
	}
	if freeSlots >= 0 {
		return nil, nil
	}
	event, err := newAutoScaleEvent(groupMetadata, "add", fmt.Sprintf(baseReasonMsg, freeSlots))
	if err != nil {
		return nil, err
	}
	log.Debugf("[node autoscale] adding a new machine, metadata value: %s, free slots: %d", groupMetadata, freeSlots)
	err = a.addNode(nodes)
	if err != nil {
		event.update(err)
		return nil, err
	}
	return event, nil
}

func (a *autoScaleConfig) createRebalanceEvent(groupMetadata string, nodes []*cluster.Node, rebalanceFilter map[string]string) (*autoScaleEvent, error) {
	_, gap, err := a.provisioner.containerGapInNodes(nodes)
	buf := safe.NewBuffer(nil)
	dryProvisioner, err := a.provisioner.rebalanceContainersByFilter(buf, nil, rebalanceFilter, true)
	if err != nil {
		return nil, fmt.Errorf("unable to run dry rebalance to check if rebalance is needed: %s - log: %s", err, buf.String())
	}
	_, gapAfter, err := dryProvisioner.containerGapInNodes(nodes)
	if err != nil {
		return nil, fmt.Errorf("couldn't find containers from rebalanced nodes: %s", err)
	}
	if math.Abs((float64)(gap-gapAfter)) <= 2.0 {
		return nil, nil
	}
	log.Debugf("[node autoscale] running rebalance, metadata value: %s, gap before: %d, gap after: %d", groupMetadata, gap, gapAfter)
	event, err := newAutoScaleEvent(groupMetadata, "rebalance", fmt.Sprintf("gap is %d, after rebalance gap will be %d", gap, gapAfter))
	if err != nil {
		if err == errAutoScaleRunning {
			log.Debugf("[node autoscale] skipping already running for: %s", groupMetadata)
			return nil, nil
		}
		return nil, fmt.Errorf("unable to create auto scale event: %s", err)
	}
	return event, nil
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
	log.Debugf("[node autoscale] new machine created: %s - Waiting for docker to start...", newAddr)
	_, err = a.provisioner.getCluster().WaitAndRegister(newAddr, metadata, a.waitTimeNewMachine)
	if err != nil {
		machine.Destroy()
		return fmt.Errorf("error registering new node %s: %s", newAddr, err.Error())
	}
	log.Debugf("[node autoscale] new machine created: %s - started!", newAddr)
	return nil
}

func (a *autoScaleConfig) removeNode(chosenNode *cluster.Node) error {
	_, hasIaas := chosenNode.Metadata["iaas"]
	if !hasIaas {
		return fmt.Errorf("no IaaS information in node (%s) metadata: %#v", chosenNode.Address, chosenNode.Metadata)
	}
	err := a.provisioner.getCluster().Unregister(chosenNode.Address)
	if err != nil {
		return fmt.Errorf("unable to unregister node (%s) for removal: %s", chosenNode.Address, err)
	}
	buf := safe.NewBuffer(nil)
	err = a.provisioner.moveContainers(urlToHost(chosenNode.Address), "", buf)
	if err != nil {
		a.provisioner.getCluster().Register(chosenNode.Address, chosenNode.Metadata)
		return fmt.Errorf("unable to move containers from node (%s): %s - log: %s", chosenNode.Address, err, buf.String())
	}
	m, err := iaas.FindMachineByAddress(urlToHost(chosenNode.Address))
	if err != nil {
		log.Errorf("unable to find machine for removal in iaas: %s", err)
		return nil
	}
	err = m.Destroy()
	if err != nil {
		log.Errorf("unable to destroy machine in IaaS: %s", err)
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

func (p *dockerProvisioner) containerGapInNodes(nodes []*cluster.Node) (int, int, error) {
	maxCount := 0
	minCount := 0
	totalCount := 0
	for _, n := range nodes {
		contCount, err := p.countRunningContainersByHost(urlToHost(n.Address))
		if err != nil {
			return 0, 0, err
		}
		if contCount > maxCount {
			maxCount = contCount
		}
		if minCount == 0 || contCount < minCount {
			minCount = contCount
		}
		totalCount += contCount
	}
	return totalCount, maxCount - minCount, nil
}
