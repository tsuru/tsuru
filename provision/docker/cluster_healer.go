// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	"gopkg.in/mgo.v2/bson"
)

type Healer struct {
	cluster               *cluster.Cluster
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

func (h *Healer) healNode(node *cluster.Node) (cluster.Node, error) {
	emptyNode := cluster.Node{}
	failingAddr := node.Address
	nodeMetadata := node.CleanMetadata()
	failingHost := urlToHost(failingAddr)
	failures := node.FailureCount()
	iaasName, hasIaas := nodeMetadata["iaas"]
	if !hasIaas {
		return emptyNode, fmt.Errorf("Can't auto-heal after %d failures for node %s: no IaaS information.", failures, failingHost)
	}
	machine, err := iaas.CreateMachineForIaaS(iaasName, nodeMetadata)
	if err != nil {
		node.ResetFailures()
		return emptyNode, fmt.Errorf("Can't auto-heal after %d failures for node %s: error creating new machine: %s", failures, failingHost, err.Error())
	}
	newAddr, err := machine.FormatNodeAddress()
	if err != nil {
		machine.Destroy()
		return emptyNode, fmt.Errorf("Can't auto-heal after %d failures for node %s: error formatting address: %s", failures, failingHost, err.Error())
	}
	err = h.cluster.Unregister(failingAddr)
	if err != nil {
		machine.Destroy()
		return emptyNode, fmt.Errorf("Can't auto-heal after %d failures for node %s: error unregistering old node: %s", failures, failingHost, err.Error())
	}
	log.Debugf("New machine created during healing process: %s - Waiting for docker to start...", newAddr)
	createdNode, err := h.cluster.WaitAndRegister(newAddr, nodeMetadata, h.waitTimeNewMachine)
	if err != nil {
		node.ResetFailures()
		h.cluster.Register(failingAddr, nodeMetadata)
		machine.Destroy()
		return emptyNode, fmt.Errorf("Can't auto-heal after %d failures for node %s: error registering new node: %s", failures, failingHost, err.Error())
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	err = moveContainers(failingHost, "", encoder)
	if err != nil {
		log.Errorf("Unable to move containers, skipping containers healing %q -> %q: %s: %s", failingHost, machine.Address, err.Error(), buf.String())
	}
	failingMachine, err := iaas.FindMachineByAddress(failingHost)
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

func (h *Healer) HandleError(node *cluster.Node) time.Duration {
	failures := node.FailureCount()
	if failures < h.failuresBeforeHealing {
		log.Debugf("%d failures detected in node %q, waiting for more failures before healing.", failures, node.Address)
		return h.disabledTime
	}
	if !node.HasSuccess() {
		log.Debugf("Node %q has never been successfully reached, healing won't run on it.", node.Address)
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

func healContainer(cont container) (container, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	createdContainer, err := moveContainer(cont.ID, "", encoder)
	if err != nil {
		err = fmt.Errorf("Error trying to heal containers %s: couldn't move container: %s - %s", cont.ID, err.Error(), buf.String())
	}
	return createdContainer, err
}

func runContainerHealer(maxUnresponsiveTime time.Duration) {
	for {
		containers, err := listUnresponsiveContainers(maxUnresponsiveTime)
		if err != nil {
			log.Errorf("Containers Healing: couldn't list unresponsive containers: %s", err.Error())
		}
		for _, cont := range containers {
			if cont.LastSuccessStatusUpdate.IsZero() {
				continue
			}
			log.Errorf("Initiating healing process for container %s, unresponsive since %s.", cont.ID, cont.LastSuccessStatusUpdate)
			evt, err := newHealingEvent(cont)
			if err != nil {
				log.Errorf("Error trying to insert container healing event: %s", err.Error())
			}
			newCont, err := healContainer(cont)
			if err != nil {
				log.Errorf("Error containers healing: %s", err.Error())
			}
			err = evt.update(newCont, err)
			if err != nil {
				log.Errorf("Error trying to update containers healing event: %s", err.Error())
			}
		}
		time.Sleep(30 * time.Second)
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
