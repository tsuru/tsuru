// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"time"

	"github.com/pkg/errors"
)

var (
	ErrEntryNotFound = errors.New("cache entry not found")
)

type CacheEntry struct {
	Key      string
	Value    string
	ExpireAt time.Time
}

type CacheService interface {
	Create(entry CacheEntry) error
	List(keys ...string) ([]CacheEntry, error)
	FindByName(key string) (CacheEntry, error)
}

type CacheStorage interface {
	GetAll(keys ...string) ([]CacheEntry, error)
	Get(key string) (CacheEntry, error)
	Put(entry CacheEntry) error
}
