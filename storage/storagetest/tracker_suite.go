// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"time"

	"github.com/tsuru/tsuru/types/tracker"
	check "gopkg.in/check.v1"
)

type InstanceTrackerSuite struct {
	SuiteHooks
	InstanceTrackerStorage tracker.InstanceStorage
}

func (s *InstanceTrackerSuite) Test_Notify_List(c *check.C) {
	t0 := time.Now().Truncate(time.Millisecond)
	err := s.InstanceTrackerStorage.Notify(tracker.TrackedInstance{
		Name:      "host1",
		Addresses: []string{"10.0.0.1", "10.0.0.2"},
	})
	c.Assert(err, check.IsNil)
	instances, err := s.InstanceTrackerStorage.List(500 * time.Millisecond)
	c.Assert(err, check.IsNil)
	c.Assert(instances, check.HasLen, 1)
	c.Assert(instances[0].LastUpdate.After(t0) || instances[0].LastUpdate.Equal(t0), check.Equals, true)
	c.Assert(instances[0].LastUpdate.Before(time.Now()), check.Equals, true)
	instances[0].LastUpdate = time.Time{}
	c.Assert(instances, check.DeepEquals, []tracker.TrackedInstance{
		{Name: "host1", Addresses: []string{"10.0.0.1", "10.0.0.2"}},
	})
}

func (s *InstanceTrackerSuite) Test_Notify_UpdateAddrs(c *check.C) {
	err := s.InstanceTrackerStorage.Notify(tracker.TrackedInstance{
		Name:      "host1",
		Addresses: []string{"10.0.0.1", "10.0.0.2"},
	})
	c.Assert(err, check.IsNil)
	instances, err := s.InstanceTrackerStorage.List(500 * time.Millisecond)
	c.Assert(err, check.IsNil)
	c.Assert(instances, check.HasLen, 1)
	c.Assert(instances[0].Addresses, check.DeepEquals, []string{"10.0.0.1", "10.0.0.2"})
	err = s.InstanceTrackerStorage.Notify(tracker.TrackedInstance{
		Name:      "host1",
		Addresses: []string{"192.168.1.2"},
	})
	c.Assert(err, check.IsNil)
	instances, err = s.InstanceTrackerStorage.List(500 * time.Millisecond)
	c.Assert(err, check.IsNil)
	c.Assert(instances, check.HasLen, 1)
	c.Assert(instances[0].Addresses, check.DeepEquals, []string{"192.168.1.2"})
}

func (s *InstanceTrackerSuite) Test_Notify_List_StaleEntry(c *check.C) {
	err := s.InstanceTrackerStorage.Notify(tracker.TrackedInstance{
		Name:      "host1",
		Addresses: []string{"10.0.0.1", "10.0.0.2"},
	})
	c.Assert(err, check.IsNil)
	time.Sleep(time.Second)
	instances, err := s.InstanceTrackerStorage.List(500 * time.Millisecond)
	c.Assert(err, check.IsNil)
	c.Assert(instances, check.HasLen, 0)
	err = s.InstanceTrackerStorage.Notify(tracker.TrackedInstance{
		Name:      "host1",
		Addresses: []string{"10.0.0.1", "10.0.0.2"},
	})
	c.Assert(err, check.IsNil)
	instances, err = s.InstanceTrackerStorage.List(500 * time.Millisecond)
	c.Assert(err, check.IsNil)
	c.Assert(instances, check.HasLen, 1)
}
