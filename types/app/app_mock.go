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
	Apps       []*App
	OnList     func(filter *Filter) ([]*App, error)
	OnRegistry func(app *App) (image.ImageRegistry, error)
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
	return router.HealthcheckData{}, errors.New("not implemented")
}

func (m *MockAppService) GetAddresses(ctx context.Context, app *App) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (m *MockAppService) GetInternalBindableAddresses(ctx context.Context, app *App) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (m *MockAppService) EnsureUUID(ctx context.Context, app *App) (string, error) {
	return "", errors.New("not implemented")
}

func (m *MockAppService) GetRegistry(ctx context.Context, app *App) (image.ImageRegistry, error) {
	if m.OnRegistry == nil {
		return "", nil
	}
	return m.OnRegistry(app)
}

func (m *MockAppService) AddInstance(ctx context.Context, app *App, addArgs bind.AddInstanceArgs) error {
	return errors.New("not implemented")

}
func (m *MockAppService) RemoveInstance(ctx context.Context, app *App, removeArgs bind.RemoveInstanceArgs) error {
	return errors.New("not implemented")
}
