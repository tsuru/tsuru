// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type NodeHealer struct {
	provisioner           *dockerProvisioner
	disabledTime          time.Duration
	waitTimeNewMachine    time.Duration
	failuresBeforeHealing int
}

type healingEvent struct {
	ID               bson.ObjectId `bson:"_id"`
	StartTime        time.Time
	EndTime          time.Time `bson:",omitempty"`
	Action           string
	FailingNode      cluster.Node `bson:",omitempty"`
	CreatedNode      cluster.Node `bson:",omitempty"`
	FailingContainer container    `bson:",omitempty"`
	CreatedContainer container    `bson:",omitempty"`
	Successful       bool
	Error            string `bson:",omitempty"`
}

var (
	consecutiveHealingsTimeframe        = 30 * time.Minute
	consecutiveHealingsLimitInTimeframe = 3
)

func healingCollection() (*storage.Collection, error) {
	name, _ := config.GetString("docker:healing:events_collection")
	if name == "" {
		name = "healing_events"
	}
	conn, err := db.Conn()
	if err != nil {
		log.Errorf("Failed to connect to the database: %s", err.Error())
		return nil, err
	}
	return conn.Collection(name), nil
}

func newHealingEvent(failing interface{}) (*healingEvent, error) {
	evt := healingEvent{
		ID:        bson.NewObjectId(),
		StartTime: time.Now().UTC(),
	}
	switch v := failing.(type) {
	case cluster.Node:
		evt.Action = "node-healing"
		evt.FailingNode = v
	case container:
		evt.Action = "container-healing"
		evt.FailingContainer = v
	}
	coll, err := healingCollection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	return &evt, coll.Insert(evt)
}

func (evt *healingEvent) update(created interface{}, err error) error {
	if err != nil {
		evt.Error = err.Error()
	}
	evt.EndTime = time.Now().UTC()
	switch v := created.(type) {
	case cluster.Node:
		evt.CreatedNode = v
		evt.Successful = v.Address != ""
	case container:
		evt.CreatedContainer = v
		evt.Successful = v.ID != ""
	}
	coll, err := healingCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.UpdateId(evt.ID, evt)
}

func (h *NodeHealer) healNode(node *cluster.Node) (cluster.Node, error) {
	emptyNode := cluster.Node{}
	failingAddr := node.Address
	nodeMetadata := node.CleanMetadata()
	failingHost := urlToHost(failingAddr)
	failures := node.FailureCount()
	machine, err := iaas.CreateMachineForIaaS(nodeMetadata["iaas"], nodeMetadata)
	if err != nil {
		node.ResetFailures()
		return emptyNode, fmt.Errorf("Can't auto-heal after %d failures for node %s: error creating new machine: %s", failures, failingHost, err.Error())
	}
	err = h.provisioner.getCluster().Unregister(failingAddr)
	if err != nil {
		machine.Destroy()
		return emptyNode, fmt.Errorf("Can't auto-heal after %d failures for node %s: error unregistering old node: %s", failures, failingHost, err.Error())
	}
	newAddr := machine.FormatNodeAddress()
	log.Debugf("New machine created during healing process: %s - Waiting for docker to start...", newAddr)
	createdNode, err := h.provisioner.getCluster().WaitAndRegister(newAddr, nodeMetadata, h.waitTimeNewMachine)
	if err != nil {
		node.ResetFailures()
		h.provisioner.getCluster().Register(failingAddr, nodeMetadata)
		machine.Destroy()
		return emptyNode, fmt.Errorf("Can't auto-heal after %d failures for node %s: error registering new node: %s", failures, failingHost, err.Error())
	}
	var buf bytes.Buffer
	err = h.provisioner.moveContainers(failingHost, "", &buf)
	if err != nil {
		log.Errorf("Unable to move containers, skipping containers healing %q -> %q: %s: %s", failingHost, machine.Address, err.Error(), buf.String())
	}
	failingMachine, err := iaas.FindMachineByIdOrAddress(node.Metadata["iaas-id"], failingHost)
	if err != nil {
		return createdNode, fmt.Errorf("Unable to find failing machine %s in IaaS: %s", failingHost, err.Error())
	}
	err = failingMachine.Destroy()
	if err != nil {
		return createdNode, fmt.Errorf("Unable to destroy machine %s from IaaS: %s", failingHost, err.Error())
	}
	log.Debugf("Done auto-healing node %q, node %q created in its place.", failingHost, machine.Address)
	return createdNode, nil
}

func (h *NodeHealer) HandleError(node *cluster.Node) time.Duration {
	failures := node.FailureCount()
	if failures < h.failuresBeforeHealing {
		log.Debugf("%d failures detected in node %q, waiting for more failures before healing.", failures, node.Address)
		return h.disabledTime
	}
	if !node.HasSuccess() {
		log.Debugf("Node %q has never been successfully reached, healing won't run on it.", node.Address)
		return h.disabledTime
	}
	_, hasIaas := node.Metadata["iaas"]
	if !hasIaas {
		log.Debugf("Node %q doesn't have IaaS information, healing won't run on it.", node.Address)
		return h.disabledTime
	}
	healingCounter, err := healingCountFor("node", node.Address, consecutiveHealingsTimeframe)
	if err != nil {
		log.Errorf("Node healing: couldn't verify number of previous healings for %s: %s", node.Address, err.Error())
		return h.disabledTime
	}
	if healingCounter > consecutiveHealingsLimitInTimeframe {
		log.Errorf("Node healing: number of healings for node %s in the last %d minutes exceeds limit of %d: %d",
			node.Address, consecutiveHealingsTimeframe/time.Minute, consecutiveHealingsLimitInTimeframe, healingCounter)
		return h.disabledTime
	}
	log.Errorf("Initiating healing process for node %q after %d failures.", node.Address, failures)
	evt, err := newHealingEvent(*node)
	if err != nil {
		log.Errorf("Error trying to insert healing event: %s", err.Error())
		return h.disabledTime
	}
	createdNode, err := h.healNode(node)
	if err != nil {
		log.Errorf("Error healing: %s", err.Error())
	}
	err = evt.update(createdNode, err)
	if err != nil {
		log.Errorf("Error trying to update healing event: %s", err.Error())
	}
	if createdNode.Address != "" {
		return 0
	}
	return h.disabledTime
}

func (p *dockerProvisioner) healContainer(cont container, locker *appLocker) (container, error) {
	var buf bytes.Buffer
	moveErrors := make(chan error, 1)
	createdContainer := p.moveOneContainer(cont, "", moveErrors, nil, &buf, locker)
	close(moveErrors)
	err := handleMoveErrors(moveErrors, &buf)
	if err != nil {
		err = fmt.Errorf("Error trying to heal containers %s: couldn't move container: %s - %s", cont.ID, err.Error(), buf.String())
	}
	return createdContainer, err
}

func (p *dockerProvisioner) hasProcfileWatcher(cont container) (bool, error) {
	topResult, err := p.getCluster().TopContainer(cont.ID, "")
	if err != nil {
		return false, err
	}
	for _, psLine := range topResult.Processes {
		line := strings.ToLower(strings.Join(psLine, " "))
		if strings.Contains(line, "procfilewatcher") {
			return true, nil
		}
	}
	return false, nil
}

func (p *dockerProvisioner) runContainerHealer(maxUnresponsiveTime time.Duration) {
	for {
		p.runContainerHealerOnce(maxUnresponsiveTime)
		time.Sleep(30 * time.Second)
	}
}

func (p *dockerProvisioner) healContainerIfNeeded(cont container) error {
	if cont.LastSuccessStatusUpdate.IsZero() {
		return nil
	}
	hasProcfile, err := p.hasProcfileWatcher(cont)
	if err != nil {
		log.Errorf("Containers healing: couldn't verify running processes in container %s: %s", cont.ID, err.Error())
	}
	if hasProcfile {
		cont.setStatus(p, provision.StatusStarted.String())
		return nil
	}
	healingCounter, err := healingCountFor("container", cont.ID, consecutiveHealingsTimeframe)
	if err != nil {
		return fmt.Errorf("Containers healing: couldn't verify number of previous healings for %s: %s", cont.ID, err.Error())
	}
	if healingCounter > consecutiveHealingsLimitInTimeframe {
		return fmt.Errorf("Containers healing: number of healings for container %s in the last %d minutes exceeds limit of %d: %d",
			cont.ID, consecutiveHealingsTimeframe/time.Minute, consecutiveHealingsLimitInTimeframe, healingCounter)
	}
	locker := &appLocker{}
	locked := locker.lock(cont.AppName)
	if !locked {
		return fmt.Errorf("Containers healing: unable to heal %s couldn't lock app %s", cont.ID, cont.AppName)
	}
	defer locker.unlock(cont.AppName)
	// Sanity check, now we have a lock, let's find out if the container still exists
	_, err = p.getContainer(cont.ID)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil
		}
		return fmt.Errorf("Containers healing: unable to heal %s couldn't verify it still exists.", cont.ID)
	}
	log.Errorf("Initiating healing process for container %s, unresponsive since %s.", cont.ID, cont.LastSuccessStatusUpdate)
	evt, err := newHealingEvent(cont)
	if err != nil {
		return fmt.Errorf("Error trying to insert container healing event, healing aborted: %s", err.Error())
	}
	newCont, healErr := p.healContainer(cont, locker)
	if healErr != nil {
		healErr = fmt.Errorf("Error healing container %s: %s", cont.ID, healErr.Error())
	}
	err = evt.update(newCont, healErr)
	if err != nil {
		log.Errorf("Error trying to update containers healing event: %s", err.Error())
	}
	return healErr
}

func (p *dockerProvisioner) runContainerHealerOnce(maxUnresponsiveTime time.Duration) {
	containers, err := p.listUnresponsiveContainers(maxUnresponsiveTime)
	if err != nil {
		log.Errorf("Containers Healing: couldn't list unresponsive containers: %s", err.Error())
	}
	for _, cont := range containers {
		err := p.healContainerIfNeeded(cont)
		if err != nil {
			log.Errorf(err.Error())
		}
	}
}

func listHealingHistory(filter string) ([]healingEvent, error) {
	coll, err := healingCollection()
	if err != nil {
		return nil, err
	}
	query := bson.M{}
	if filter != "" {
		query["action"] = filter + "-healing"
	}
	var history []healingEvent
	err = coll.Find(query).Sort("-_id").Limit(200).All(&history)
	if err != nil {
		return nil, err
	}
	return history, nil
}

func healingCountFor(action string, failingId string, duration time.Duration) (int, error) {
	coll, err := healingCollection()
	if err != nil {
		return 0, err
	}
	minStartTime := time.Now().UTC().Add(-duration)
	query := bson.M{"action": action + "-healing", "starttime": bson.M{"$gte": minStartTime}}
	maxCount := 10
	count := 0
	for count < maxCount {
		if action == "node" {
			query["creatednode._id"] = failingId
		} else {
			query["createdcontainer.id"] = failingId
		}
		var parent healingEvent
		err = coll.Find(query).One(&parent)
		if err != nil {
			if err == mgo.ErrNotFound {
				break
			}
			return 0, err
		}
		if action == "node" {
			failingId = parent.FailingNode.Address
		} else {
			failingId = parent.FailingContainer.ID
		}
		count += 1
	}
	return count, nil
}
