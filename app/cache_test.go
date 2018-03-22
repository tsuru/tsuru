// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/check.v1"
)

func (s *S) TestCacheCreate(c *check.C) {
	e := appTypes.CacheEntry{
		Key:   "k1",
		Value: "v1",
	}
	service := &cacheService{
		storage: &appTypes.MockCacheStorage{
			OnPut: func(entry appTypes.CacheEntry) error {
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
		storage: &appTypes.MockCacheStorage{
			OnGetAll: func(keys ...string) ([]appTypes.CacheEntry, error) {
				return []appTypes.CacheEntry{
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
		storage: &appTypes.MockCacheStorage{
			OnGet: func(key string) (appTypes.CacheEntry, error) {
				c.Check(key, check.Equals, "k1")
				return appTypes.CacheEntry{
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
