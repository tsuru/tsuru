// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"errors"
	"fmt"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision/docker/container"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type HealingEvent struct {
	ID               interface{} `bson:"_id"`
	StartTime        time.Time
	EndTime          time.Time `bson:",omitempty"`
	Action           string
	Reason           string
	Extra            interface{}
	FailingNode      cluster.Node        `bson:",omitempty"`
	CreatedNode      cluster.Node        `bson:",omitempty"`
	FailingContainer container.Container `bson:",omitempty"`
	CreatedContainer container.Container `bson:",omitempty"`
	Successful       bool
	Error            string `bson:",omitempty"`
	LockUpdateTime   time.Time
	done             chan bool
}

var (
	consecutiveHealingsTimeframe        = 30 * time.Minute
	consecutiveHealingsLimitInTimeframe = 3
	lockUpdateInterval                  = 30 * time.Second
	lockExpireTimeout                   = 5 * time.Minute

	errHealingInProgress = errors.New("healing already in progress")
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

func NewHealingEventWithReason(failing interface{}, reason string, extra interface{}) (*HealingEvent, error) {
	now := time.Now().UTC()
	evt := HealingEvent{
		StartTime:      now,
		LockUpdateTime: now,
		Reason:         reason,
		Extra:          extra,
		done:           make(chan bool),
	}
	switch v := failing.(type) {
	case cluster.Node:
		evt.ID = v.Address
		evt.Action = "node-healing"
		evt.FailingNode = v
	case container.Container:
		evt.ID = v.ID
		evt.Action = "container-healing"
		evt.FailingContainer = v
	default:
		return nil, fmt.Errorf("invalid healing object: %#v", failing)
	}
	coll, err := healingCollection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	maxRetries := 1
	for i := 0; i < maxRetries+1; i++ {
		err = coll.Insert(evt)
		if err == nil {
			go evt.startLockUpdate(evt.ID)
			return &evt, nil
		}
		if mgo.IsDup(err) {
			err = errHealingInProgress
			if i < maxRetries && checkIsExpired(coll, evt.ID) {
				coll.RemoveId(evt.ID)
			}
		}
	}
	return nil, err
}

func checkIsExpired(coll *storage.Collection, id interface{}) bool {
	var existingEvt HealingEvent
	err := coll.FindId(id).One(&existingEvt)
	if err == nil {
		now := time.Now().UTC()
		lastUpdate := existingEvt.LockUpdateTime.UTC()
		if now.After(lastUpdate.Add(lockExpireTimeout)) {
			existingEvt.Update(nil, fmt.Errorf("healing event expired, no update for %v", time.Since(lastUpdate)))
			return true
		}
	}
	return false
}

func NewHealingEvent(failing interface{}) (*HealingEvent, error) {
	return NewHealingEventWithReason(failing, "", nil)
}

func (evt *HealingEvent) startLockUpdate(id interface{}) {
	for {
		select {
		case <-time.After(lockUpdateInterval):
		case <-evt.done:
			return
		}
		coll, err := healingCollection()
		if err == nil {
			err = coll.UpdateId(id, bson.M{"$set": bson.M{"lockupdatetime": time.Now().UTC()}})
			coll.Close()
			if err == mgo.ErrNotFound {
				<-evt.done
				return
			}
		}
	}
}

func (evt *HealingEvent) Update(created interface{}, healingErr error) error {
	coll, err := healingCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	if evt.done != nil {
		evt.done <- true
		close(evt.done)
	}
	if created == nil && healingErr == nil {
		return coll.RemoveId(evt.ID)
	}
	if healingErr != nil {
		evt.Error = healingErr.Error()
	}
	evt.EndTime = time.Now().UTC()
	switch v := created.(type) {
	case cluster.Node:
		evt.CreatedNode = v
		evt.Successful = v.Address != ""
	case container.Container:
		evt.CreatedContainer = v
		evt.Successful = v.ID != ""
	}
	defer coll.RemoveId(evt.ID)
	evt.ID = bson.NewObjectId()
	return coll.Insert(evt)
}

func ListHealingHistory(filter string) ([]HealingEvent, error) {
	coll, err := healingCollection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	query := bson.M{}
	if filter != "" {
		query["action"] = filter + "-healing"
	}
	var history []HealingEvent
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
	defer coll.Close()
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
		var parent HealingEvent
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
