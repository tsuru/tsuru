// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

var (
	_ AppQuotaStorage = &MockAppQuotaStorage{}
	_ AppQuotaService = &MockAppQuotaService{}
)

type MockAppQuotaStorage struct {
	OnIncInUse func(AppQuotaService, *AppQuota, int) error
	OnSetLimit func(string, int) error
	OnSetInUse func(string, int) error
}

func (m *MockAppQuotaStorage) IncInUse(service AppQuotaService, quota *AppQuota, quantity int) error {
	return m.OnIncInUse(service, quota, quantity)
}

func (m *MockAppQuotaStorage) SetLimit(appName string, limit int) error {
	return m.OnSetLimit(appName, limit)
}

func (m *MockAppQuotaStorage) SetInUse(appName string, inUse int) error {
	return m.OnSetInUse(appName, inUse)
}

type MockAppQuotaService struct {
	OnCheckAppUsage    func(*AppQuota, int) error
	OnCheckAppLimit    func(*AppQuota, int) error
	OnReserveUnits     func(*AppQuota, int) error
	OnReleaseUnits     func(*AppQuota, int) error
	OnChangeLimitQuota func(*AppQuota, int) error
	OnChangeInUseQuota func(*AppQuota, int) error
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

func (m *MockAppQuotaService) ChangeLimitQuota(quota *AppQuota, quantity int) error {
	if m.OnChangeLimitQuota == nil {
		return nil
	}
	return m.OnChangeLimitQuota(quota, quantity)
}

func (m *MockAppQuotaService) ChangeInUseQuota(quota *AppQuota, quantity int) error {
	if m.OnChangeInUseQuota == nil {
		return nil
	}
	return m.OnChangeInUseQuota(quota, quantity)
}
