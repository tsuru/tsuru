// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
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
	Action        string // "rebalance" or "add"
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

func newAutoScaleEvent(metadataValue, action string) (*autoScaleEvent, error) {
	// Use metadataValue as ID to ensure only one auto scale process runs for
	// each metadataValue. (*autoScaleEvent).update() will generate a new
	// unique ID and remove this initial record.
	evt := autoScaleEvent{
		ID:            metadataValue,
		StartTime:     time.Now().UTC(),
		MetadataValue: metadataValue,
		Action:        action,
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
	var scaler autoScaler
	if isMemoryBased {
		scaler = &memoryScaler{a}
	} else {
		scaler = &countScaler{a}
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
		if event != nil {
			buf := safe.NewBuffer(nil)
			_, err := a.provisioner.rebalanceContainersByFilter(buf, nil, rebalanceFilter, false)
			if err != nil {
				log.Errorf("[node autoscale] unable to rebalance containers: %s - log: %s", err.Error(), buf.String())
			}
			event.update(err)
		}
	}
	return
}

func (a *memoryScaler) scale(groupMetadata string, nodes []*cluster.Node) (*autoScaleEvent, error) {
	return nil, nil
}

func (a *countScaler) scale(groupMetadata string, nodes []*cluster.Node) (*autoScaleEvent, error) {
	totalCount, _, err := a.provisioner.containerGapInNodes(nodes)
	if err != nil {
		return nil, fmt.Errorf("couldn't find containers from nodes: %s", err)
	}
	freeSlots := (len(nodes) * a.maxContainerCount) - totalCount
	if freeSlots >= 0 {
		return nil, nil
	}
	event, err := newAutoScaleEvent(groupMetadata, "add")
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
	event, err := newAutoScaleEvent(groupMetadata, "rebalance")
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
