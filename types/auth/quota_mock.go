// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

var (
	_ AuthQuotaStorage = &MockAuthQuotaStorage{}
	_ AuthQuotaService = &MockAuthQuotaService{}
)

type MockAuthQuotaStorage struct {
	OnIncInUse func(string, *AuthQuota, int) error
	OnSetLimit func(string, int) error
	OnSetInUse func(string, int) error
}

func (m *MockAuthQuotaStorage) IncInUse(email string, quota *AuthQuota, quantity int) error {
	return m.OnIncInUse(email, quota, quantity)
}

func (m *MockAuthQuotaStorage) SetLimit(appName string, quota *AuthQuota, limit int) error {
	return m.OnSetLimit(appName, limit)
}

type MockAuthQuotaService struct {
	OnReserveApp  func(string, *AuthQuota) error
	OnReleaseApp  func(string, *AuthQuota) error
	OnChangeQuota func(string, int) error
}

func (m *MockAuthQuotaService) ReserveApp(email string, quota *AuthQuota) error {
	if m.OnReserveApp == nil {
		return nil
	}
	return m.OnReserveApp(email, quota)
}

func (m *MockAuthQuotaService) ReleaseApp(email string, quota *AuthQuota) error {
	if m.OnReleaseApp == nil {
		return nil
	}
	return m.OnReleaseApp(email, quota)
}

func (m *MockAuthQuotaService) ChangeQuota(email string, quantity int) error {
	if m.OnChangeQuota == nil {
		return nil
	}
	return m.OnChangeQuota(email, quantity)
}
