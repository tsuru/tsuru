// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/tsuru/tsuru/types/cache"
	"gopkg.in/check.v1"
)

func (s *S) TestCacheCreate(c *check.C) {
	e := cache.CacheEntry{
		Key:   "k1",
		Value: "v1",
	}
	service := &cacheService{
		storage: &cache.MockCacheStorage{
			OnPut: func(entry cache.CacheEntry) error {
				c.Assert(e, check.Equals, entry)
				return nil
			},
		},
	}
	err := service.Create(e)
	c.Assert(err, check.IsNil)
}

func (s *S) TestCacheList(c *check.C) {
	service := &cacheService{
		storage: &cache.MockCacheStorage{
			OnGetAll: func(keys ...string) ([]cache.CacheEntry, error) {
				return []cache.CacheEntry{
					{Key: "k1", Value: "v1"},
					{Key: "k2", Value: "v2"},
				}, nil
			},
		},
	}
	cs, err := service.List("k1", "k2")
	c.Assert(err, check.IsNil)
	c.Assert(cs, check.HasLen, 2)
}

func (s *S) TestCacheFindByName(c *check.C) {
	service := &cacheService{
		storage: &cache.MockCacheStorage{
			OnGet: func(key string) (cache.CacheEntry, error) {
				c.Check(key, check.Equals, "k1")
				return cache.CacheEntry{
					Key:   "k1",
					Value: "v1",
				}, nil
			},
		},
	}
	entry, err := service.FindByName("k1")
	c.Assert(err, check.IsNil)
	c.Assert(entry.Key, check.Equals, "k1")
	c.Assert(entry.Value, check.Equals, "v1")
}
