// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tsuru/tsuru/db/storagev2"
	eventTypes "github.com/tsuru/tsuru/types/event"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	mongoBSON "go.mongodb.org/mongo-driver/bson"
)

const eventBlockCollectionName = "event_blocks"

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
	ID         primitive.ObjectID `bson:"_id,omitempty"`
	StartTime  time.Time
	EndTime    time.Time `bson:"endtime,omitempty"`
	KindName   string
	OwnerName  string
	Target     eventTypes.Target `bson:"target,omitempty"`
	Conditions map[string]string `bson:"conditions,omitempty"`
	Reason     string
	Active     bool
}

type startCustomDataMatch struct {
	Name  string
	Value string
}

func (b *Block) Blocks(e *Event) bool {
	if !(strings.HasPrefix(e.Kind.Name, b.KindName) || b.KindName == "") {
		return false
	}
	if !(e.Owner.Name == b.OwnerName || b.OwnerName == "") {
		return false
	}
	if !(e.Target == b.Target || b.Target == eventTypes.Target{} || (b.Target.Type == e.Target.Type && b.Target.Value == "")) {
		return false
	}
	if b.Conditions != nil {

		var eventCustomData []startCustomDataMatch
		e.StartCustomData.Unmarshal(&eventCustomData)
		for k, v := range b.Conditions {
			matchedFields := false
			for i := range eventCustomData {
				if eventCustomData[i].Value == v && eventCustomData[i].Name == k {
					matchedFields = true
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

func AddBlock(ctx context.Context, b *Block) error {
	collection, err := storagev2.Collection(eventBlockCollectionName)
	if err != nil {
		return err
	}
	b.Active = true
	b.ID = primitive.NewObjectID()
	b.StartTime = time.Now()

	_, err = collection.InsertOne(ctx, b)

	if err != nil {
		return err
	}

	return nil
}

func RemoveBlock(ctx context.Context, id primitive.ObjectID) error {
	collection, err := storagev2.Collection(eventBlockCollectionName)
	if err != nil {
		return err
	}
	query := mongoBSON.M{"_id": id, "active": true}

	result, err := collection.UpdateOne(ctx, query, mongoBSON.M{"$set": mongoBSON.M{"active": false, "endtime": time.Now()}})

	if err == mongo.ErrNoDocuments {
		return &ErrActiveEventBlockNotFound{id: id.Hex()}
	}

	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return &ErrActiveEventBlockNotFound{id: id.Hex()}
	}

	return err
}

func ListBlocks(ctx context.Context, active *bool) ([]Block, error) {
	query := mongoBSON.M{}
	if active != nil {
		query["active"] = *active
	}
	return listBlocks(ctx, query)
}

func listBlocks(ctx context.Context, query mongoBSON.M) ([]Block, error) {
	collection, err := storagev2.Collection(eventBlockCollectionName)
	if err != nil {
		return nil, err
	}
	var blocks []Block

	if query == nil {
		query = mongoBSON.M{}
	}

	cursor, err := collection.Find(ctx, query, options.Find().SetSort(mongoBSON.M{"starttime": -1}))
	if err != nil {
		return nil, err
	}

	err = cursor.All(ctx, &blocks)

	if err != nil {
		return nil, err
	}
	return blocks, nil
}

func checkIsBlocked(ctx context.Context, evt *Event) error {
	if evt.Target.Type == eventTypes.TargetTypeEventBlock {
		return nil
	}

	blocks, err := listBlocks(ctx, mongoBSON.M{"active": true})
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
