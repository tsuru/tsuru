// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"

	appTypes "github.com/tsuru/tsuru/types/app"
)

type appService struct{}

func (a *appService) GetByName(ctx context.Context, name string) (appTypes.App, error) {
	return GetByName(ctx, name)
}

func (a *appService) List(ctx context.Context, filter *appTypes.Filter) ([]appTypes.App, error) {
	var f *Filter
	if filter != nil {
		f = func(f Filter) *Filter { return &f }(Filter(*filter))
	}
	apps, err := List(ctx, f)
	if err != nil {
		return nil, err
	}
	as := make([]appTypes.App, 0, len(apps))
	for i := range apps {
		as = append(as, &apps[i])
	}
	return as, nil
}

func AppService() (appTypes.AppService, error) {
	return &appService{}, nil
}
