// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

var (
	_ AuthQuotaStorage = &MockAuthQuotaStorage{}
	_ AuthQuotaService = &MockAuthQuotaService{}
)

type MockAuthQuotaStorage struct {
	OnIncInUse        func(string, *AuthQuota, int) error
	OnSetLimit        func(string, int) error
	OnFindByUserEmail func(string) (*AuthQuota, error)
}

func (m *MockAuthQuotaStorage) IncInUse(email string, quota *AuthQuota, quantity int) error {
	return m.OnIncInUse(email, quota, quantity)
}

func (m *MockAuthQuotaStorage) SetLimit(email string, quota *AuthQuota, limit int) error {
	return m.OnSetLimit(email, limit)
}

func (m *MockAuthQuotaStorage) FindByUserEmail(email string) (*AuthQuota, error) {
	return m.OnFindByUserEmail(email)
}

type MockAuthQuotaService struct {
	OnReserveApp      func(string, *AuthQuota) error
	OnReleaseApp      func(string, *AuthQuota) error
	OnChangeLimit     func(string, *AuthQuota, int) error
	OnFindByUserEmail func(string) (*AuthQuota, error)
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

func (m *MockAuthQuotaService) ChangeLimit(email string, quota *AuthQuota, quantity int) error {
	if m.OnChangeLimit == nil {
		return nil
	}
	return m.OnChangeLimit(email, quota, quantity)
}

func (m *MockAuthQuotaService) FindByUserEmail(email string) (*AuthQuota, error) {
	if m.OnFindByUserEmail == nil {
		return nil, nil
	}
	return m.OnFindByUserEmail(email)
}
