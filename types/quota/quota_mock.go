// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quota

var (
	_ UserQuotaStorage = &MockUserQuotaStorage{}
	_ UserQuotaService = &MockUserQuotaService{}
	_ AppQuotaStorage  = &MockAppQuotaStorage{}
	_ AppQuotaService  = &MockAppQuotaService{}
)

type MockUserQuotaStorage struct {
	OnIncInUse        func(string, int) error
	OnSetLimit        func(string, int) error
	OnFindByUserEmail func(string) (*Quota, error)
}

func (m *MockUserQuotaStorage) IncInUse(email string, quantity int) error {
	return m.OnIncInUse(email, quantity)
}

func (m *MockUserQuotaStorage) SetLimit(email string, limit int) error {
	return m.OnSetLimit(email, limit)
}

func (m *MockUserQuotaStorage) FindByUserEmail(email string) (*Quota, error) {
	return m.OnFindByUserEmail(email)
}

type MockAppQuotaStorage struct {
	OnIncInUse      func(string, int) error
	OnSetLimit      func(string, int) error
	OnSetInUse      func(string, int) error
	OnFindByAppName func(string) (*Quota, error)
}

func (m *MockAppQuotaStorage) IncInUse(appName string, quantity int) error {
	return m.OnIncInUse(appName, quantity)
}

func (m *MockAppQuotaStorage) SetLimit(appName string, limit int) error {
	return m.OnSetLimit(appName, limit)
}

func (m *MockAppQuotaStorage) SetInUse(appName string, inUse int) error {
	return m.OnSetInUse(appName, inUse)
}

func (m *MockAppQuotaStorage) FindByAppName(appName string) (*Quota, error) {
	return m.OnFindByAppName(appName)
}

type MockUserQuotaService struct {
	OnReserveApp      func(string) error
	OnReleaseApp      func(string) error
	OnChangeLimit     func(string, int) error
	OnFindByUserEmail func(string) (*Quota, error)
}

func (m *MockUserQuotaService) ReserveApp(email string) error {
	if m.OnReserveApp == nil {
		return nil
	}
	return m.OnReserveApp(email)
}

func (m *MockUserQuotaService) ReleaseApp(email string) error {
	if m.OnReleaseApp == nil {
		return nil
	}
	return m.OnReleaseApp(email)
}

func (m *MockUserQuotaService) ChangeLimit(email string, quantity int) error {
	if m.OnChangeLimit == nil {
		return nil
	}
	return m.OnChangeLimit(email, quantity)
}

func (m *MockUserQuotaService) FindByUserEmail(email string) (*Quota, error) {
	if m.OnFindByUserEmail == nil {
		return nil, nil
	}
	return m.OnFindByUserEmail(email)
}

type MockAppQuotaService struct {
	OnCheckAppUsage  func(*Quota, int) error
	OnCheckAppLimit  func(*Quota, int) error
	OnReserveUnits   func(string, int) error
	OnReleaseUnits   func(string, int) error
	OnChangeLimit    func(string, int) error
	OnChangeInUse    func(string, int) error
	OnFindByAppName  func(string) (*Quota, error)
	OnCheckAppExists func(string) error
}

func (m *MockAppQuotaService) CheckAppUsage(quota *Quota, quantity int) error {
	if m.OnCheckAppUsage == nil {
		return nil
	}
	return m.OnCheckAppUsage(quota, quantity)
}

func (m *MockAppQuotaService) CheckAppLimit(quota *Quota, quantity int) error {
	if m.OnCheckAppLimit == nil {
		return nil
	}
	return m.OnCheckAppLimit(quota, quantity)
}

func (m *MockAppQuotaService) ReserveUnits(appName string, quantity int) error {
	if m.OnReserveUnits == nil {
		return nil
	}
	return m.OnReserveUnits(appName, quantity)
}

func (m *MockAppQuotaService) ReleaseUnits(appName string, quantity int) error {
	if m.OnReleaseUnits == nil {
		return nil
	}
	return m.OnReleaseUnits(appName, quantity)
}

func (m *MockAppQuotaService) ChangeLimit(appName string, quantity int) error {
	if m.OnChangeLimit == nil {
		return nil
	}
	return m.OnChangeLimit(appName, quantity)
}

func (m *MockAppQuotaService) ChangeInUse(appName string, quantity int) error {
	if m.OnChangeInUse == nil {
		return nil
	}
	return m.OnChangeInUse(appName, quantity)
}

func (m *MockAppQuotaService) FindByAppName(appName string) (*Quota, error) {
	if m.OnFindByAppName == nil {
		return nil, nil
	}
	return m.OnFindByAppName(appName)
}
