// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/tsuru/tsuru/storage"
	"github.com/tsuru/tsuru/types/cache"
)

var _ cache.CacheService = &cacheService{}

type cacheService struct {
	storage cache.CacheStorage
}

func CacheService() (cache.CacheService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &cacheService{dbDriver.AppCacheStorage}, nil
}

func (s *cacheService) Create(entry cache.CacheEntry) error {
	return s.storage.Put(entry)
}

func (s *cacheService) List(keys ...string) ([]cache.CacheEntry, error) {
	return s.storage.GetAll(keys...)
}

func (s *cacheService) FindByName(key string) (cache.CacheEntry, error) {
	return s.storage.Get(key)
}
