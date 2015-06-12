// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

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

func listHealingHistory(filter string) ([]healingEvent, error) {
	coll, err := healingCollection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
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
