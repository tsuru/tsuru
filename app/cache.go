// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"

	"github.com/tsuru/tsuru/storage"
	"github.com/tsuru/tsuru/types/cache"
)

var _ cache.AppCacheService = (*cacheService)(nil)

type cacheService struct {
	storage cache.CacheStorage
}

func CacheService() (cache.AppCacheService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &cacheService{dbDriver.AppCacheStorage}, nil
}

func (s *cacheService) Create(ctx context.Context, entry cache.CacheEntry) error {
	return s.storage.Put(ctx, entry)
}

func (s *cacheService) List(ctx context.Context, keys ...string) ([]cache.CacheEntry, error) {
	return s.storage.GetAll(ctx, keys...)
}

func (s *cacheService) FindByName(ctx context.Context, key string) (cache.CacheEntry, error) {
	return s.storage.Get(ctx, key)
}
