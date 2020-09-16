// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quota

var (
	_ QuotaStorage = &MockQuotaStorage{}
	_ QuotaService = &MockQuotaService{}
)

type MockQuotaStorage struct {
	OnSet      func(string, int) error
	OnSetLimit func(string, int) error
	OnGet      func(string) (*Quota, error)
}

func (m *MockQuotaStorage) Set(name string, limit int) error {
	return m.OnSet(name, limit)
}

func (m *MockQuotaStorage) SetLimit(name string, limit int) error {
	return m.OnSetLimit(name, limit)
}

func (m *MockQuotaStorage) Get(name string) (*Quota, error) {
	return m.OnGet(name)
}

type MockQuotaService struct {
	OnInc      func(QuotaItem, int) error
	OnSet      func(QuotaItem, int) error
	OnSetLimit func(QuotaItem, int) error
	OnGet      func(QuotaItem) (*Quota, error)
}

func (m *MockQuotaService) Inc(item QuotaItem, delta int) error {
	if m.OnInc == nil {
		return nil
	}
	return m.OnInc(item, delta)
}

func (m *MockQuotaService) SetLimit(item QuotaItem, limit int) error {
	if m.OnSetLimit == nil {
		return nil
	}
	return m.OnSetLimit(item, limit)
}

func (m *MockQuotaService) Set(item QuotaItem, quantity int) error {
	if m.OnSet == nil {
		return nil
	}
	return m.OnSet(item, quantity)
}

func (m *MockQuotaService) Get(item QuotaItem) (*Quota, error) {
	return m.OnGet(item)
}
