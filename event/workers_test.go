// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tsuru/tsuru/permission"
	check "gopkg.in/check.v1"
)

func (s *S) TestUpdaterUpdatesAndStopsUpdating(c *check.C) {
	updater.stop()
	oldUpdateInterval := lockUpdateInterval
	lockUpdateInterval = time.Millisecond
	defer func() {
		updater.stop()
		lockUpdateInterval = oldUpdateInterval
	}()
	evt, err := New(context.TODO(), &Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	t0 := evts[0].LockUpdateTime
	time.Sleep(100 * time.Millisecond)
	evts, err = All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	t1 := evts[0].LockUpdateTime
	c.Assert(t0.Before(t1), check.Equals, true)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	time.Sleep(100 * time.Millisecond)
	evts, err = All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	t0 = evts[0].LockUpdateTime
	time.Sleep(100 * time.Millisecond)
	evts, err = All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	t1 = evts[0].LockUpdateTime
	c.Assert(t0, check.DeepEquals, t1)
}

func (s *S) TestUpdaterRemoveEventStress(c *check.C) {
	updater.stop()
	oldUpdateInterval := lockUpdateInterval
	lockUpdateInterval = time.Millisecond
	defer func() {
		updater.stop()
		lockUpdateInterval = oldUpdateInterval
	}()
	wg := sync.WaitGroup{}
	nGoroutines := 100
	for i := 0; i < nGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			evt, err := New(context.TODO(), &Opts{
				Target:  Target{Type: "app", Value: fmt.Sprintf("myapp-%d", i)},
				Kind:    permission.PermAppUpdateEnvSet,
				Owner:   s.token,
				Allowed: Allowed(permission.PermAppReadEvents),
			})
			c.Assert(err, check.IsNil)
			evt.Done(context.TODO(), nil)
		}(i)
	}
	wg.Wait()
	updater.stop()
	c.Assert(updater.set, check.HasLen, 0)
}

func (s *S) TestUpdaterUpdatesMultipleEvents(c *check.C) {
	updater.stop()
	oldUpdateInterval := lockUpdateInterval
	lockUpdateInterval = time.Millisecond
	defer func() {
		updater.stop()
		lockUpdateInterval = oldUpdateInterval
	}()
	evt0, err := New(context.TODO(), &Opts{
		Target:  Target{Type: "app", Value: "myapp0"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	evt1, err := New(context.TODO(), &Opts{
		Target:  Target{Type: "app", Value: "myapp1"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 2)
	t0 := evts[0].LockUpdateTime
	t1 := evts[0].LockUpdateTime
	time.Sleep(100 * time.Millisecond)
	evts, err = All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 2)
	c.Assert(t0.Before(evts[0].LockUpdateTime), check.Equals, true)
	c.Assert(t1.Before(evts[1].LockUpdateTime), check.Equals, true)
	err = evt0.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	err = evt1.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	time.Sleep(100 * time.Millisecond)
	evts, err = All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 2)
	t0 = evts[0].LockUpdateTime
	t1 = evts[1].LockUpdateTime
	time.Sleep(100 * time.Millisecond)
	evts, err = All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 2)
	c.Assert(t0, check.DeepEquals, evts[0].LockUpdateTime)
	c.Assert(t1, check.DeepEquals, evts[1].LockUpdateTime)
}

func (s *S) TestUpdaterUpdatesNonBlockingEvents(c *check.C) {
	updater.stop()
	oldUpdateInterval := lockUpdateInterval
	lockUpdateInterval = time.Millisecond
	defer func() {
		updater.stop()
		lockUpdateInterval = oldUpdateInterval
	}()
	evt, err := New(context.TODO(), &Opts{
		Target:      Target{Type: "app", Value: "myapp"},
		Kind:        permission.PermAppUpdateEnvSet,
		Owner:       s.token,
		Allowed:     Allowed(permission.PermAppReadEvents),
		DisableLock: true,
	})
	c.Assert(err, check.IsNil)
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	t0 := evts[0].LockUpdateTime
	time.Sleep(100 * time.Millisecond)
	evts, err = All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	t1 := evts[0].LockUpdateTime
	c.Assert(t0.Before(t1), check.Equals, true)
	err = evt.Done(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	time.Sleep(100 * time.Millisecond)
	evts, err = All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	t0 = evts[0].LockUpdateTime
	time.Sleep(100 * time.Millisecond)
	evts, err = All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	t1 = evts[0].LockUpdateTime
	c.Assert(t0, check.DeepEquals, t1)
}

func (s *S) TestEventCleaner(c *check.C) {
	cleaner.stop()
	oldEventCleanerInterval := eventCleanerInterval
	eventCleanerInterval = time.Millisecond
	oldLockExpire := lockExpireTimeout
	lockExpireTimeout = 100 * time.Millisecond
	defer func() {
		eventCleanerInterval = oldEventCleanerInterval
		lockExpireTimeout = oldLockExpire
	}()
	_, err := New(context.TODO(), &Opts{
		Target:  Target{Type: "app", Value: "myapp"},
		Kind:    permission.PermAppUpdateEnvSet,
		Owner:   s.token,
		Allowed: Allowed(permission.PermAppReadEvents),
	})
	c.Assert(err, check.IsNil)
	updater.stop()
	evts, err := All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Running, check.Equals, true)
	cleaner.start()
	cleaner.stop()
	evts, err = All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Running, check.Equals, true)
	time.Sleep(120 * time.Millisecond)
	cleaner.start()
	cleaner.stop()
	evts, err = All(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Running, check.Equals, false)
	c.Assert(evts[0].Error, check.Matches, `event expired, no update for .*ms`)
}
