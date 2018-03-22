// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

var _ CacheStorage = &MockCacheStorage{}
var _ CacheService = &MockCacheService{}

// MockCacheStorage implements CacheStorage interface
type MockCacheStorage struct {
	OnPut    func(CacheEntry) error
	OnGetAll func(...string) ([]CacheEntry, error)
	OnGet    func(string) (CacheEntry, error)
}

func (m *MockCacheStorage) Put(e CacheEntry) error {
	return m.OnPut(e)
}

func (m *MockCacheStorage) GetAll(keys ...string) ([]CacheEntry, error) {
	return m.OnGetAll(keys...)
}

func (m *MockCacheStorage) Get(key string) (CacheEntry, error) {
	return m.OnGet(key)
}

// MockCacheService implements CacheService interface
type MockCacheService struct {
	OnCreate     func(CacheEntry) error
	OnList       func(...string) ([]CacheEntry, error)
	OnFindByName func(string) (CacheEntry, error)
}

func (m *MockCacheService) Create(e CacheEntry) error {
	if m.OnCreate == nil {
		return nil
	}
	return m.OnCreate(e)
}

func (m *MockCacheService) List(keys ...string) ([]CacheEntry, error) {
	if m.OnList == nil {
		return []CacheEntry{}, nil
	}
	return m.OnList(keys...)
}

func (m *MockCacheService) FindByName(k string) (CacheEntry, error) {
	if m.OnFindByName == nil {
		return CacheEntry{}, nil
	}
	return m.OnFindByName(k)
}
