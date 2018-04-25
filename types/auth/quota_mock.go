// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

var (
	_ QuotaStorage = &MockQuotaStorage{}
	_ QuotaService = &MockQuotaService{}
)

type MockQuotaStorage struct {
	OnIncInUse        func(string, int) error
	OnSetLimit        func(string, int) error
	OnFindByUserEmail func(string) (*Quota, error)
}

func (m *MockQuotaStorage) IncInUse(email string, quantity int) error {
	return m.OnIncInUse(email, quantity)
}

func (m *MockQuotaStorage) SetLimit(email string, limit int) error {
	return m.OnSetLimit(email, limit)
}

func (m *MockQuotaStorage) FindByUserEmail(email string) (*Quota, error) {
	return m.OnFindByUserEmail(email)
}

type MockQuotaService struct {
	OnReserveApp      func(string) error
	OnReleaseApp      func(string) error
	OnChangeLimit     func(string, int) error
	OnFindByUserEmail func(string) (*Quota, error)
}

func (m *MockQuotaService) ReserveApp(email string) error {
	if m.OnReserveApp == nil {
		return nil
	}
	return m.OnReserveApp(email)
}

func (m *MockQuotaService) ReleaseApp(email string) error {
	if m.OnReleaseApp == nil {
		return nil
	}
	return m.OnReleaseApp(email)
}

func (m *MockQuotaService) ChangeLimit(email string, quantity int) error {
	if m.OnChangeLimit == nil {
		return nil
	}
	return m.OnChangeLimit(email, quantity)
}

func (m *MockQuotaService) FindByUserEmail(email string) (*Quota, error) {
	if m.OnFindByUserEmail == nil {
		return nil, nil
	}
	return m.OnFindByUserEmail(email)
}
