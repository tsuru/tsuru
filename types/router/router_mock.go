// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import "context"

var (
	_ DynamicRouterService = &MockDynamicRouterService{}
)

type MockDynamicRouterService struct {
	OnCreate func(DynamicRouter) error
	OnUpdate func(DynamicRouter) error
	OnGet    func(name string) (*DynamicRouter, error)
	OnList   func() ([]DynamicRouter, error)
	OnRemove func(name string) error
}

func (m *MockDynamicRouterService) Create(ctx context.Context, dr DynamicRouter) error {
	if m.OnCreate == nil {
		return nil
	}
	return m.OnCreate(dr)
}

func (m *MockDynamicRouterService) Update(ctx context.Context, dr DynamicRouter) error {
	if m.OnUpdate == nil {
		return nil
	}
	return m.OnUpdate(dr)
}

func (m *MockDynamicRouterService) Get(ctx context.Context, name string) (*DynamicRouter, error) {
	if m.OnGet == nil {
		return nil, ErrDynamicRouterNotFound
	}
	return m.OnGet(name)
}

func (m *MockDynamicRouterService) List(ctx context.Context) ([]DynamicRouter, error) {
	if m.OnList == nil {
		return nil, nil
	}
	return m.OnList()
}

func (m *MockDynamicRouterService) Remove(ctx context.Context, name string) error {
	if m.OnRemove == nil {
		return ErrDynamicRouterNotFound
	}
	return m.OnRemove(name)
}
