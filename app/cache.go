// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/tsuru/tsuru/storage"
	appTypes "github.com/tsuru/tsuru/types/app"
)

var _ appTypes.CacheService = &cacheService{}

type cacheService struct {
	storage appTypes.CacheStorage
}

func CacheService() (appTypes.CacheService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	return &cacheService{dbDriver.CacheStorage}, nil
}

func (s *cacheService) Create(entry appTypes.CacheEntry) error {
	return s.storage.Put(entry)
}

func (s *cacheService) List(keys ...string) ([]appTypes.CacheEntry, error) {
	return s.storage.GetAll(keys...)
}

func (s *cacheService) FindByName(key string) (appTypes.CacheEntry, error) {
	return s.storage.Get(key)
}
