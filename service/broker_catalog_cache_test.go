// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"encoding/json"
	"fmt"

	osb "github.com/pmorie/go-open-service-broker-client/v2"
	"github.com/tsuru/tsuru/types/cache"
	"gopkg.in/check.v1"
)

func (s *S) TestCacheSave(c *check.C) {
	catalog := osb.CatalogResponse{
		Services: []osb.Service{{
			ID:          "123",
			Name:        "service1",
			Description: "my service",
		}},
	}
	service := &serviceBrokerCatalogCacheService{
		storage: &cache.MockCacheStorage{
			OnPut: func(entry cache.CacheEntry) error {
				c.Assert(entry.Key, check.Equals, "my-catalog")
				var cat osb.CatalogResponse
				err := json.Unmarshal([]byte(entry.Value), &cat)
				c.Assert(err, check.IsNil)
				c.Assert(cat.Services, check.HasLen, 1)
				c.Assert(cat.Services[0].ID, check.Equals, catalog.Services[0].ID)
				c.Assert(cat.Services[0].Name, check.Equals, catalog.Services[0].Name)
				c.Assert(cat.Services[0].Description, check.Equals, catalog.Services[0].Description)
				return nil
			},
		},
	}
	err := service.Save("my-catalog", catalog)
	c.Assert(err, check.IsNil)
}

func (s *S) TestCacheLoad(c *check.C) {
	catalog := osb.CatalogResponse{
		Services: []osb.Service{{
			ID:          "123",
			Name:        "service1",
			Description: "my service",
		}},
	}
	service := &serviceBrokerCatalogCacheService{
		storage: &cache.MockCacheStorage{
			OnGet: func(key string) (cache.CacheEntry, error) {
				c.Assert(key, check.Equals, "my-catalog")
				b, err := json.Marshal(catalog)
				c.Assert(err, check.IsNil)
				return cache.CacheEntry{Key: key, Value: string(b)}, nil
			},
		},
	}
	cat, err := service.Load("my-catalog")
	c.Assert(err, check.IsNil)
	c.Assert(cat.Services, check.HasLen, 1)
	c.Assert(cat.Services[0].ID, check.Equals, catalog.Services[0].ID)
	c.Assert(cat.Services[0].Name, check.Equals, catalog.Services[0].Name)
	c.Assert(cat.Services[0].Description, check.Equals, catalog.Services[0].Description)
}

func (s *S) TestCacheLoadNotFound(c *check.C) {
	service := &serviceBrokerCatalogCacheService{
		storage: &cache.MockCacheStorage{
			OnGet: func(key string) (cache.CacheEntry, error) {
				c.Assert(key, check.Equals, "unknown-catalog")
				return cache.CacheEntry{}, fmt.Errorf("not found")
			},
		},
	}
	cat, err := service.Load("unknown-catalog")
	c.Assert(err, check.NotNil)
	c.Assert(cat, check.IsNil)
}
