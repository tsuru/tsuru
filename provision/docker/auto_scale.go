// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/nodecontainer"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/safe"
	"gopkg.in/mgo.v2"
)

const (
	poolMetadataName   = "pool"
	autoScaleEventKind = "autoscale"
)

type errAppNotLocked struct {
	app string
}

func (e errAppNotLocked) Error() string {
	return fmt.Sprintf("unable to lock app %q", e.app)
}

type autoScaleConfig struct {
	WaitTimeNewMachine  time.Duration
	RunInterval         time.Duration
	TotalMemoryMetadata string
	Enabled             bool
	provisioner         *dockerProvisioner
	done                chan bool
	writer              io.Writer
}

type scalerResult struct {
	ToAdd       int
	ToRemove    []cluster.Node
	ToRebalance bool
	Reason      string
}

func (r *scalerResult) IsRebalanceOnly() bool {
	return r.ToAdd == 0 && len(r.ToRemove) == 0 && r.ToRebalance
}

func (r *scalerResult) NoAction() bool {
	return r.ToAdd == 0 && len(r.ToRemove) == 0 && !r.ToRebalance
}

type autoScaler interface {
	scale(pool string, nodes []*cluster.Node) (*scalerResult, error)
}

type metaWithFrequency struct {
	metadata map[string]string
	nodes    []*cluster.Node
}

type metaWithFrequencyList []metaWithFrequency

func (l metaWithFrequencyList) Len() int           { return len(l) }
func (l metaWithFrequencyList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l metaWithFrequencyList) Less(i, j int) bool { return len(l[i].nodes) < len(l[j].nodes) }

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
			err = errors.Wrap(err, "[node autoscale]")
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
			retErr = errors.Errorf("recovered panic, we can never stop! panic: %v", r)
		}
	}()
	nodes, err := a.provisioner.Cluster().Nodes()
	if err != nil {
		retErr = errors.Wrap(err, "error getting nodes")
		return
	}
	clusterMap := map[string][]*cluster.Node{}
	for i := range nodes {
		node := &nodes[i]
		pool := node.Metadata[poolMetadataName]
		if pool == "" {
			a.logDebug("skipped node %s, no metadata value for %s.", node.Address, poolMetadataName)
			continue
		}
		clusterMap[pool] = append(clusterMap[pool], node)
	}
	for pool, nodes := range clusterMap {
		a.runScalerInNodes(pool, nodes)
	}
	return
}

type evtCustomData struct {
	Result *scalerResult
	Nodes  []cluster.Node
	Rule   *autoScaleRule
}

func (a *autoScaleConfig) runScalerInNodes(pool string, nodes []*cluster.Node) {
	evt, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Type: event.TargetTypePool, Value: pool},
		InternalKind: autoScaleEventKind,
		Allowed:      event.Allowed(permission.PermPoolReadEvents, permission.Context(permission.CtxPool, pool)),
	})
	if err != nil {
		if _, ok := err.(event.ErrEventLocked); ok {
			a.logDebug("skipping already running for: %s", pool)
		} else {
			a.logError("error creating scale event %s: %s", pool, err.Error())
		}
		return
	}
	evt.SetLogWriter(a.writer)
	var retErr error
	var sResult *scalerResult
	var evtNodes []cluster.Node
	var rule *autoScaleRule
	defer func() {
		if retErr != nil {
			evt.Logf(retErr.Error())
		}
		if (sResult == nil && retErr == nil) || (sResult != nil && sResult.NoAction()) {
			evt.Logf("nothing to do for %q: %q", poolMetadataName, pool)
			evt.Abort()
		} else {
			evt.DoneCustomData(retErr, evtCustomData{
				Result: sResult,
				Nodes:  evtNodes,
				Rule:   rule,
			})
		}
	}()
	rule, err = autoScaleRuleForMetadata(pool)
	if err == mgo.ErrNotFound {
		rule, err = autoScaleRuleForMetadata("")
	}
	if err != nil {
		if err != mgo.ErrNotFound {
			retErr = errors.Wrapf(err, "unable to fetch auto scale rules for %s", pool)
			return
		}
		evt.Logf("no auto scale rule for %s", pool)
		return
	}
	if !rule.Enabled {
		evt.Logf("auto scale rule disabled for %s", pool)
		return
	}
	scaler, err := a.scalerForRule(rule)
	if err != nil {
		retErr = errors.Wrapf(err, "error getting scaler for %s", pool)
		return
	}
	evt.Logf("running scaler %T for %q: %q", scaler, poolMetadataName, pool)
	sResult, err = scaler.scale(pool, nodes)
	if err != nil {
		if _, ok := err.(errAppNotLocked); ok {
			evt.Logf("aborting scaler for now, gonna retry later: %s", err)
			return
		}
		retErr = errors.Wrapf(err, "error scaling group %s", pool)
		return
	}
	if sResult.ToAdd > 0 {
		evt.Logf("running event \"add\" for %q: %#v", pool, sResult)
		evtNodes, err = a.addMultipleNodes(evt, nodes, sResult.ToAdd)
		if err != nil {
			if len(evtNodes) == 0 {
				retErr = err
				return
			}
			evt.Logf("not all required nodes were created: %s", err)
		}
	} else if len(sResult.ToRemove) > 0 {
		evt.Logf("running event \"remove\" for %q: %#v", pool, sResult)
		evtNodes = sResult.ToRemove
		err = a.removeMultipleNodes(evt, sResult.ToRemove)
		if err != nil {
			retErr = err
			return
		}
	}
	if !rule.PreventRebalance {
		err := a.rebalanceIfNeeded(evt, pool, nodes, sResult)
		if err != nil {
			if sResult.IsRebalanceOnly() {
				retErr = err
			} else {
				evt.Logf("unable to rebalance: %s", err.Error())
			}
		}
	}
}

func (a *autoScaleConfig) rebalanceIfNeeded(evt *event.Event, pool string, nodes []*cluster.Node, sResult *scalerResult) error {
	if len(sResult.ToRemove) > 0 {
		return nil
	}
	if sResult.ToAdd > 0 {
		sResult.ToRebalance = true
	}
	rebalanceFilter := map[string]string{poolMetadataName: pool}
	if !sResult.ToRebalance {
		// No action yet, check if we need rebalance
		_, gap, err := a.provisioner.containerGapInNodes(nodes)
		buf := safe.NewBuffer(nil)
		dryProvisioner, err := a.provisioner.rebalanceContainersByFilter(buf, nil, rebalanceFilter, true)
		if err != nil {
			return errors.Wrapf(err, "unable to run dry rebalance to check if rebalance is needed. log: %s", buf.String())
		}
		if dryProvisioner == nil {
			return nil
		}
		_, gapAfter, err := dryProvisioner.containerGapInNodes(nodes)
		if err != nil {
			return errors.Wrap(err, "couldn't find containers from rebalanced nodes")
		}
		if math.Abs((float64)(gap-gapAfter)) > 2.0 {
			sResult.ToRebalance = true
			if sResult.Reason == "" {
				sResult.Reason = fmt.Sprintf("gap is %d, after rebalance gap will be %d", gap, gapAfter)
			}
		}
	}
	if sResult.ToRebalance {
		evt.Logf("running rebalance, for %q: %#v", pool, sResult)
		buf := safe.NewBuffer(nil)
		writer := io.MultiWriter(buf, evt)
		_, err := a.provisioner.rebalanceContainersByFilter(writer, nil, rebalanceFilter, false)
		if err != nil {
			return errors.Wrapf(err, "unable to rebalance containers. log: %s", buf.String())
		}
	}
	return nil
}

func (a *autoScaleConfig) addMultipleNodes(evt *event.Event, modelNodes []*cluster.Node, count int) ([]cluster.Node, error) {
	wg := sync.WaitGroup{}
	wg.Add(count)
	nodesCh := make(chan *cluster.Node, count)
	errCh := make(chan error, count)
	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			node, err := a.addNode(evt, modelNodes)
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

func (a *autoScaleConfig) addNode(evt *event.Event, modelNodes []*cluster.Node) (*cluster.Node, error) {
	metadata, err := chooseMetadataFromNodes(modelNodes)
	if err != nil {
		return nil, err
	}
	_, hasIaas := metadata["iaas"]
	if !hasIaas {
		return nil, errors.Errorf("no IaaS information in nodes metadata: %#v", metadata)
	}
	machine, err := iaas.CreateMachineForIaaS(metadata["iaas"], metadata)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create machine")
	}
	newAddr := machine.FormatNodeAddress()
	evt.Logf("new machine created: %s - Waiting for docker to start...", newAddr)
	createdNode := cluster.Node{
		Address:        newAddr,
		Metadata:       metadata,
		CreationStatus: cluster.NodeCreationStatusPending,
	}
	err = a.provisioner.Cluster().Register(createdNode)
	if err != nil {
		machine.Destroy()
		return nil, errors.Wrapf(err, "error registering new node %s", newAddr)
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
		return nil, errors.Wrap(err, "error running bs task")
	}
	evt.Logf("new machine created: %s - started!", newAddr)
	return &createdNode, nil
}

func (a *autoScaleConfig) removeMultipleNodes(evt *event.Event, chosenNodes []cluster.Node) error {
	nodeAddrs := make([]string, len(chosenNodes))
	nodeHosts := make([]string, len(chosenNodes))
	for i, node := range chosenNodes {
		_, hasIaas := node.Metadata["iaas"]
		if !hasIaas {
			return errors.Errorf("no IaaS information in node (%s) metadata: %#v", node.Address, node.Metadata)
		}
		nodeAddrs[i] = node.Address
		nodeHosts[i] = net.URLToHost(node.Address)
	}
	err := a.provisioner.Cluster().UnregisterNodes(nodeAddrs...)
	if err != nil {
		return errors.Wrapf(err, "unable to unregister nodes (%s) for removal", strings.Join(nodeAddrs, ", "))
	}
	buf := safe.NewBuffer(nil)
	err = a.provisioner.moveContainersFromHosts(nodeHosts, "", buf)
	if err != nil {
		for _, node := range chosenNodes {
			a.provisioner.Cluster().Register(node)
		}
		return errors.Wrapf(err, "unable to move containers from nodes (%s). log: %s", strings.Join(nodeAddrs, ", "), buf.String())
	}
	wg := sync.WaitGroup{}
	for i := range chosenNodes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			node := chosenNodes[i]
			m, err := iaas.FindMachineByIdOrAddress(node.Metadata["iaas-id"], net.URLToHost(node.Address))
			if err != nil {
				evt.Logf("unable to find machine for removal in iaas: %s", err)
				return
			}
			err = m.Destroy()
			if err != nil {
				evt.Logf("unable to destroy machine in IaaS: %s", err)
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
	exclusiveList, _, err := splitMetadata(nodes)
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
			if len(item.nodes) > 1 {
				return true, nil
			}
			return false, nil
		}
	}
	return false, nil
}

func cleanMetadata(n *cluster.Node) map[string]string {
	// iaas-id is ignored because it wasn't created in previous tsuru versions
	// and having nodes with and without it would cause unbalanced metadata
	// errors.
	ignoredMetadata := []string{"iaas-id"}
	metadata := n.CleanMetadata()
	for _, val := range ignoredMetadata {
		delete(metadata, val)
	}
	return metadata
}

func splitMetadata(nodes []*cluster.Node) (metaWithFrequencyList, map[string]string, error) {
	common := make(map[string]string)
	exclusive := make([]map[string]string, len(nodes))
	for i := range nodes {
		metadata := cleanMetadata(nodes[i])
		for k, v := range metadata {
			isExclusive := false
			for j := range nodes {
				if i == j {
					continue
				}
				otherMetadata := cleanMetadata(nodes[j])
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
		groupNodes := []*cluster.Node{nodes[i]}
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
			if diffCount > 0 && (diffCount < len(exclusive[i]) || diffCount > len(exclusive[j])) {
				return nil, nil, errors.Errorf("unbalanced metadata for node group: %v vs %v", exclusive[i], exclusive[j])
			}
			if diffCount == 0 {
				sameMap[j] = true
				groupNodes = append(groupNodes, nodes[j])
			}
		}
		if !sameMap[i] && exclusive[i] != nil {
			group = append(group, metaWithFrequency{metadata: exclusive[i], nodes: groupNodes})
		}
	}
	return group, common, nil
}

func chooseMetadataFromNodes(modelNodes []*cluster.Node) (map[string]string, error) {
	exclusiveList, baseMetadata, err := splitMetadata(modelNodes)
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
			return nil, errAppNotLocked{app: appName}
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
