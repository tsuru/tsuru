// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quota

import "context"

var (
	_ QuotaStorage       = &MockQuotaStorage{}
	_ LegacyQuotaService = &MockQuotaService[QuotaItem]{}
)

type MockQuotaStorage struct {
	OnSet      func(string, int) error
	OnSetLimit func(string, int) error
	OnGet      func(string) (*Quota, error)
}

func (m *MockQuotaStorage) Set(ctx context.Context, name string, limit int) error {
	return m.OnSet(name, limit)
}

func (m *MockQuotaStorage) SetLimit(ctx context.Context, name string, limit int) error {
	return m.OnSetLimit(name, limit)
}

func (m *MockQuotaStorage) Get(ctx context.Context, name string) (*Quota, error) {
	return m.OnGet(name)
}

type MockQuotaService[I any] struct {
	OnInc      func(I, int) error
	OnSet      func(I, int) error
	OnSetLimit func(I, int) error
	OnGet      func(I) (*Quota, error)
}

func (m *MockQuotaService[I]) Inc(ctx context.Context, item I, delta int) error {
	if m.OnInc == nil {
		return nil
	}
	return m.OnInc(item, delta)
}

func (m *MockQuotaService[I]) SetLimit(ctx context.Context, item I, limit int) error {
	if m.OnSetLimit == nil {
		return nil
	}
	return m.OnSetLimit(item, limit)
}

func (m *MockQuotaService[I]) Set(ctx context.Context, item I, quantity int) error {
	if m.OnSet == nil {
		return nil
	}
	return m.OnSet(item, quantity)
}

func (m *MockQuotaService[I]) Get(ctx context.Context, item I) (*Quota, error) {
	return m.OnGet(item)
}
