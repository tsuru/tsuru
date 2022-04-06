// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"fmt"
	"strings"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
)

const blockListLimit = 25

type ErrActiveEventBlockNotFound struct {
	id string
}

func (e *ErrActiveEventBlockNotFound) Error() string {
	return fmt.Sprintf("active event block with id %s not found", e.id)
}

type ErrEventBlocked struct {
	event *Event
	block *Block
}

func (e ErrEventBlocked) Error() string {
	return fmt.Sprintf("error running %q on %s(%s): %s",
		e.event.Kind,
		e.event.Target.Type,
		e.event.Target.Value,
		e.block,
	)
}

type Block struct {
	ID         bson.ObjectId `bson:"_id,omitempty"`
	StartTime  time.Time
	EndTime    time.Time `bson:"endtime,omitempty"`
	KindName   string
	OwnerName  string
	Target     Target            `bson:"target,omitempty"`
	Conditions map[string]string `bson:"conditions,omitempty"`
	Reason     string
	Active     bool
}

func (b *Block) Blocks(e *Event) bool {
	if !(strings.HasPrefix(e.Kind.Name, b.KindName) || b.KindName == "") {
		return false
	}
	if !(e.Owner.Name == b.OwnerName || b.OwnerName == "") {
		return false
	}
	if !(e.Target == b.Target || b.Target == Target{} || (b.Target.Type == e.Target.Type && b.Target.Value == "")) {
		return false
	}
	if b.Conditions != nil {
		var eventCustomData []map[string]interface{}
		e.StartCustomData.Unmarshal(&eventCustomData)
		for k, v := range b.Conditions {
			matchedFields := false
			for i := range eventCustomData {
				if value, ok := eventCustomData[i]["value"].(string); ok && value == v {
					if name, ok := eventCustomData[i]["name"].(string); ok && name == k {
						matchedFields = true
					}
				}
			}
			if !matchedFields {
				return false
			}
		}
	}
	return true
}

func (b *Block) String() string {
	kind := b.KindName
	if kind == "" {
		kind = "all actions"
	}
	owner := b.OwnerName
	if owner == "" {
		owner = "all users"
	}
	target := "all targets"
	if b.Target.Type != "" {
		target = b.Target.String()
	}
	return fmt.Sprintf("block %s by %s on %s: %s", kind, owner, target, b.Reason)
}

func AddBlock(b *Block) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	b.Active = true
	b.ID = bson.NewObjectId()
	b.StartTime = time.Now()
	return conn.EventBlocks().Insert(b)
}

func RemoveBlock(id bson.ObjectId) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	query := bson.M{"_id": id, "active": true}
	err = conn.EventBlocks().Update(query, bson.M{"$set": bson.M{"active": false, "endtime": time.Now()}})
	if err == mgo.ErrNotFound {
		return &ErrActiveEventBlockNotFound{id: id.Hex()}
	}
	return err
}

func ListBlocks(active *bool) ([]Block, error) {
	query := bson.M{}
	if active != nil {
		query["active"] = *active
	}
	return listBlocks(query)
}

func listBlocks(query bson.M) ([]Block, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var blocks []Block
	err = conn.EventBlocks().Find(query).Sort("-starttime").Limit(blockListLimit).All(&blocks)
	if err != nil {
		return nil, err
	}
	return blocks, nil
}

func checkIsBlocked(evt *Event) error {
	if evt.Target.Type == TargetTypeEventBlock {
		return nil
	}
	blocks, err := listBlocks(bson.M{"active": true})
	if err != nil {
		return err
	}
	for _, b := range blocks {
		if b.Blocks(evt) {
			return ErrEventBlocked{event: evt, block: &b}
		}
	}
	return nil
}
