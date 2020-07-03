// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

var (
	_ RouterTemplateService = &MockRouterTemplateService{}
)

type MockRouterTemplateService struct {
	OnCreate func(RouterTemplate) error
	OnUpdate func(RouterTemplate) error
	OnGet    func(name string) (*RouterTemplate, error)
	OnList   func() ([]RouterTemplate, error)
	OnRemove func(name string) error
}

func (m *MockRouterTemplateService) Create(rt RouterTemplate) error {
	if m.OnCreate == nil {
		return nil
	}
	return m.OnCreate(rt)
}

func (m *MockRouterTemplateService) Update(rt RouterTemplate) error {
	if m.OnUpdate == nil {
		return nil
	}
	return m.OnUpdate(rt)
}

func (m *MockRouterTemplateService) Get(name string) (*RouterTemplate, error) {
	if m.OnGet == nil {
		return nil, ErrRouterTemplateNotFound
	}
	return m.OnGet(name)
}

func (m *MockRouterTemplateService) List() ([]RouterTemplate, error) {
	if m.OnList == nil {
		return nil, nil
	}
	return m.OnList()
}

func (m *MockRouterTemplateService) Remove(name string) error {
	if m.OnRemove == nil {
		return ErrRouterTemplateNotFound
	}
	return m.OnRemove(name)
}
