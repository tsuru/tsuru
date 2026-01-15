// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"errors"

	"github.com/tsuru/tsuru/types/app/image"
	"github.com/tsuru/tsuru/types/bind"
	"github.com/tsuru/tsuru/types/router"
)

var _ AppService = &MockAppService{}

type MockAppService struct {
	Apps                           []*App
	OnList                         func(filter *Filter) ([]*App, error)
	OnRegistry                     func(app *App) (image.ImageRegistry, error)
	OnGetAddresses                 func(app *App) ([]string, error)
	OnAddInstance                  func(app *App, addArgs bind.AddInstanceArgs) error
	OnRemoveInstance               func(app *App, removeArgs bind.RemoveInstanceArgs) error
	OnGetInternalBindableAddresses func(app *App) ([]string, error)
	OnGetHealthcheckData           func(app *App) (router.HealthcheckData, error)
}

func (m *MockAppService) GetByName(ctx context.Context, name string) (*App, error) {
	for _, app := range m.Apps {
		if app.Name == name {
			return app, nil
		}
	}
	return nil, ErrAppNotFound
}

func (m *MockAppService) List(ctx context.Context, f *Filter) ([]*App, error) {
	if m.OnList == nil {
		return nil, nil
	}

	return m.OnList(f)
}

func (m *MockAppService) GetHealthcheckData(ctx context.Context, app *App) (router.HealthcheckData, error) {
	if m.OnGetHealthcheckData != nil {
		return m.OnGetHealthcheckData(app)
	}
	return router.HealthcheckData{}, errors.New("MockAppService.GetHealthcheckData is not implemented")
}

func (m *MockAppService) GetAddresses(ctx context.Context, app *App) ([]string, error) {
	if m.OnGetAddresses != nil {
		return m.OnGetAddresses(app)
	}

	return nil, errors.New("MockAppService.GetAddresses is not implemented")
}

func (m *MockAppService) GetInternalBindableAddresses(ctx context.Context, app *App) ([]string, error) {
	if m.OnGetInternalBindableAddresses != nil {
		return m.OnGetInternalBindableAddresses(app)
	}
	return nil, errors.New("MockAppService.GetInternalBindableAddresses is not implemented")
}

func (m *MockAppService) GetRegistry(ctx context.Context, app *App) (image.ImageRegistry, error) {
	if m.OnRegistry == nil {
		return "", nil
	}
	return m.OnRegistry(app)
}

func (m *MockAppService) AddInstance(ctx context.Context, app *App, addArgs bind.AddInstanceArgs) error {
	if m.OnAddInstance != nil {
		return m.OnAddInstance(app, addArgs)
	}

	return errors.New("MockAppService.AddInstance is not implemented")
}

func (m *MockAppService) RemoveInstance(ctx context.Context, app *App, removeArgs bind.RemoveInstanceArgs) error {
	if m.OnRemoveInstance != nil {
		return m.OnRemoveInstance(app, removeArgs)
	}

	return errors.New("MockAppService.RemoveInstance is not implemented")
}
