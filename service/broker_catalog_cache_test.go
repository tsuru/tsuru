// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/tsuru/tsuru/types/cache"
	"github.com/tsuru/tsuru/types/service"
	check "gopkg.in/check.v1"
)

func (s *S) TestCacheSaveDefaultExpiration(c *check.C) {
	catalog := service.BrokerCatalog{
		Services: []service.BrokerService{{
			ID:          "123",
			Name:        "service1",
			Description: "my service",
			Plans: []service.BrokerPlan{{
				ID:          "456",
				Name:        "my-plan",
				Description: "plan description",
			}},
		}},
	}
	service := &serviceBrokerCatalogCacheService{
		storage: &cache.MockCacheStorage{
			OnPut: func(entry cache.CacheEntry) error {
				c.Assert(entry.Key, check.Equals, "my-catalog")
				expiration := time.Until(entry.ExpireAt)
				c.Assert(expiration > 0, check.Equals, true)
				c.Assert(expiration <= defaultExpiration, check.Equals, true)
				var cat service.BrokerCatalog
				err := json.Unmarshal([]byte(entry.Value), &cat)
				c.Assert(err, check.IsNil)
				c.Assert(cat, check.DeepEquals, catalog)
				return nil
			},
		},
	}
	err := service.Save(context.TODO(), "my-catalog", catalog)
	c.Assert(err, check.IsNil)
}

func (s *S) TestCacheSaveNegativeExpiration(c *check.C) {
	var calls int32
	s.mockService.ServiceBroker.OnFind = func(name string) (service.Broker, error) {
		atomic.AddInt32(&calls, 1)
		return service.Broker{
			Name: name,
			Config: service.BrokerConfig{
				CacheExpirationSeconds: -1,
			},
		}, nil
	}
	catalog := service.BrokerCatalog{
		Services: []service.BrokerService{{
			ID:   "123",
			Name: "service1",
		}},
	}
	service := &serviceBrokerCatalogCacheService{
		storage: &cache.MockCacheStorage{
			OnPut: func(entry cache.CacheEntry) error {
				atomic.AddInt32(&calls, 1)
				expiration := time.Until(entry.ExpireAt)
				c.Assert(expiration > 0, check.Equals, true)
				c.Assert(expiration <= defaultExpiration, check.Equals, true)
				return nil
			},
		},
	}
	err := service.Save(context.TODO(), "my-catalog", catalog)
	c.Assert(err, check.IsNil)
	c.Assert(atomic.LoadInt32(&calls), check.Equals, int32(2))
}

func (s *S) TestCacheSaveCustomExpiration(c *check.C) {
	var calls int32
	duration := 300
	s.mockService.ServiceBroker.OnFind = func(name string) (service.Broker, error) {
		atomic.AddInt32(&calls, 1)
		return service.Broker{
			Name: name,
			Config: service.BrokerConfig{
				CacheExpirationSeconds: duration,
			},
		}, nil
	}
	catalog := service.BrokerCatalog{
		Services: []service.BrokerService{{
			ID:   "123",
			Name: "service1",
		}},
	}
	service := &serviceBrokerCatalogCacheService{
		storage: &cache.MockCacheStorage{
			OnPut: func(entry cache.CacheEntry) error {
				atomic.AddInt32(&calls, 1)
				c.Assert(time.Until(entry.ExpireAt) <= time.Duration(time.Duration(duration)*time.Second), check.Equals, true)
				return nil
			},
		},
	}
	err := service.Save(context.TODO(), "my-catalog", catalog)
	c.Assert(err, check.IsNil)
	c.Assert(atomic.LoadInt32(&calls), check.Equals, int32(2))
}

func (s *S) TestCacheLoad(c *check.C) {
	catalog := service.BrokerCatalog{
		Services: []service.BrokerService{{
			ID:          "123",
			Name:        "service1",
			Description: "my service",
			Plans: []service.BrokerPlan{{
				ID:          "456",
				Name:        "my-plan",
				Description: "plan description",
			}},
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
	cat, err := service.Load(context.TODO(), "my-catalog")
	c.Assert(err, check.IsNil)
	c.Assert(cat, check.NotNil)
	c.Assert(*cat, check.DeepEquals, catalog)
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
	cat, err := service.Load(context.TODO(), "unknown-catalog")
	c.Assert(err, check.NotNil)
	c.Assert(cat, check.IsNil)
}
