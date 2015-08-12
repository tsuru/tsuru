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
	"strings"
	"sync"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/queue"
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
	Nodes         []cluster.Node
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

func (evt *autoScaleEvent) updateNodes(nodes []cluster.Node) {
	evt.Nodes = nodes
}

func (evt *autoScaleEvent) update(action, reason string) error {
	evt.Action = action
	evt.Reason = reason
	coll, err := autoScaleCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
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
	defer coll.Close()
	query := coll.Find(nil).Sort("-starttime")
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
	for i := range list {
		if len(list[i].Nodes) == 0 {
			node := list[i].Node
			if node.Address != "" {
				list[i].Nodes = []cluster.Node{node}
			}
		}
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
		err = a.runScaler(scaler)
		if err != nil {
			a.logError(err.Error())
			err = fmt.Errorf("[node autoscale] %s", err.Error())
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
	nodes, err := a.provisioner.Cluster().Nodes()
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

func (a *memoryScaler) choseNodeForRemoval(maxPlanMemory int64, groupMetadata string, nodes []*cluster.Node) ([]cluster.Node, error) {
	memoryData, err := a.nodesMemoryData(a.provisioner, nodes)
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
	scaledMaxPlan := int64(float32(maxPlanMemory) * a.scaleDownRatio)
	toRemoveCount := len(nodes) - int(((totalReserved+scaledMaxPlan)/memPerNode)+1)
	if toRemoveCount <= 0 {
		return nil, nil
	}
	chosenNodes := chooseNodeForRemoval(nodes, toRemoveCount)
	if len(chosenNodes) == 0 {
		a.logDebug("would remove any node but can't due to metadata restrictions")
		return nil, nil
	}
	return chosenNodes, nil
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
	chosenNodes, err := a.choseNodeForRemoval(maxPlanMemory, groupMetadata, nodes)
	if err != nil {
		return fmt.Errorf("unable to choose node for removal: %s", err)
	}
	if chosenNodes != nil {
		event.updateNodes(chosenNodes)
		err = event.update(scaleActionRemove, fmt.Sprintf("containers can be distributed in only %d nodes", len(nodes)-len(chosenNodes)))
		if err != nil {
			return err
		}
		a.logDebug("running event %q for %q: %s", event.Action, event.MetadataValue, event.Reason)
		return a.removeMultipleNodes(chosenNodes)
	}
	memoryData, err := a.nodesMemoryData(a.provisioner, nodes)
	if err != nil {
		return err
	}
	canFitMax := false
	var totalReserved, totalMem int64
	for _, node := range nodes {
		data := memoryData[node.Address]
		a.logDebug("checking scale up, node %s, memory data: %#v", node.Address, data)
		if maxPlanMemory > data.maxMemory {
			return fmt.Errorf("aborting, impossible to fit max plan memory of %d bytes, node max available memory is %d", maxPlanMemory, data.maxMemory)
		}
		totalReserved += data.reserved
		totalMem += data.maxMemory
		if data.available >= maxPlanMemory {
			canFitMax = true
			break
		}
	}
	if canFitMax {
		return nil
	}
	nodesToAdd := int((totalReserved + maxPlanMemory) / totalMem)
	if nodesToAdd == 0 {
		return nil
	}
	err = event.update(scaleActionAdd, fmt.Sprintf("can't add %d bytes to an existing node, adding %d nodes", maxPlanMemory, nodesToAdd))
	if err != nil {
		return err
	}
	a.logDebug("running event %q for %q: %s", event.Action, event.MetadataValue, event.Reason)
	newNodes, err := a.addMultipleNodes(nodes, nodesToAdd)
	if err != nil {
		if len(newNodes) == 0 {
			return err
		}
		a.logError("not all required nodes were created: %s", err)
	}
	event.updateNodes(newNodes)
	return nil
}

func (a *countScaler) scale(event *autoScaleEvent, groupMetadata string, nodes []*cluster.Node) error {
	totalCount, _, err := a.provisioner.containerGapInNodes(nodes)
	if err != nil {
		return fmt.Errorf("couldn't find containers from nodes: %s", err)
	}
	freeSlots := (len(nodes) * a.maxContainerCount) - totalCount
	reasonMsg := fmt.Sprintf("number of free slots is %d", freeSlots)
	scaledMaxCount := int(float32(a.maxContainerCount) * a.scaleDownRatio)
	if freeSlots > scaledMaxCount {
		toRemoveCount := freeSlots / scaledMaxCount
		chosenNodes := chooseNodeForRemoval(nodes, toRemoveCount)
		if len(chosenNodes) == 0 {
			a.logDebug("would remove any node but can't due to metadata restrictions")
			return nil
		}
		event.updateNodes(chosenNodes)
		downMsg := fmt.Sprintf("%s, removing %d nodes", reasonMsg, len(chosenNodes))
		err := event.update(scaleActionRemove, downMsg)
		if err != nil {
			return fmt.Errorf("error updating event: %s", err)
		}
		a.logDebug("running event %q for %q: %s", event.Action, event.MetadataValue, event.Reason)
		return a.removeMultipleNodes(chosenNodes)
	}
	if freeSlots >= 0 {
		return nil
	}
	nodesToAdd := -freeSlots / a.maxContainerCount
	if nodesToAdd == 0 {
		nodesToAdd = 1
	}
	upMsg := fmt.Sprintf("%s, adding %d nodes", reasonMsg, nodesToAdd)
	err = event.update(scaleActionAdd, upMsg)
	if err != nil {
		return fmt.Errorf("error updating event: %s", err)
	}
	a.logDebug("running event %q for %q: %s", event.Action, event.MetadataValue, event.Reason)
	newNodes, err := a.addMultipleNodes(nodes, nodesToAdd)
	if err != nil {
		if len(newNodes) == 0 {
			return err
		}
		a.logError("not all required nodes were created: %s", err)
	}
	event.updateNodes(newNodes)
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

func (a *autoScaleConfig) addMultipleNodes(modelNodes []*cluster.Node, count int) ([]cluster.Node, error) {
	wg := sync.WaitGroup{}
	wg.Add(count)
	nodesCh := make(chan *cluster.Node, count)
	errCh := make(chan error, count)
	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			node, err := a.addNode(modelNodes)
			if err != nil {
				errCh <- err
				return
			}
			nodesCh <- node
		}()
	}
	wg.Wait()
	close(nodesCh)
	close(errCh)
	var nodes []cluster.Node
	for n := range nodesCh {
		nodes = append(nodes, *n)
	}
	return nodes, <-errCh
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
	createdNode := cluster.Node{
		Address:        newAddr,
		Metadata:       metadata,
		CreationStatus: cluster.NodeCreationStatusPending,
	}
	err = a.provisioner.Cluster().Register(createdNode)
	if err != nil {
		machine.Destroy()
		return nil, fmt.Errorf("error registering new node %s: %s", newAddr, err.Error())
	}
	q, err := queue.Queue()
	if err != nil {
		return nil, err
	}
	jobParams := monsterqueue.JobParams{
		"endpoint": createdNode.Address,
		"machine":  machine.Id,
		"metadata": createdNode.Metadata,
	}
	job, err := q.EnqueueWait(runBsTaskName, jobParams, a.waitTimeNewMachine)
	if err != nil {
		return nil, err
	}
	_, err = job.Result()
	if err != nil {
		return nil, err
	}
	a.logDebug("new machine created: %s - started!", newAddr)
	return &createdNode, nil
}

func (a *autoScaleConfig) removeMultipleNodes(chosenNodes []cluster.Node) error {
	nodeAddrs := make([]string, len(chosenNodes))
	nodeHosts := make([]string, len(chosenNodes))
	for i, node := range chosenNodes {
		_, hasIaas := node.Metadata["iaas"]
		if !hasIaas {
			return fmt.Errorf("no IaaS information in node (%s) metadata: %#v", node.Address, node.Metadata)
		}
		nodeAddrs[i] = node.Address
		nodeHosts[i] = urlToHost(node.Address)
	}
	err := a.provisioner.Cluster().UnregisterNodes(nodeAddrs...)
	if err != nil {
		return fmt.Errorf("unable to unregister nodes (%s) for removal: %s", strings.Join(nodeAddrs, ", "), err)
	}
	buf := safe.NewBuffer(nil)
	err = a.provisioner.moveContainersFromHosts(nodeHosts, "", buf)
	if err != nil {
		for _, node := range chosenNodes {
			a.provisioner.Cluster().Register(node)
		}
		return fmt.Errorf("unable to move containers from nodes (%s): %s - log: %s", strings.Join(nodeAddrs, ", "), err, buf.String())
	}
	wg := sync.WaitGroup{}
	for i := range chosenNodes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			node := chosenNodes[i]
			m, err := iaas.FindMachineByIdOrAddress(node.Metadata["iaas-id"], urlToHost(node.Address))
			if err != nil {
				a.logError("unable to find machine for removal in iaas: %s", err)
				return
			}
			err = m.Destroy()
			if err != nil {
				a.logError("unable to destroy machine in IaaS: %s", err)
			}
		}(i)
	}
	wg.Wait()
	return nil
}

func (a *autoScaleConfig) removeNode(chosenNode *cluster.Node) error {
	_, hasIaas := chosenNode.Metadata["iaas"]
	if !hasIaas {
		return fmt.Errorf("no IaaS information in node (%s) metadata: %#v", chosenNode.Address, chosenNode.Metadata)
	}
	err := a.provisioner.Cluster().Unregister(chosenNode.Address)
	if err != nil {
		return fmt.Errorf("unable to unregister node (%s) for removal: %s", chosenNode.Address, err)
	}
	buf := safe.NewBuffer(nil)
	err = a.provisioner.moveContainers(urlToHost(chosenNode.Address), "", buf)
	if err != nil {
		a.provisioner.Cluster().Register(*chosenNode)
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

func chooseNodeForRemoval(nodes []*cluster.Node, toRemoveCount int) []cluster.Node {
	var chosenNodes []cluster.Node
	remainingNodes := nodes[:]
	for _, node := range nodes {
		canRemove, _ := canRemoveNode(node, remainingNodes)
		if canRemove {
			for i := range remainingNodes {
				if remainingNodes[i].Address == node.Address {
					remainingNodes = append(remainingNodes[:i], remainingNodes[i+1:]...)
					break
				}
			}
			chosenNodes = append(chosenNodes, *node)
			if len(chosenNodes) >= toRemoveCount {
				break
			}
		}
	}
	return chosenNodes
}

func canRemoveNode(chosenNode *cluster.Node, nodes []*cluster.Node) (bool, error) {
	if len(nodes) == 1 {
		return false, nil
	}
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

func (p *dockerProvisioner) runningContainersByNode(nodes []*cluster.Node) (map[string][]container.Container, error) {
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
	result := map[string][]container.Container{}
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
	minCount := -1
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
		if minCount == -1 || contCount < minCount {
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
