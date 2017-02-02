// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package autoscale

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/app"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/safe"
	"gopkg.in/mgo.v2"
)

const (
	poolMetadataName   = "pool"
	autoScaleEventKind = "autoscale"
)

var globalConfig *AutoScaleConfig

type AutoScaleConfig struct {
	WaitTimeNewMachine  time.Duration
	RunInterval         time.Duration
	TotalMemoryMetadata string
	Enabled             bool
	done                chan bool
	writer              io.Writer
}

func BaseConfig() (AutoScaleConfig, error) {
	if globalConfig == nil {
		return AutoScaleConfig{}, errors.New("autoscale not initialized")
	}
	return *globalConfig, nil
}

func Initialize() error {
	enabled, _ := config.GetBool("docker:auto-scale:enabled")
	waitSecondsNewMachine, _ := config.GetInt("docker:auto-scale:wait-new-time")
	runInterval, _ := config.GetInt("docker:auto-scale:run-interval")
	totalMemoryMetadata, _ := config.GetString("docker:scheduler:total-memory-metadata")
	globalConfig = &AutoScaleConfig{
		TotalMemoryMetadata: totalMemoryMetadata,
		WaitTimeNewMachine:  time.Duration(waitSecondsNewMachine) * time.Second,
		RunInterval:         time.Duration(runInterval) * time.Second,
		Enabled:             enabled,
		done:                make(chan bool),
	}
	if !globalConfig.Enabled {
		return nil
	}
	shutdown.Register(globalConfig)
	go globalConfig.run()
	return nil
}

type errAppNotLocked struct {
	app string
}

func (e errAppNotLocked) Error() string {
	return fmt.Sprintf("unable to lock app %q", e.app)
}

type scalerResult struct {
	ToAdd       int
	ToRemove    []provision.NodeSpec
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
	scale(pool string, nodes []provision.Node) (*scalerResult, error)
}

type metaWithFrequency struct {
	metadata map[string]string
	nodes    []provision.Node
}

type metaWithFrequencyList []metaWithFrequency

func (l metaWithFrequencyList) Len() int           { return len(l) }
func (l metaWithFrequencyList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l metaWithFrequencyList) Less(i, j int) bool { return len(l[i].nodes) < len(l[j].nodes) }

func (a *AutoScaleConfig) initialize() {
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

func (a *AutoScaleConfig) scalerForRule(rule *autoScaleRule) (autoScaler, error) {
	if rule.MaxContainerCount > 0 {
		return &countScaler{AutoScaleConfig: a, rule: rule}, nil
	}
	return &memoryScaler{AutoScaleConfig: a, rule: rule}, nil
}

func (a *AutoScaleConfig) run() error {
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

func (a *AutoScaleConfig) logError(msg string, params ...interface{}) {
	msg = fmt.Sprintf("[node autoscale] %s", msg)
	log.Errorf(msg, params...)
}

func (a *AutoScaleConfig) logDebug(msg string, params ...interface{}) {
	msg = fmt.Sprintf("[node autoscale] %s", msg)
	log.Debugf(msg, params...)
}

func (a *AutoScaleConfig) runOnce() error {
	a.initialize()
	err := a.runScaler()
	if err != nil {
		a.logError(err.Error())
	}
	return err
}

func (a *AutoScaleConfig) stop() {
	a.done <- true
}

func (a *AutoScaleConfig) Shutdown() {
	a.stop()
}

func (a *AutoScaleConfig) String() string {
	return "node auto scale"
}

func (a *AutoScaleConfig) runScaler() (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = errors.Errorf("recovered panic, we can never stop! panic: %v", r)
		}
	}()
	provs, err := provision.Registry()
	if err != nil {
		return errors.Wrap(err, "error getting provisioners")
	}
	provPoolMap := map[string]provision.NodeProvisioner{}
	var allNodes []provision.Node
	for _, prov := range provs {
		nodeProv, ok := prov.(provision.NodeProvisioner)
		if !ok {
			continue
		}
		var nodes []provision.Node
		nodes, err = nodeProv.ListNodes(nil)
		if err != nil {
			return errors.Wrap(err, "error getting nodes")
		}
		for _, n := range nodes {
			provPoolMap[n.Pool()] = nodeProv
		}
		allNodes = append(allNodes, nodes...)
	}
	clusterMap := map[string][]provision.Node{}
	for _, node := range allNodes {
		pool := node.Pool()
		if pool == "" {
			a.logDebug("skipped node %s, no pool value found.", node.Address)
			continue
		}
		clusterMap[pool] = append(clusterMap[pool], node)
	}
	for pool, nodes := range clusterMap {
		a.runScalerInNodes(provPoolMap[pool], pool, nodes)
	}
	return
}

type evtCustomData struct {
	Result *scalerResult
	Nodes  []provision.NodeSpec
	Rule   *autoScaleRule
}

func nodesToSpec(nodes []provision.Node) []provision.NodeSpec {
	var nodeSpecs []provision.NodeSpec
	for _, n := range nodes {
		nodeSpecs = append(nodeSpecs, provision.NodeToSpec(n))
	}
	return nodeSpecs
}

func (a *AutoScaleConfig) runScalerInNodes(prov provision.NodeProvisioner, pool string, nodes []provision.Node) {
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
	var evtNodes []provision.NodeSpec
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
		evtNodes, err = a.addMultipleNodes(evt, prov, nodes, sResult.ToAdd)
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
		err = a.removeMultipleNodes(evt, prov, sResult.ToRemove)
		if err != nil {
			retErr = err
			return
		}
	}
	if !rule.PreventRebalance {
		err := a.rebalanceIfNeeded(evt, prov, pool, nodes, sResult)
		if err != nil {
			if sResult.IsRebalanceOnly() {
				retErr = err
			} else {
				evt.Logf("unable to rebalance: %s", err.Error())
			}
		}
	}
}

func (a *AutoScaleConfig) rebalanceIfNeeded(evt *event.Event, prov provision.NodeProvisioner, pool string, nodes []provision.Node, sResult *scalerResult) error {
	if len(sResult.ToRemove) > 0 {
		return nil
	}
	rebalanceProv, ok := prov.(provision.NodeRebalanceProvisioner)
	if !ok {
		return nil
	}
	buf := safe.NewBuffer(nil)
	writer := io.MultiWriter(buf, evt)
	shouldRebalance, err := rebalanceProv.RebalanceNodes(provision.RebalanceNodesOptions{
		Force:          false,
		MetadataFilter: map[string]string{"pool": pool},
		Writer:         writer,
	})
	sResult.ToRebalance = shouldRebalance
	if err != nil {
		return errors.Wrapf(err, "unable to rebalance containers. log: %s", buf.String())
	}
	return nil
}

func (a *AutoScaleConfig) addMultipleNodes(evt *event.Event, prov provision.NodeProvisioner, modelNodes []provision.Node, count int) ([]provision.NodeSpec, error) {
	wg := sync.WaitGroup{}
	wg.Add(count)
	nodesCh := make(chan provision.Node, count)
	errCh := make(chan error, count)
	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			node, err := a.addNode(evt, prov, modelNodes)
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
	var nodes []provision.NodeSpec
	for n := range nodesCh {
		nodes = append(nodes, provision.NodeToSpec(n))
	}
	return nodes, <-errCh
}

func (a *AutoScaleConfig) addNode(evt *event.Event, prov provision.NodeProvisioner, modelNodes []provision.Node) (provision.Node, error) {
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
	createOpts := provision.AddNodeOptions{
		Address:    newAddr,
		Metadata:   metadata,
		WaitTO:     a.WaitTimeNewMachine,
		CaCert:     machine.CaCert,
		ClientCert: machine.ClientCert,
		ClientKey:  machine.ClientKey,
	}
	err = prov.AddNode(createOpts)
	if err != nil {
		return nil, errors.Wrapf(err, "error adding new node %s", newAddr)
	}
	evt.Logf("new machine created: %s - started!", newAddr)
	node, err := prov.GetNode(newAddr)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting new node %s", newAddr)
	}
	return node, nil
}

func (a *AutoScaleConfig) removeMultipleNodes(evt *event.Event, prov provision.NodeProvisioner, chosenNodes []provision.NodeSpec) error {
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
	errCh := make(chan error, len(chosenNodes))
	wg := sync.WaitGroup{}
	for i := range chosenNodes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			node := chosenNodes[i]
			buf := safe.NewBuffer(nil)
			err := prov.RemoveNode(provision.RemoveNodeOptions{
				Address:   node.Address,
				Writer:    buf,
				Rebalance: true,
			})
			if err != nil {
				errCh <- errors.Wrapf(err, "unable to unregister node %s for removal", node.Address)
				return
			}
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
	close(errCh)
	multiErr := tsuruErrors.NewMultiError()
	for err := range errCh {
		multiErr.Add(err)
	}
	if multiErr.Len() > 0 {
		return multiErr
	}
	return nil
}

func chooseNodeForRemoval(nodes []provision.Node, toRemoveCount int) []provision.Node {
	var chosenNodes []provision.Node
	remainingNodes := nodes[:]
	for _, node := range nodes {
		canRemove, _ := canRemoveNode(node, remainingNodes)
		if canRemove {
			for i := range remainingNodes {
				if remainingNodes[i].Address() == node.Address() {
					remainingNodes = append(remainingNodes[:i], remainingNodes[i+1:]...)
					break
				}
			}
			chosenNodes = append(chosenNodes, node)
			if len(chosenNodes) >= toRemoveCount {
				break
			}
		}
	}
	return chosenNodes
}

func canRemoveNode(chosenNode provision.Node, nodes []provision.Node) (bool, error) {
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
	hasMetadata := func(n provision.Node, meta map[string]string) bool {
		metadata := n.Metadata()
		for k, v := range meta {
			if metadata[k] != v {
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

func cleanMetadata(n provision.Node) map[string]string {
	// iaas-id is ignored because it wasn't created in previous tsuru versions
	// and having nodes with and without it would cause unbalanced metadata
	// errors.
	ignoredMetadata := []string{"iaas-id"}
	metadata := map[string]string{}
	for k, v := range n.Metadata() {
		metadata[k] = v
	}
	for _, val := range ignoredMetadata {
		delete(metadata, val)
	}
	return metadata
}

func splitMetadata(nodes []provision.Node) (metaWithFrequencyList, map[string]string, error) {
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
		groupNodes := []provision.Node{nodes[i]}
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

func chooseMetadataFromNodes(modelNodes []provision.Node) (map[string]string, error) {
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

func preciseUnitsByNode(pool string, nodes []provision.Node) (map[string][]provision.Unit, error) {
	appsInPool, err := app.List(&app.Filter{
		Pool: pool,
	})
	if err != nil {
		return nil, err
	}
	for _, a := range appsInPool {
		var locked bool
		locked, err = app.AcquireApplicationLock(a.Name, app.InternalAppName, "node auto scale")
		if err != nil {
			return nil, err
		}
		if !locked {
			return nil, errAppNotLocked{app: a.Name}
		}
		defer app.ReleaseApplicationLock(a.Name)
	}
	unitsByNode := map[string][]provision.Unit{}
	for _, node := range nodes {
		var nodeUnits []provision.Unit
		nodeUnits, err = node.Units()
		if err != nil {
			return nil, err
		}
		unitsByNode[node.Address()] = nodeUnits
	}
	return unitsByNode, err
}

func unitsGapInNodes(pool string, nodes []provision.Node) (int, int, error) {
	maxCount := 0
	minCount := -1
	totalCount := 0
	unitsByNode, err := preciseUnitsByNode(pool, nodes)
	if err != nil {
		return 0, 0, err
	}
	for _, units := range unitsByNode {
		unitCount := len(units)
		if unitCount > maxCount {
			maxCount = unitCount
		}
		if minCount == -1 || unitCount < minCount {
			minCount = unitCount
		}
		totalCount += unitCount
	}
	return totalCount, maxCount - minCount, nil
}
