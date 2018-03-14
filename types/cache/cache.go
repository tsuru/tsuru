// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

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

type CacheStorage interface {
	GetAll(keys ...string) ([]CacheEntry, error)
	Get(key string) (CacheEntry, error)
	Put(entry CacheEntry) error
}
