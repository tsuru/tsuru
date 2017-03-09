// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type ErrEventBlocked struct {
	event *Event
	block *Block
}

func (e *ErrEventBlocked) Error() string {
	return fmt.Sprintf("error running %q on %s(%s): %s",
		e.event.Kind,
		e.event.Target.Type,
		e.event.Target.Value,
		e.block,
	)
}

type Block struct {
	ID        bson.ObjectId `bson:"_id,omitempty"`
	KindName  string
	OwnerName string
	Target    Target `bson:"target,omitempty"`
	Reason    string
	Active    bool
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
	return conn.EventBlocks().Insert(b)
}

func RemoveBlock(id bson.ObjectId) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.EventBlocks().UpdateId(id, bson.M{"$set": bson.M{"active": false}})
	if err == mgo.ErrNotFound {
		return errors.WithMessage(err, "failed to remove event block")
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
	err = conn.EventBlocks().Find(query).All(&blocks)
	if err != nil {
		return nil, err
	}
	return blocks, nil
}

func checkIsBlocked(evt *Event) error {
	query := bson.M{"$and": []bson.M{
		{"active": true},
		{"$or": []bson.M{{"kindname": evt.Kind.Name}, {"kindname": ""}}},
		{"$or": []bson.M{{"ownername": evt.Owner.Name}, {"ownername": ""}}},
		{"$or": []bson.M{
			{"target": evt.Target},
			{"target": bson.M{"$exists": false}},
			{"$and": []bson.M{{"target.type": evt.Target.Type}, {"target.value": ""}}}}},
	}}
	blocks, err := listBlocks(query)
	if err != nil {
		return err
	}
	if len(blocks) > 0 {
		return &ErrEventBlocked{event: evt, block: &blocks[0]}
	}
	return nil
}
