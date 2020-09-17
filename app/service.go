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

func AppService() (appTypes.AppService, error) {
	return &appService{}, nil
}
