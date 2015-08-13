// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"io"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/safe"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

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
	logBuffer     safe.Buffer
	writer        io.Writer
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

func newAutoScaleEvent(metadataValue string, writer io.Writer) (*autoScaleEvent, error) {
	// Use metadataValue as ID to ensure only one auto scale process runs for
	// each metadataValue. (*autoScaleEvent).finish() will generate a new
	// unique ID and remove this initial record.
	evt := autoScaleEvent{
		ID:            metadataValue,
		StartTime:     time.Now().UTC(),
		MetadataValue: metadataValue,
		writer:        writer,
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

func (evt *autoScaleEvent) updateNodes(nodes []cluster.Node) {
	evt.Nodes = nodes
}

func (evt *autoScaleEvent) logMsg(msg string, params ...interface{}) {
	log.Debugf(fmt.Sprintf("[node autoscale] %s", msg), params...)
	msg += "\n"
	if evt.writer != nil {
		fmt.Fprintf(evt.writer, msg, params...)
	}
	fmt.Fprintf(&evt.logBuffer, msg, params...)
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

func (evt *autoScaleEvent) finish(errParam error) error {
	if errParam != nil {
		evt.Error = errParam.Error()
		evt.logMsg(evt.Error)
	}
	coll, err := autoScaleCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	if evt.Action == "" {
		return coll.RemoveId(evt.ID)
	}
	evt.Log = evt.logBuffer.String()
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
