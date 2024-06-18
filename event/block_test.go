// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"context"
	"reflect"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
	check "gopkg.in/check.v1"
)

func (s *S) TestAddBlock(c *check.C) {
	block := &Block{KindName: "app.deploy", Reason: "maintenance"}
	err := AddBlock(context.TODO(), block)
	c.Assert(err, check.IsNil)
	blocks, err := listBlocks(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	blocks[0].StartTime = block.StartTime
	c.Assert(blocks[0], check.DeepEquals, *block)
}

func (s *S) TestRemoveBlock(c *check.C) {
	block := &Block{KindName: "app.deploy", Reason: "maintenance"}
	err := AddBlock(context.TODO(), block)
	c.Assert(err, check.IsNil)
	blocks, err := listBlocks(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(blocks[0].Active, check.Equals, true)
	err = RemoveBlock(context.TODO(), blocks[0].ID)
	c.Assert(err, check.IsNil)
	blocks, err = listBlocks(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(blocks[0].Active, check.Equals, false)
	c.Assert(blocks[0].EndTime.IsZero(), check.Equals, false)
}

func (s *S) TestRemoveBlockNotFound(c *check.C) {
	err := RemoveBlock(context.TODO(), primitive.NewObjectID())
	c.Assert(err, check.NotNil)
}

func (s *S) TestListBlocks(c *check.C) {
	block := &Block{KindName: "app.deploy", Reason: "maintenance"}
	err := AddBlock(context.TODO(), block)
	c.Assert(err, check.IsNil)
	time.Sleep(100 * time.Millisecond)
	block2 := &Block{KindName: "app.create", Reason: "maintenance"}
	err = AddBlock(context.TODO(), block2)
	c.Assert(err, check.IsNil)
	err = RemoveBlock(context.TODO(), block2.ID)
	c.Assert(err, check.IsNil)
	active := true
	deactive := false
	tt := []struct {
		active   *bool
		expected []Block
	}{
		{nil, []Block{*block2, *block}},
		{&active, []Block{*block}},
		{&deactive, []Block{*block2}},
	}
	for i, t := range tt {
		blocks, err := ListBlocks(context.TODO(), t.active)
		c.Assert(err, check.IsNil)
		c.Assert(len(blocks), check.Equals, len(t.expected))
		for j := range blocks {
			if blocks[j].ID.Hex() != t.expected[j].ID.Hex() {
				c.Errorf("(%d) Expected %#+v to be in index %d. Got %#+v.", i, t.expected[j], j, blocks[j])
			}
		}
	}
}

func (s *S) TestCheckIsBlocked(c *check.C) {
	blocks := map[string]*Block{
		"blockApp":                       {Target: Target{Type: TargetTypeApp, Value: "blocked-app"}},
		"blockAllDeploys":                {KindName: "app.deploy", Reason: "maintenance"},
		"blockAllNodes":                  {Target: Target{Type: TargetTypeNode}},
		"blockUser":                      {OwnerName: "blocked-user"},
		"blockMachineTemplate":           {KindName: "machine.template"},
		"blockCreateAppInPool":           {KindName: "app.create", Conditions: map[string]string{"pool": "pool2"}},
		"blockCreateAppInPoolAndCluster": {KindName: "app.create", Conditions: map[string]string{"pool": "pool1", "cluster": "c1"}},
	}
	for _, b := range blocks {
		err := AddBlock(context.TODO(), b)
		c.Assert(err, check.IsNil)
	}
	bsonDataBlockedPoolCluster, _ := makeBSONRaw([]map[string]interface{}{{"name": "pool", "value": "pool1"}, {"name": "cluster", "value": "c1"}})
	bsonDataBlockedPool, _ := makeBSONRaw([]map[string]interface{}{{"name": "pool", "value": "pool2"}, {"name": "cluster", "value": "c2"}})
	bsonDataAllowedPool, _ := makeBSONRaw([]map[string]interface{}{{"name": "pool", "value": "pool1"}})
	bsonDataUnhandledFields, _ := makeBSONRaw([]map[string]interface{}{{"foo": "bar"}})
	tt := []struct {
		event     *Event
		blockedBy *Block
	}{
		{&Event{eventData: eventData{Kind: Kind{Name: "app.update"}}}, nil},
		{&Event{eventData: eventData{Kind: Kind{Name: "app.update"}, Target: Target{Type: TargetTypeApp, Value: "unblocked-app"}}}, nil},
		{&Event{eventData: eventData{Kind: Kind{Name: "app.update"}, Owner: Owner{Type: OwnerTypeUser, Name: "unblocked-user"}}}, nil},
		{&Event{eventData: eventData{Kind: Kind{Name: "app.deploy"}}}, blocks["blockAllDeploys"]},
		{&Event{eventData: eventData{Kind: Kind{Name: "app.update"}, Target: Target{Type: TargetTypeApp, Value: "blocked-app"}}}, blocks["blockApp"]},
		{&Event{eventData: eventData{Kind: Kind{Name: "app.update"}, Owner: Owner{Type: OwnerTypeUser, Name: "blocked-user"}}}, blocks["blockUser"]},
		{&Event{eventData: eventData{Kind: Kind{Name: "node.update"}, Target: Target{Type: TargetTypeNode, Value: "my-node"}}}, blocks["blockAllNodes"]},
		{&Event{eventData: eventData{Target: Target{Type: TargetTypeEventBlock}, Owner: Owner{Type: OwnerTypeUser, Name: "blocked-user"}}}, nil},
		{&Event{eventData: eventData{Kind: Kind{Name: "machine.template"}}}, blocks["blockMachineTemplate"]},
		{&Event{eventData: eventData{Kind: Kind{Name: "machine.template.create"}}}, blocks["blockMachineTemplate"]},
		{&Event{eventData: eventData{Kind: Kind{Name: "machine.create"}}}, nil},
		{&Event{eventData: eventData{Kind: Kind{Name: "app.create"}, Target: Target{Type: TargetTypeApp, Value: "my-app"}, StartCustomData: bsonDataBlockedPoolCluster}}, blocks["blockCreateAppInPoolAndCluster"]},
		{&Event{eventData: eventData{Kind: Kind{Name: "app.create"}, Target: Target{Type: TargetTypeApp, Value: "my-app"}, StartCustomData: bsonDataBlockedPool}}, blocks["blockCreateAppInPool"]},
		{&Event{eventData: eventData{Kind: Kind{Name: "app.create"}, Target: Target{Type: TargetTypeApp, Value: "my-app"}, StartCustomData: bsonDataAllowedPool}}, nil},
		{&Event{eventData: eventData{Kind: Kind{Name: "app.create"}, Target: Target{Type: TargetTypeApp, Value: "my-app"}, StartCustomData: bsonDataUnhandledFields}}, nil},
	}
	for i, t := range tt {
		errBlock := checkIsBlocked(context.TODO(), t.event)
		var expectedErr error
		if t.blockedBy != nil {
			if errBlock == nil {
				c.Fatalf("(%d) Expected %#+v. Got nil", i, t.blockedBy)
			}
			errBlock.(ErrEventBlocked).block.StartTime = t.blockedBy.StartTime
			expectedErr = ErrEventBlocked{event: t.event, block: t.blockedBy}
		}
		if !reflect.DeepEqual(errBlock, expectedErr) {
			c.Errorf("(%d) Expected %#+v. Got %#+v", i, expectedErr, errBlock)
		}
	}
}
