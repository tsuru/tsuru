// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

var (
	_ AppQuotaStorage = &MockAppQuotaStorage{}
	_ AppQuotaService = &MockAppQuotaService{}
)

type MockAppQuotaStorage struct {
	OnIncInUse      func(*AppQuota, int) error
	OnSetLimit      func(string, int) error
	OnSetInUse      func(string, int) error
	OnFindByAppName func(string) (*AppQuota, error)
}

func (m *MockAppQuotaStorage) IncInUse(quota *AppQuota, quantity int) error {
	return m.OnIncInUse(quota, quantity)
}

func (m *MockAppQuotaStorage) SetLimit(appName string, limit int) error {
	return m.OnSetLimit(appName, limit)
}

func (m *MockAppQuotaStorage) SetInUse(appName string, inUse int) error {
	return m.OnSetInUse(appName, inUse)
}

func (m *MockAppQuotaStorage) FindByAppName(appName string) (*AppQuota, error) {
	return m.OnFindByAppName(appName)
}

type MockAppQuotaService struct {
	OnCheckAppUsage  func(*AppQuota, int) error
	OnCheckAppLimit  func(*AppQuota, int) error
	OnReserveUnits   func(*AppQuota, int) error
	OnReleaseUnits   func(*AppQuota, int) error
	OnChangeLimit    func(*AppQuota, int) error
	OnChangeInUse    func(*AppQuota, int) error
	OnFindByAppName  func(string) (*AppQuota, error)
	OnCheckAppExists func(string) error
}

func (m *MockAppQuotaService) CheckAppUsage(quota *AppQuota, quantity int) error {
	if m.OnCheckAppUsage == nil {
		return nil
	}
	return m.OnCheckAppUsage(quota, quantity)
}

func (m *MockAppQuotaService) CheckAppLimit(quota *AppQuota, quantity int) error {
	if m.OnCheckAppLimit == nil {
		return nil
	}
	return m.OnCheckAppLimit(quota, quantity)
}

func (m *MockAppQuotaService) ReserveUnits(quota *AppQuota, quantity int) error {
	if m.OnReserveUnits == nil {
		return nil
	}
	return m.OnReserveUnits(quota, quantity)
}

func (m *MockAppQuotaService) ReleaseUnits(quota *AppQuota, quantity int) error {
	if m.OnReleaseUnits == nil {
		return nil
	}
	return m.OnReleaseUnits(quota, quantity)
}

func (m *MockAppQuotaService) ChangeLimit(quota *AppQuota, quantity int) error {
	if m.OnChangeLimit == nil {
		return nil
	}
	return m.OnChangeLimit(quota, quantity)
}

func (m *MockAppQuotaService) ChangeInUse(quota *AppQuota, quantity int) error {
	if m.OnChangeInUse == nil {
		return nil
	}
	return m.OnChangeInUse(quota, quantity)
}

func (m *MockAppQuotaService) FindByAppName(appName string) (*AppQuota, error) {
	if m.OnFindByAppName == nil {
		return nil, nil
	}
	return m.OnFindByAppName(appName)
}
