// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"sync"

	"github.com/tsuru/tsuru/api/shutdown"
	appTypes "github.com/tsuru/tsuru/types/app"
)

const appBackgroudRouterUpdaterLimit = 20

type appRouterUpdater struct {
	limiter chan struct{}
	wg      sync.WaitGroup
}

var globalAppRouterUpdater struct {
	updater *appRouterUpdater
	once    sync.Once
}

func GetAppRouterUpdater() *appRouterUpdater {
	globalAppRouterUpdater.once.Do(func() {
		globalAppRouterUpdater.updater = &appRouterUpdater{
			limiter: make(chan struct{}, appBackgroudRouterUpdaterLimit),
		}
		shutdown.Register(globalAppRouterUpdater.updater)
	})
	return globalAppRouterUpdater.updater
}

func (u *appRouterUpdater) update(ctx context.Context, a *appTypes.App) {
	u.wg.Add(1)
	go func() {
		u.limiter <- struct{}{}
		defer func() { <-u.limiter }()
		defer u.wg.Done()
		GetRoutersWithAddr(ctx, a)
	}()
}

func (u *appRouterUpdater) Shutdown(ctx context.Context) error {
	waitCh := make(chan struct{})
	go func() {
		u.wg.Wait()
		close(waitCh)
	}()
	select {
	case <-waitCh:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}
