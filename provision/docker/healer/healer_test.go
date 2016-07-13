// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"time"

	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"gopkg.in/check.v1"
)

func (s *S) TestListHealingHistory(c *check.C) {
	evt1, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Name: "node", Value: "addr1"},
		InternalKind: "healer",
	})
	c.Assert(err, check.IsNil)
	err = evt1.Done(nil)
	c.Assert(err, check.IsNil)
	time.Sleep(100 * time.Millisecond)
	evt2, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Name: "container", Value: "cont1"},
		InternalKind: "healer",
	})
	c.Assert(err, check.IsNil)
	err = evt2.Done(nil)
	c.Assert(err, check.IsNil)
	evts, err := ListHealingHistory("")
	c.Assert(err, check.IsNil)
	c.Assert(evts, eventtest.EvtEquals, []*event.Event{evt2, evt1})
}
