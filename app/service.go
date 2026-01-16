// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"

	appTypes "github.com/tsuru/tsuru/types/app"
	imgTypes "github.com/tsuru/tsuru/types/app/image"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	routerTypes "github.com/tsuru/tsuru/types/router"
)

type appService struct{}

func (a *appService) GetByName(ctx context.Context, name string) (*appTypes.App, error) {
	return GetByName(ctx, name)
}

func (a *appService) GetAddresses(ctx context.Context, app *appTypes.App) ([]string, error) {
	return GetAddresses(ctx, app)
}

func (a *appService) GetRegistry(ctx context.Context, app *appTypes.App) (imgTypes.ImageRegistry, error) {
	return GetRegistry(ctx, app)
}

func (a *appService) AddInstance(ctx context.Context, app *appTypes.App, addArgs bindTypes.AddInstanceArgs) error {
	return AddInstance(ctx, app, addArgs)
}

func (a *appService) RemoveInstance(ctx context.Context, app *appTypes.App, removeArgs bindTypes.RemoveInstanceArgs) error {
	return RemoveInstance(ctx, app, removeArgs)
}

func (a *appService) GetInternalBindableAddresses(ctx context.Context, app *appTypes.App) ([]string, error) {
	return GetInternalBindableAddresses(ctx, app)
}

func (a *appService) List(ctx context.Context, filter *appTypes.Filter) ([]*appTypes.App, error) {
	var f *Filter
	if filter != nil {
		f = func(f Filter) *Filter { return &f }(Filter(*filter))
	}
	return List(ctx, f)
}

func (a *appService) GetHealthcheckData(ctx context.Context, app *appTypes.App) (routerTypes.HealthcheckData, error) {
	return GetHealthcheckData(ctx, app)
}

func AppService() (appTypes.AppService, error) {
	return &appService{}, nil
}
