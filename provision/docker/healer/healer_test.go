// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/provision/docker/container"
	"gopkg.in/check.v1"
)

func (s *S) TestNewHealingEventInProgress(c *check.C) {
	cont := container.Container{ID: "cont1"}
	evt1, err := NewHealingEvent(cont)
	c.Assert(err, check.IsNil)
	c.Assert(evt1.ID, check.Equals, "cont1")
	evt2, err := NewHealingEvent(cont)
	c.Assert(err, check.Equals, errHealingInProgress)
	c.Assert(evt2, check.IsNil)
	events, err := ListHealingHistory("")
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
	cont2 := container.Container{ID: "cont2"}
	err = evt1.Update(cont2, nil)
	c.Assert(err, check.IsNil)
	evt2, err = NewHealingEvent(cont)
	c.Assert(err, check.IsNil)
	c.Assert(evt2.ID, check.Equals, "cont1")
}

func (s *S) TestNewHealingEventExpired(c *check.C) {
	oldLockExpire := lockExpireTimeout
	defer func() {
		lockExpireTimeout = oldLockExpire
	}()
	lockExpireTimeout = 200 * time.Millisecond
	cont := container.Container{ID: "cont1"}
	evt1, err := NewHealingEvent(cont)
	c.Assert(err, check.IsNil)
	c.Assert(evt1.ID, check.Equals, "cont1")
	time.Sleep(500 * time.Millisecond)
	evt2, err := NewHealingEvent(cont)
	c.Assert(err, check.IsNil)
	c.Assert(evt2.ID, check.Equals, "cont1")
	events, err := ListHealingHistory("")
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 2)
	c.Assert(events[0].ID, check.Not(check.Equals), "cont1")
	c.Assert(events[0].Error, check.Matches, `healing event expired, no update for \d+\.\d+ms`)
	c.Assert(events[1].ID, check.Equals, "cont1")
}

func (s *S) TestNewHealingEventRace(c *check.C) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(10))
	successCount := int32(0)
	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cont := container.Container{ID: "cont1"}
			_, err := NewHealingEvent(cont)
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			} else if err != errHealingInProgress {
				c.Fatalf("unexpected error error: %s", err)
			}
		}()
	}
	wg.Wait()
	c.Assert(successCount, check.Equals, int32(1))
	events, err := ListHealingHistory("")
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, 1)
}

func (s *S) TestNewHealingEventUpdateRace(c *check.C) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(10))
	successCount := int32(0)
	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cont := container.Container{ID: "cont1"}
			evt, err := NewHealingEvent(cont)
			if err == nil {
				atomic.AddInt32(&successCount, 1)
				updateErr := evt.Update(nil, errors.New("my err"))
				c.Assert(updateErr, check.IsNil)
			} else if err != errHealingInProgress {
				c.Fatalf("unexpected error error: %s", err)
			}
		}()
	}
	wg.Wait()
	c.Assert(successCount > 0, check.Equals, true)
	events, err := ListHealingHistory("")
	c.Assert(err, check.IsNil)
	c.Assert(events, check.HasLen, int(successCount))
}

func (s *S) TestHealingCountFor(c *check.C) {
	conts := []container.Container{
		{ID: "cont1"}, {ID: "cont2"}, {ID: "cont3"}, {ID: "cont4"},
		{ID: "cont5"}, {ID: "cont6"}, {ID: "cont7"}, {ID: "cont8"},
	}
	for i := 0; i < len(conts)-1; i++ {
		evt, err := NewHealingEvent(conts[i])
		c.Assert(err, check.IsNil)
		err = evt.Update(conts[i+1], nil)
		c.Assert(err, check.IsNil)
	}
	count, err := healingCountFor("container", "cont8", time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 7)
}

func (s *S) TestHealingCountForOldEventsNotConsidered(c *check.C) {
	conts := []container.Container{
		{ID: "cont1"}, {ID: "cont2"}, {ID: "cont3"}, {ID: "cont4"},
		{ID: "cont5"}, {ID: "cont6"}, {ID: "cont7"}, {ID: "cont8"},
	}
	for i := 0; i < len(conts)-1; i++ {
		evt, err := NewHealingEvent(conts[i])
		c.Assert(err, check.IsNil)
		err = evt.Update(conts[i+1], nil)
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
		evt, err := NewHealingEvent(nodes[i])
		c.Assert(err, check.IsNil)
		err = evt.Update(nodes[i+1], nil)
		c.Assert(err, check.IsNil)
	}
	count, err := healingCountFor("node", "addr8", time.Minute)
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 7)
}
