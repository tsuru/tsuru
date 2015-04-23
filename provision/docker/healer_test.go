// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"time"

	"github.com/tsuru/docker-cluster/cluster"
	"gopkg.in/check.v1"
)

func (s *S) TestHealingCountFor(c *check.C) {
	conts := []container{
		{ID: "cont1"}, {ID: "cont2"}, {ID: "cont3"}, {ID: "cont4"},
		{ID: "cont5"}, {ID: "cont6"}, {ID: "cont7"}, {ID: "cont8"},
	}
	for i := 0; i < len(conts)-1; i++ {
		evt, err := newHealingEvent(conts[i])
		c.Assert(err, check.IsNil)
		err = evt.update(conts[i+1], nil)
		c.Assert(err, check.IsNil)
	}
	count, err := healingCountFor("container", "cont8", time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 7)
}

func (s *S) TestHealingCountForOldEventsNotConsidered(c *check.C) {
	conts := []container{
		{ID: "cont1"}, {ID: "cont2"}, {ID: "cont3"}, {ID: "cont4"},
		{ID: "cont5"}, {ID: "cont6"}, {ID: "cont7"}, {ID: "cont8"},
	}
	for i := 0; i < len(conts)-1; i++ {
		evt, err := newHealingEvent(conts[i])
		c.Assert(err, check.IsNil)
		err = evt.update(conts[i+1], nil)
		c.Assert(err, check.IsNil)
		if i < 4 {
			coll, err := healingCollection()
			c.Assert(err, check.IsNil)
			defer coll.Close()
			evt.StartTime = time.Now().UTC().Add(-2 * time.Minute)
			err = coll.UpdateId(evt.ID, evt)
			c.Assert(err, check.IsNil)
		}
	}
	count, err := healingCountFor("container", "cont8", time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 3)
}

func (s *S) TestHealingCountForWithNode(c *check.C) {
	nodes := []cluster.Node{
		{Address: "addr1"}, {Address: "addr2"}, {Address: "addr3"}, {Address: "addr4"},
		{Address: "addr5"}, {Address: "addr6"}, {Address: "addr7"}, {Address: "addr8"},
	}
	for i := 0; i < len(nodes)-1; i++ {
		evt, err := newHealingEvent(nodes[i])
		c.Assert(err, check.IsNil)
		err = evt.update(nodes[i+1], nil)
		c.Assert(err, check.IsNil)
	}
	count, err := healingCountFor("node", "addr8", time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 7)
}
