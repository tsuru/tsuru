// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"sort"
	"time"

	"github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

type CacheSuite struct {
	SuiteHooks
	CacheStorage app.CacheStorage
}

func (s *CacheSuite) TestCachePut(c *check.C) {
	err := s.CacheStorage.Put(app.CacheEntry{
		Key:   "k1",
		Value: "v1",
	})
	c.Assert(err, check.IsNil)
	entry, err := s.CacheStorage.Get("k1")
	c.Assert(err, check.IsNil)
	c.Assert(entry, check.DeepEquals, app.CacheEntry{
		Key:   "k1",
		Value: "v1",
	})
}

func (s *CacheSuite) TestCacheGetNotFound(c *check.C) {
	entry, err := s.CacheStorage.Get("k1")
	c.Assert(err, check.Equals, app.ErrEntryNotFound)
	c.Assert(entry, check.DeepEquals, app.CacheEntry{})
}

func (s *CacheSuite) TestCacheGetAll(c *check.C) {
	err := s.CacheStorage.Put(app.CacheEntry{
		Key:   "k1",
		Value: "v1",
	})
	c.Assert(err, check.IsNil)
	err = s.CacheStorage.Put(app.CacheEntry{
		Key:   "k2",
		Value: "v2",
	})
	c.Assert(err, check.IsNil)
	err = s.CacheStorage.Put(app.CacheEntry{
		Key:   "k3",
		Value: "v3",
	})
	c.Assert(err, check.IsNil)
	entries, err := s.CacheStorage.GetAll("k1", "k3")
	c.Assert(err, check.IsNil)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})
	c.Assert(entries, check.DeepEquals, []app.CacheEntry{
		{Key: "k1", Value: "v1"},
		{Key: "k3", Value: "v3"},
	})
	entries, err = s.CacheStorage.GetAll("kx")
	c.Assert(err, check.IsNil)
	c.Assert(entries, check.HasLen, 0)
}

func (s *CacheSuite) TestCacheExpiration(c *check.C) {
	err := s.CacheStorage.Put(app.CacheEntry{
		Key:      "k1",
		Value:    "v1",
		ExpireAt: time.Now().Add(time.Second),
	})
	c.Assert(err, check.IsNil)
	entry, err := s.CacheStorage.Get("k1")
	c.Assert(err, check.IsNil)
	c.Assert(entry.Value, check.Equals, "v1")
	timeout := time.After(70 * time.Second)
	for {
		_, err = s.CacheStorage.Get("k1")
		if err != nil {
			c.Assert(err, check.Equals, app.ErrEntryNotFound)
			break
		}
		select {
		case <-time.After(500 * time.Millisecond):
		case <-timeout:
			c.Fatal("timeout waiting for key to expire")
		}
	}

}
