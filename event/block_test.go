// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"reflect"

	check "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestAddBlock(c *check.C) {
	block := &Block{KindName: "app.deploy", Reason: "maintenance"}
	err := AddBlock(block)
	c.Assert(err, check.IsNil)
	blocks, err := listBlocks(nil)
	c.Assert(err, check.IsNil)
	c.Assert(blocks[0], check.DeepEquals, *block)
}

func (s *S) TestRemoveBlock(c *check.C) {
	block := &Block{KindName: "app.deploy", Reason: "maintenance"}
	err := AddBlock(block)
	c.Assert(err, check.IsNil)
	blocks, err := listBlocks(nil)
	c.Assert(err, check.IsNil)
	err = RemoveBlock(blocks[0].ID)
	c.Assert(err, check.IsNil)
	blocks, err = listBlocks(nil)
	c.Assert(err, check.IsNil)
	c.Assert(blocks[0].Active, check.Equals, false)
}

func (s *S) TestRemoveBlockNotFound(c *check.C) {
	err := RemoveBlock(bson.NewObjectId())
	c.Assert(err, check.NotNil)
}

func (s *S) TestListBlocks(c *check.C) {
	block := &Block{KindName: "app.deploy", Reason: "maintenance"}
	err := AddBlock(block)
	c.Assert(err, check.IsNil)
	block2 := &Block{KindName: "app.create", Reason: "maintenance"}
	err = AddBlock(block2)
	c.Assert(err, check.IsNil)
	err = RemoveBlock(block2.ID)
	c.Assert(err, check.IsNil)
	block2.Active = false
	active := true
	deactive := false
	tt := []struct {
		active   *bool
		expected []Block
	}{
		{nil, []Block{*block, *block2}},
		{&active, []Block{*block}},
		{&deactive, []Block{*block2}},
	}
	for i, t := range tt {
		blocks, err := ListBlocks(t.active)
		c.Assert(err, check.IsNil)
		if !reflect.DeepEqual(blocks, t.expected) {
			c.Errorf("(%d) Expected %#+v. Got %#+v.", i, t.expected, blocks)
		}
		c.Assert(blocks, check.DeepEquals, t.expected)
	}
}

func (s *S) TestCheckIsBlocked(c *check.C) {
	blocks := map[string]*Block{
		"blockApp":        &Block{Target: Target{Type: TargetTypeApp, Value: "blocked-app"}},
		"blockAllDeploys": &Block{KindName: "app.deploy", Reason: "maintenance"},
		"blockAllNodes":   &Block{Target: Target{Type: TargetTypeNode}},
		"blockUser":       &Block{OwnerName: "blocked-user"},
	}
	for _, b := range blocks {
		err := AddBlock(b)
		c.Assert(err, check.IsNil)
	}
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
	}
	for i, t := range tt {
		var expectedErr error
		if t.blockedBy != nil {
			expectedErr = &ErrEventBlocked{event: t.event, block: t.blockedBy}
		}
		errBlock := checkIsBlocked(t.event)
		if !reflect.DeepEqual(errBlock, expectedErr) {
			c.Errorf("(%d) Expected %#+v. Got %#+v", i, expectedErr, errBlock)
		}
	}
}
