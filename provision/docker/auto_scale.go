// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/nodecontainer"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/safe"
	"gopkg.in/mgo.v2"
)

var errAutoScaleRunning = errors.New("autoscale already running")

type autoScaleConfig struct {
	GroupByMetadata     string
	WaitTimeNewMachine  time.Duration
	RunInterval         time.Duration
	TotalMemoryMetadata string
	Enabled             bool
	provisioner         *dockerProvisioner
	done                chan bool
	writer              io.Writer
}

type scalerResult struct {
	toAdd    int
	toRemove []cluster.Node
	reason   string
}

type autoScaler interface {
	scale(groupMetadata string, nodes []*cluster.Node) (*scalerResult, error)
}

type metaWithFrequency struct {
	metadata map[string]string
	freq     int
}

type metaWithFrequencyList []metaWithFrequency

func (l metaWithFrequencyList) Len() int           { return len(l) }
func (l metaWithFrequencyList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l metaWithFrequencyList) Less(i, j int) bool { return l[i].freq < l[j].freq }

func (a *autoScaleConfig) initialize() {
	if a.TotalMemoryMetadata == "" {
		a.TotalMemoryMetadata, _ = config.GetString("docker:scheduler:total-memory-metadata")
	}
	if a.RunInterval == 0 {
		a.RunInterval = time.Hour
	}
	if a.WaitTimeNewMachine == 0 {
		a.WaitTimeNewMachine = 5 * time.Minute
	}
}

func (a *autoScaleConfig) scalerForRule(rule *autoScaleRule) (autoScaler, error) {
	if rule.MaxContainerCount > 0 {
		return &countScaler{autoScaleConfig: a, rule: rule}, nil
	}
	return &memoryScaler{autoScaleConfig: a, rule: rule}, nil
}

func (a *autoScaleConfig) run() error {
	a.initialize()
	for {
		err := a.runScaler()
		if err != nil {
			a.logError(err.Error())
			err = fmt.Errorf("[node autoscale] %s", err.Error())
		}
		select {
		case <-a.done:
			return err
		case <-time.After(a.RunInterval):
		}
	}
}

func (a *autoScaleConfig) logError(msg string, params ...interface{}) {
	msg = fmt.Sprintf("[node autoscale] %s", msg)
	log.Errorf(msg, params...)
}

func (a *autoScaleConfig) logDebug(msg string, params ...interface{}) {
	msg = fmt.Sprintf("[node autoscale] %s", msg)
	log.Debugf(msg, params...)
}

func (a *autoScaleConfig) runOnce() error {
	a.initialize()
	err := a.runScaler()
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

func (a *autoScaleConfig) runScaler() (retErr error) {
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
		if a.GroupByMetadata == "" {
			clusterMap[""] = append(clusterMap[""], node)
			continue
		}
		groupMetadata := node.Metadata[a.GroupByMetadata]
		if groupMetadata == "" {
			a.logDebug("skipped node %s, no metadata value for %s.", node.Address, a.GroupByMetadata)
			continue
		}
		clusterMap[groupMetadata] = append(clusterMap[groupMetadata], node)
	}
	for groupMetadata, nodes := range clusterMap {
		a.runScalerInNodes(groupMetadata, nodes)
	}
	return
}

func (a *autoScaleConfig) runScalerInNodes(groupMetadata string, nodes []*cluster.Node) {
	event, err := newAutoScaleEvent(groupMetadata, a.writer)
	if err != nil {
		if err == errAutoScaleRunning {
			a.logDebug("skipping already running for: %s", groupMetadata)
		} else {
			a.logError("error creating scale event %s: %s", groupMetadata, err.Error())
		}
		return
	}
	var retErr error
	defer func() {
		event.finish(retErr)
	}()
	rule, err := autoScaleRuleForMetadata(groupMetadata)
	if err == mgo.ErrNotFound {
		rule, err = autoScaleRuleForMetadata("")
	}
	if err != nil {
		if err != mgo.ErrNotFound {
			retErr = fmt.Errorf("unable to fetch auto scale rules for %s: %s", groupMetadata, err)
			return
		}
		event.logMsg("no auto scale rule for %s", groupMetadata)
		return
	}
	if !rule.Enabled {
		event.logMsg("auto scale rule disabled for %s", groupMetadata)
		return
	}
	scaler, err := a.scalerForRule(rule)
	if err != nil {
		retErr = fmt.Errorf("error getting scaler for %s: %s", groupMetadata, err)
		return
	}
	event.logMsg("running scaler %T for %q: %q", scaler, a.GroupByMetadata, groupMetadata)
	scalerResult, err := scaler.scale(groupMetadata, nodes)
	if err != nil {
		retErr = fmt.Errorf("error scaling group %s: %s", groupMetadata, err.Error())
		return
	}
	if scalerResult != nil {
		if scalerResult.toAdd > 0 {
			msg := fmt.Sprintf("%s, adding %d nodes", scalerResult.reason, scalerResult.toAdd)
			err = event.update(scaleActionAdd, msg)
			if err != nil {
				retErr = fmt.Errorf("error updating event: %s", err)
				return
			}
			event.logMsg("running event %q for %q: %s", event.Action, event.MetadataValue, event.Reason)
			var newNodes []cluster.Node
			newNodes, err = a.addMultipleNodes(event, nodes, scalerResult.toAdd)
			if err != nil {
				if len(newNodes) == 0 {
					retErr = err
					return
				}
				event.logMsg("not all required nodes were created: %s", err)
			}
			event.updateNodes(newNodes)
		} else if len(scalerResult.toRemove) > 0 {
			event.updateNodes(scalerResult.toRemove)
			msg := fmt.Sprintf("%s, removing %d nodes", scalerResult.reason, len(scalerResult.toRemove))
			err = event.update(scaleActionRemove, msg)
			if err != nil {
				retErr = fmt.Errorf("error updating event: %s", err)
				return
			}
			event.logMsg("running event %q for %q: %s", event.Action, event.MetadataValue, event.Reason)
			err = a.removeMultipleNodes(event, scalerResult.toRemove)
			if err != nil {
				retErr = err
				return
			}
		}
	}
	if !rule.PreventRebalance {
		err = a.rebalanceIfNeeded(event, groupMetadata, nodes)
		if err != nil {
			event.logMsg("unable to rebalance: %s", err.Error())
		}
	}
	if event.Action == "" {
		event.logMsg("nothing to do for %q: %q", a.GroupByMetadata, groupMetadata)
	}
	return
}

func (a *autoScaleConfig) rebalanceIfNeeded(event *autoScaleEvent, groupMetadata string, nodes []*cluster.Node) error {
	var rebalanceFilter map[string]string
	if a.GroupByMetadata != "" {
		rebalanceFilter = map[string]string{a.GroupByMetadata: groupMetadata}
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
		event.logMsg("running rebalance, due to %q for %q: %s", event.Action, event.MetadataValue, event.Reason)
		buf := safe.NewBuffer(nil)
		writer := io.MultiWriter(buf, &event.logBuffer)
		_, err := a.provisioner.rebalanceContainersByFilter(writer, nil, rebalanceFilter, false)
		if err != nil {
			return fmt.Errorf("unable to rebalance containers: %s - log: %s", err.Error(), buf.String())
		}
		return nil
	}
	return nil
}

func (a *autoScaleConfig) addMultipleNodes(event *autoScaleEvent, modelNodes []*cluster.Node, count int) ([]cluster.Node, error) {
	wg := sync.WaitGroup{}
	wg.Add(count)
	nodesCh := make(chan *cluster.Node, count)
	errCh := make(chan error, count)
	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			node, err := a.addNode(event, modelNodes)
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

func (a *autoScaleConfig) addNode(event *autoScaleEvent, modelNodes []*cluster.Node) (*cluster.Node, error) {
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
	event.logMsg("new machine created: %s - Waiting for docker to start...", newAddr)
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
	if err == nil {
		jobParams := monsterqueue.JobParams{
			"endpoint": createdNode.Address,
			"machine":  machine.Id,
			"metadata": createdNode.Metadata,
		}
		var job monsterqueue.Job
		job, err = q.EnqueueWait(nodecontainer.QueueTaskName, jobParams, a.WaitTimeNewMachine)
		if err == nil {
			_, err = job.Result()
		}
	}
	if err != nil {
		machine.Destroy()
		a.provisioner.Cluster().Unregister(newAddr)
		return nil, fmt.Errorf("error running bs task: %s", err)
	}
	event.logMsg("new machine created: %s - started!", newAddr)
	return &createdNode, nil
}

func (a *autoScaleConfig) removeMultipleNodes(event *autoScaleEvent, chosenNodes []cluster.Node) error {
	nodeAddrs := make([]string, len(chosenNodes))
	nodeHosts := make([]string, len(chosenNodes))
	for i, node := range chosenNodes {
		_, hasIaas := node.Metadata["iaas"]
		if !hasIaas {
			return fmt.Errorf("no IaaS information in node (%s) metadata: %#v", node.Address, node.Metadata)
		}
		nodeAddrs[i] = node.Address
		nodeHosts[i] = net.URLToHost(node.Address)
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
			m, err := iaas.FindMachineByIdOrAddress(node.Metadata["iaas-id"], net.URLToHost(node.Address))
			if err != nil {
				event.logMsg("unable to find machine for removal in iaas: %s", err)
				return
			}
			err = m.Destroy()
			if err != nil {
				event.logMsg("unable to destroy machine in IaaS: %s", err)
			}
		}(i)
	}
	wg.Wait()
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
		nodeConts, err := p.listRunningContainersByHost(net.URLToHost(n.Address))
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
