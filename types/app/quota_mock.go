// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

var (
	_ QuotaStorage = &MockQuotaStorage{}
	_ QuotaService = &MockQuotaService{}
)

type MockQuotaStorage struct {
	OnIncInUse      func(string, int) error
	OnSetLimit      func(string, int) error
	OnSetInUse      func(string, int) error
	OnFindByAppName func(string) (*Quota, error)
}

func (m *MockQuotaStorage) IncInUse(appName string, quantity int) error {
	return m.OnIncInUse(appName, quantity)
}

func (m *MockQuotaStorage) SetLimit(appName string, limit int) error {
	return m.OnSetLimit(appName, limit)
}

func (m *MockQuotaStorage) SetInUse(appName string, inUse int) error {
	return m.OnSetInUse(appName, inUse)
}

func (m *MockQuotaStorage) FindByAppName(appName string) (*Quota, error) {
	return m.OnFindByAppName(appName)
}

type MockQuotaService struct {
	OnCheckAppUsage  func(*Quota, int) error
	OnCheckAppLimit  func(*Quota, int) error
	OnReserveUnits   func(string, int) error
	OnReleaseUnits   func(string, int) error
	OnChangeLimit    func(string, int) error
	OnChangeInUse    func(string, int) error
	OnFindByAppName  func(string) (*Quota, error)
	OnCheckAppExists func(string) error
}

func (m *MockQuotaService) CheckAppUsage(quota *Quota, quantity int) error {
	if m.OnCheckAppUsage == nil {
		return nil
	}
	return m.OnCheckAppUsage(quota, quantity)
}

func (m *MockQuotaService) CheckAppLimit(quota *Quota, quantity int) error {
	if m.OnCheckAppLimit == nil {
		return nil
	}
	return m.OnCheckAppLimit(quota, quantity)
}

func (m *MockQuotaService) ReserveUnits(appName string, quantity int) error {
	if m.OnReserveUnits == nil {
		return nil
	}
	return m.OnReserveUnits(appName, quantity)
}

func (m *MockQuotaService) ReleaseUnits(appName string, quantity int) error {
	if m.OnReleaseUnits == nil {
		return nil
	}
	return m.OnReleaseUnits(appName, quantity)
}

func (m *MockQuotaService) ChangeLimit(appName string, quantity int) error {
	if m.OnChangeLimit == nil {
		return nil
	}
	return m.OnChangeLimit(appName, quantity)
}

func (m *MockQuotaService) ChangeInUse(appName string, quantity int) error {
	if m.OnChangeInUse == nil {
		return nil
	}
	return m.OnChangeInUse(appName, quantity)
}

func (m *MockQuotaService) FindByAppName(appName string) (*Quota, error) {
	if m.OnFindByAppName == nil {
		return nil, nil
	}
	return m.OnFindByAppName(appName)
}
