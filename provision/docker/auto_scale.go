// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"fmt"
	"io"
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

var errAutoScaleRunning = errors.New("autoscale already running")

const (
	scaleActionAdd       = "add"
	scaleActionRemove    = "remove"
	scaleActionRebalance = "rebalance"
)

type autoScaleEvent struct {
	ID            interface{} `bson:"_id"`
	MetadataValue string
	Action        string // scaleActionAdd, scaleActionRemove, scaleActionRebalance
	Reason        string // dependend on scaler
	StartTime     time.Time
	EndTime       time.Time `bson:",omitempty"`
	Successful    bool
	Error         string       `bson:",omitempty"`
	Node          cluster.Node `bson:",omitempty"`
	Log           string       `bson:",omitempty"`
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

func newAutoScaleEvent(metadataValue string) (*autoScaleEvent, error) {
	// Use metadataValue as ID to ensure only one auto scale process runs for
	// each metadataValue. (*autoScaleEvent).finish() will generate a new
	// unique ID and remove this initial record.
	evt := autoScaleEvent{
		ID:            metadataValue,
		StartTime:     time.Now().UTC(),
		MetadataValue: metadataValue,
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

func (evt *autoScaleEvent) updateNode(node *cluster.Node) {
	evt.Node = *node
}

func (evt *autoScaleEvent) update(action, reason string) error {
	evt.Action = action
	evt.Reason = reason
	coll, err := autoScaleCollection()
	if err != nil {
		return err
	}
	return coll.UpdateId(evt.ID, evt)
}

func (evt *autoScaleEvent) finish(errParam error, log string) error {
	coll, err := autoScaleCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	if evt.Action == "" {
		return coll.RemoveId(evt.ID)
	}
	if errParam != nil {
		evt.Error = errParam.Error()
	}
	evt.Log = log
	evt.Successful = errParam == nil
	evt.EndTime = time.Now().UTC()
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
	done                chan bool
	scaleDownRatio      float32
	waitTimeNewMachine  time.Duration
	runInterval         time.Duration
	preventRebalance    bool
	writer              io.Writer
	logBuffer           safe.Buffer
}

type autoScaler interface {
	scale(event *autoScaleEvent, groupMetadata string, nodes []*cluster.Node) error
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

func (a *autoScaleConfig) setUpScaler() (autoScaler, error) {
	var scaler autoScaler
	if a.maxContainerCount > 0 {
		scaler = &countScaler{a}
	} else if a.totalMemoryMetadata != "" && a.maxMemoryRatio != 0 {
		scaler = &memoryScaler{a}
	} else {
		err := fmt.Errorf("[node autoscale] aborting node auto scale, either memory information or max container count must be informed in config")
		a.logError(err.Error())
		return nil, err
	}
	if a.scaleDownRatio == 0.0 {
		a.scaleDownRatio = 1.333
	} else if a.scaleDownRatio <= 1.0 {
		err := fmt.Errorf("[node autoscale] scale down ratio needs to be greater than 1.0, got %f", a.scaleDownRatio)
		a.logError(err.Error())
		return nil, err
	}
	if a.runInterval == 0 {
		a.runInterval = time.Hour
	}
	if a.waitTimeNewMachine == 0 {
		a.waitTimeNewMachine = 5 * time.Minute
	}
	writers := []io.Writer{&a.logBuffer}
	if a.writer != nil {
		writers = append(writers, a.writer)
	}
	a.writer = io.MultiWriter(writers...)
	return scaler, nil
}

func (a *autoScaleConfig) run() error {
	scaler, err := a.setUpScaler()
	if err != nil {
		return err
	}
	for {
		err := a.runScaler(scaler)
		if err != nil {
			err = fmt.Errorf("[node autoscale] %s", err.Error())
			a.logError(err.Error())
		}
		select {
		case <-a.done:
			return err
		case <-time.After(a.runInterval):
		}
	}
}

func (a *autoScaleConfig) logError(msg string, params ...interface{}) {
	msg = fmt.Sprintf("[node autoscale] %s", msg)
	if a.writer != nil {
		fmt.Fprintf(a.writer, fmt.Sprintf("ERROR:%s\n", msg), params...)
	}
	log.Errorf(msg, params...)
}

func (a *autoScaleConfig) logDebug(msg string, params ...interface{}) {
	msg = fmt.Sprintf("[node autoscale] %s", msg)
	if a.writer != nil {
		fmt.Fprintf(a.writer, msg+"\n", params...)
	}
	log.Debugf(msg, params...)
}

func (a *autoScaleConfig) runOnce() error {
	scaler, err := a.setUpScaler()
	if err != nil {
		return err
	}
	err = a.runScaler(scaler)
	if err != nil {
		a.logError(err.Error())
	}
	return err
}

func (a *autoScaleConfig) stop() {
	a.done <- true
}

func (a *autoScaleConfig) Shutdown() {
	a.stop()
}

func (a *autoScaleConfig) String() string {
	return "node auto scale"
}

func (a *autoScaleConfig) runScaler(scaler autoScaler) (retErr error) {
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
			a.logDebug("skipped node %s, no metadata value for %s.", node.Address, a.groupByMetadata)
			continue
		}
		if a.matadataFilter != "" && a.matadataFilter != groupMetadata {
			continue
		}
		clusterMap[groupMetadata] = append(clusterMap[groupMetadata], node)
	}
	for groupMetadata, nodes := range clusterMap {
		a.logBuffer.Reset()
		event, err := newAutoScaleEvent(groupMetadata)
		if err != nil {
			if err == errAutoScaleRunning {
				a.logDebug("skipping already running for: %s", groupMetadata)
				continue
			}
			retErr = fmt.Errorf("error creating scale event %s: %s", groupMetadata, err.Error())
			return
		}
		a.logDebug("running scaler %T for %q: %q", scaler, a.groupByMetadata, groupMetadata)
		err = scaler.scale(event, groupMetadata, nodes)
		if err != nil {
			event.finish(err, a.logBuffer.String())
			retErr = fmt.Errorf("error scaling group %s: %s", groupMetadata, err.Error())
			return
		}
		err = a.rebalanceIfNeeded(event, groupMetadata, nodes)
		if err != nil {
			a.logError("unable to rebalance: %s", err.Error())
		}
		if event.Action == "" {
			a.logDebug("nothing to do for %q: %q", a.groupByMetadata, groupMetadata)
		}
		event.finish(nil, a.logBuffer.String())
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
	containersMap, err := prov.runningContainersByNode(nodes)
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		totalMemory, _ := strconv.ParseFloat(node.Metadata[a.totalMemoryMetadata], 64)
		if totalMemory == 0.0 {
			return nil, fmt.Errorf("no value found for memory metadata (%s) in node %s", a.totalMemoryMetadata, node.Address)
		}
		maxMemory := int64(float64(a.maxMemoryRatio) * totalMemory)
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

// Creates a dry provisioner and try provisioning existing containers without
// each one of existing nodes.
//
// If it's possible to distribute containers and we still have spare memory
// such node can be removed.
func (a *memoryScaler) choseNodeForRemoval(maxPlanMemory int64, groupMetadata string, nodes []*cluster.Node) (*cluster.Node, error) {
	containersMap, err := a.provisioner.runningContainersByNode(nodes)
	if err != nil {
		return nil, err
	}
	var containers []container
	for _, node := range nodes {
		containers = append(containers, containersMap[node.Address]...)
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
			a.logError("unable to dry rebalance containers without %s: %s - log: %s", node.Address, err, buf.String())
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
		canRemove, _ := canRemoveNode(chosenNode, nodes)
		if !canRemove {
			a.logDebug("would remove node %s but can't due to metadata restrictions", chosenNode.Address)
			return nil, nil
		}
		return chosenNode, nil
	}
	return nil, nil
}

func (a *memoryScaler) scale(event *autoScaleEvent, groupMetadata string, nodes []*cluster.Node) error {
	plans, err := app.PlansList()
	if err != nil {
		return fmt.Errorf("couldn't list plans: %s", err)
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
			return fmt.Errorf("couldn't get default plan: %s", err)
		}
		maxPlanMemory = defaultPlan.Memory
	}
	chosenNode, err := a.choseNodeForRemoval(maxPlanMemory, groupMetadata, nodes)
	if err != nil {
		return fmt.Errorf("unable to choose node for removal: %s", err)
	}
	if chosenNode != nil {
		event.updateNode(chosenNode)
		err = event.update(scaleActionRemove, fmt.Sprintf("containers from %s can be distributed in cluster", chosenNode.Address))
		if err != nil {
			return err
		}
		a.logDebug("running event %q for %q: %s", event.Action, event.MetadataValue, event.Reason)
		return a.removeNode(chosenNode)
	}
	memoryData, err := a.nodesMemoryData(a.provisioner, nodes)
	if err != nil {
		return err
	}
	canFitMax := false
	for _, node := range nodes {
		data := memoryData[node.Address]
		a.logDebug("node %s, memory data: %#v", node.Address, data)
		if maxPlanMemory > data.maxMemory {
			return fmt.Errorf("aborting, impossible to fit max plan memory of %d bytes, node max available memory is %d", maxPlanMemory, data.maxMemory)
		}
		if data.available >= maxPlanMemory {
			canFitMax = true
			break
		}
	}
	if canFitMax {
		return nil
	}
	err = event.update(scaleActionAdd, fmt.Sprintf("can't add %d bytes to an existing node", maxPlanMemory))
	if err != nil {
		return err
	}
	a.logDebug("running event %q for %q: %s", event.Action, event.MetadataValue, event.Reason)
	newNode, err := a.addNode(nodes)
	if err != nil {
		return err
	}
	event.updateNode(newNode)
	return nil
}

func (a *countScaler) scale(event *autoScaleEvent, groupMetadata string, nodes []*cluster.Node) error {
	totalCount, _, err := a.provisioner.containerGapInNodes(nodes)
	if err != nil {
		return fmt.Errorf("couldn't find containers from nodes: %s", err)
	}
	freeSlots := (len(nodes) * a.maxContainerCount) - totalCount
	reasonMsg := fmt.Sprintf("number of free slots is %d", freeSlots)
	if freeSlots > int(float32(a.maxContainerCount)*a.scaleDownRatio) {
		var chosenNode *cluster.Node
		for _, node := range nodes {
			canRemove, _ := canRemoveNode(node, nodes)
			if canRemove {
				chosenNode = node
				break
			}
		}
		if chosenNode == nil {
			a.logDebug("would remove any node but can't due to metadata restrictions")
			return nil
		}
		event.updateNode(chosenNode)
		err := event.update(scaleActionRemove, reasonMsg)
		if err != nil {
			return fmt.Errorf("error updating event: %s", err)
		}
		a.logDebug("running event %q for %q: %s", event.Action, event.MetadataValue, event.Reason)
		return a.removeNode(chosenNode)
	}
	if freeSlots >= 0 {
		return nil
	}
	err = event.update(scaleActionAdd, reasonMsg)
	if err != nil {
		return fmt.Errorf("error updating event: %s", err)
	}
	a.logDebug("running event %q for %q: %s", event.Action, event.MetadataValue, event.Reason)
	newNode, err := a.addNode(nodes)
	if err != nil {
		return err
	}
	event.updateNode(newNode)
	return nil
}

func (a *autoScaleConfig) rebalanceIfNeeded(event *autoScaleEvent, groupMetadata string, nodes []*cluster.Node) error {
	if a.preventRebalance {
		return nil
	}
	var rebalanceFilter map[string]string
	if a.groupByMetadata != "" {
		rebalanceFilter = map[string]string{a.groupByMetadata: groupMetadata}
	}
	if event.Action == "" {
		// No action yet, check if we need rebalance
		_, gap, err := a.provisioner.containerGapInNodes(nodes)
		buf := safe.NewBuffer(nil)
		dryProvisioner, err := a.provisioner.rebalanceContainersByFilter(buf, nil, rebalanceFilter, true)
		if err != nil {
			return fmt.Errorf("unable to run dry rebalance to check if rebalance is needed: %s - log: %s", err, buf.String())
		}
		if dryProvisioner == nil {
			return nil
		}
		_, gapAfter, err := dryProvisioner.containerGapInNodes(nodes)
		if err != nil {
			return fmt.Errorf("couldn't find containers from rebalanced nodes: %s", err)
		}
		if math.Abs((float64)(gap-gapAfter)) > 2.0 {
			err = event.update(scaleActionRebalance, fmt.Sprintf("gap is %d, after rebalance gap will be %d", gap, gapAfter))
			if err != nil {
				return fmt.Errorf("unable to update event: %s", err)
			}
		}
	}
	if event.Action != "" && event.Action != scaleActionRemove {
		a.logDebug("running rebalance, due to %q for %q: %s", event.Action, event.MetadataValue, event.Reason)
		buf := safe.NewBuffer(nil)
		var writer io.Writer = buf
		if a.writer != nil {
			writer = io.MultiWriter(buf, a.writer)
		}
		_, err := a.provisioner.rebalanceContainersByFilter(writer, nil, rebalanceFilter, false)
		if err != nil {
			return fmt.Errorf("unable to rebalance containers: %s - log: %s", err.Error(), buf.String())
		}
		return nil
	}
	return nil
}

func (a *autoScaleConfig) addNode(modelNodes []*cluster.Node) (*cluster.Node, error) {
	metadata, err := chooseMetadataFromNodes(modelNodes)
	if err != nil {
		return nil, err
	}
	_, hasIaas := metadata["iaas"]
	if !hasIaas {
		return nil, fmt.Errorf("no IaaS information in nodes metadata: %#v", metadata)
	}
	machine, err := iaas.CreateMachineForIaaS(metadata["iaas"], metadata)
	if err != nil {
		return nil, fmt.Errorf("unable to create machine: %s", err.Error())
	}
	newAddr := machine.FormatNodeAddress()
	a.logDebug("new machine created: %s - Waiting for docker to start...", newAddr)
	createdNode, err := a.provisioner.getCluster().WaitAndRegister(newAddr, metadata, a.waitTimeNewMachine)
	if err != nil {
		machine.Destroy()
		return nil, fmt.Errorf("error registering new node %s: %s", newAddr, err.Error())
	}
	a.logDebug("new machine created: %s - started!", newAddr)
	return &createdNode, nil
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
	m, err := iaas.FindMachineByIdOrAddress(chosenNode.Metadata["iaas-id"], urlToHost(chosenNode.Address))
	if err != nil {
		a.logError("unable to find machine for removal in iaas: %s", err)
		return nil
	}
	err = m.Destroy()
	if err != nil {
		a.logError("unable to destroy machine in IaaS: %s", err)
	}
	return nil
}

func canRemoveNode(chosenNode *cluster.Node, nodes []*cluster.Node) (bool, error) {
	exclusiveList, _, err := splitMetadata(createMetadataList(nodes))
	if err != nil {
		return false, err
	}
	if len(exclusiveList) == 0 {
		return true, nil
	}
	hasMetadata := func(n *cluster.Node, meta map[string]string) bool {
		for k, v := range meta {
			if n.Metadata[k] != v {
				return false
			}
		}
		return true
	}
	for _, item := range exclusiveList {
		if hasMetadata(chosenNode, item.metadata) {
			if item.freq > 1 {
				return true, nil
			}
			return false, nil
		}
	}
	return false, nil
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
	exclusiveList, baseMetadata, err := splitMetadata(createMetadataList(modelNodes))
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

func (p *dockerProvisioner) runningContainersByNode(nodes []*cluster.Node) (map[string][]container, error) {
	appNames, err := p.listAppsForNodes(nodes)
	if err != nil {
		return nil, err
	}
	for _, appName := range appNames {
		locked, err := app.AcquireApplicationLock(appName, app.InternalAppName, "node auto scale")
		if err != nil {
			return nil, err
		}
		if !locked {
			return nil, fmt.Errorf("unable to lock app %s, aborting", appName)
		}
		defer app.ReleaseApplicationLock(appName)
	}
	result := map[string][]container{}
	for _, n := range nodes {
		nodeConts, err := p.listRunningContainersByHost(urlToHost(n.Address))
		if err != nil {
			return nil, err
		}
		result[n.Address] = nodeConts
	}
	return result, nil
}

func (p *dockerProvisioner) containerGapInNodes(nodes []*cluster.Node) (int, int, error) {
	maxCount := 0
	minCount := 0
	totalCount := 0
	containersMap, err := p.runningContainersByNode(nodes)
	if err != nil {
		return 0, 0, err
	}
	for _, containers := range containersMap {
		contCount := len(containers)
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

func createMetadataList(nodes []*cluster.Node) []map[string]string {
	// iaas-id is ignored because it wasn't created in previous tsuru versions
	// and having nodes with and without it would cause unbalanced metadata
	// errors.
	ignoredMetadata := []string{"iaas-id"}
	metadataList := make([]map[string]string, len(nodes))
	for i, n := range nodes {
		metadata := n.CleanMetadata()
		for _, val := range ignoredMetadata {
			delete(metadata, val)
		}
		metadataList[i] = metadata
	}
	return metadataList
}
