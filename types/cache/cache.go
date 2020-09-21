// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"context"
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

type AppCacheService interface {
	Create(ctx context.Context, entry CacheEntry) error
	List(ctx context.Context, keys ...string) ([]CacheEntry, error)
	FindByName(ctx context.Context, key string) (CacheEntry, error)
}

type CacheStorage interface {
	GetAll(ctx context.Context, keys ...string) ([]CacheEntry, error)
	Get(ctx context.Context, key string) (CacheEntry, error)
	Put(ctx context.Context, entry CacheEntry) error
}
