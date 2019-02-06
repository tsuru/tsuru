// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/healer"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/types"
	check "gopkg.in/check.v1"
)

func mongoTime(t time.Time) time.Time {
	return time.Unix(0, int64((time.Duration(t.UnixNano())/time.Millisecond)*time.Millisecond)).UTC()
}

func (s *S) TestListHealingHistory(c *check.C) {
	evt1, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Type: "node", Value: "addr1"},
		InternalKind: "healer",
		Allowed:      event.Allowed(permission.PermPoolReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt1.Done(nil)
	c.Assert(err, check.IsNil)
	time.Sleep(100 * time.Millisecond)
	evt2, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Type: "container", Value: "cont1"},
		InternalKind: "healer",
		Allowed:      event.Allowed(permission.PermPoolReadEvents),
	})
	c.Assert(err, check.IsNil)
	err = evt2.Done(nil)
	c.Assert(err, check.IsNil)
	evts, err := ListHealingHistory("")
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.DeepEquals, []types.HealingEvent{
		{
			ID:         evt2.UniqueID,
			StartTime:  mongoTime(evt2.StartTime),
			EndTime:    mongoTime(evt2.EndTime),
			Action:     "container-healing",
			Successful: true,
			Error:      "",
		},
		{
			ID:         evt1.UniqueID,
			StartTime:  mongoTime(evt1.StartTime),
			EndTime:    mongoTime(evt1.EndTime),
			Action:     "node-healing",
			Successful: true,
			Error:      "",
		},
	})
}

func (s *S) TestMigrateHealingToEvents(c *check.C) {
	now := mongoTime(time.Now())
	evts := []types.HealingEvent{
		{
			ID:               bson.NewObjectId(),
			StartTime:        now.Add(-time.Minute),
			EndTime:          now,
			Action:           "container-healing",
			Successful:       false,
			Error:            "x",
			FailingContainer: types.Container{ID: "c1"},
			CreatedContainer: types.Container{ID: "c2"},
		},
		{
			ID:         bson.NewObjectId(),
			StartTime:  now,
			EndTime:    now.Add(time.Minute),
			Action:     "node-healing",
			Successful: false,
			Error:      "y",
			FailingNode: provision.NodeSpec{
				Address:  "addr1",
				Metadata: map[string]string{},
			},
			CreatedNode: provision.NodeSpec{
				Address:  "addr2",
				Metadata: map[string]string{},
			},
			Reason: "r2",
			Extra:  &healer.NodeChecks{Time: now, Checks: []provision.NodeCheckResult{{Name: "a"}}},
		},
	}
	coll, err := oldHealingCollection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	for _, evt := range evts {
		err = coll.Insert(evt)
		c.Assert(err, check.IsNil)
	}
	err = MigrateHealingToEvents()
	c.Assert(err, check.IsNil)
	dbEvts, err := ListHealingHistory("")
	c.Assert(err, check.IsNil)
	c.Assert(dbEvts, check.DeepEquals, []types.HealingEvent{evts[1], evts[0]})
}
