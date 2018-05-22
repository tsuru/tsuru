// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quota

var (
	_ QuotaStorage = &MockQuotaStorage{}
	_ QuotaService = &MockQuotaService{}
)

type MockQuotaStorage struct {
	OnInc      func(string, int) error
	OnSet      func(string, int) error
	OnSetLimit func(string, int) error
	OnGet      func(string) (*Quota, error)
}

func (m *MockQuotaStorage) Inc(name string, quantity int) error {
	return m.OnInc(name, quantity)
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
	OnInc      func(string, int) error
	OnSet      func(string, int) error
	OnSetLimit func(string, int) error
	OnGet      func(string) (*Quota, error)
}

func (m *MockQuotaService) Inc(name string, delta int) error {
	if m.OnInc == nil {
		return nil
	}
	return m.OnInc(name, delta)
}

func (m *MockQuotaService) SetLimit(name string, limit int) error {
	if m.OnSetLimit == nil {
		return nil
	}
	return m.OnSetLimit(name, limit)
}

func (m *MockQuotaService) Set(name string, quantity int) error {
	if m.OnSet == nil {
		return nil
	}
	return m.OnSet(name, quantity)
}

func (m *MockQuotaService) Get(name string) (*Quota, error) {
	if m.OnGet == nil {
		return nil, nil
	}
	return m.OnGet(name)
}
