// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import "context"

var _ CacheStorage = &MockCacheStorage{}
var _ AppCacheService = &MockAppCacheService{}

// MockCacheStorage implements CacheStorage interface
type MockCacheStorage struct {
	OnPut    func(CacheEntry) error
	OnGetAll func(...string) ([]CacheEntry, error)
	OnGet    func(string) (CacheEntry, error)
}

func (m *MockCacheStorage) Put(ctx context.Context, e CacheEntry) error {
	return m.OnPut(e)
}

func (m *MockCacheStorage) GetAll(ctx context.Context, keys ...string) ([]CacheEntry, error) {
	return m.OnGetAll(keys...)
}

func (m *MockCacheStorage) Get(ctx context.Context, key string) (CacheEntry, error) {
	return m.OnGet(key)
}

// MockAppCacheService implements AppCacheService interface
type MockAppCacheService struct {
	OnCreate     func(CacheEntry) error
	OnList       func(...string) ([]CacheEntry, error)
	OnFindByName func(string) (CacheEntry, error)
}

func (m *MockAppCacheService) Create(ctx context.Context, e CacheEntry) error {
	if m.OnCreate == nil {
		return nil
	}
	return m.OnCreate(e)
}

func (m *MockAppCacheService) List(ctx context.Context, keys ...string) ([]CacheEntry, error) {
	if m.OnList == nil {
		return []CacheEntry{}, nil
	}
	return m.OnList(keys...)
}

func (m *MockAppCacheService) FindByName(ctx context.Context, k string) (CacheEntry, error) {
	if m.OnFindByName == nil {
		return CacheEntry{}, nil
	}
	return m.OnFindByName(k)
}
