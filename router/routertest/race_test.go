// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build race
// +build race

package routertest

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/tsuru/tsuru/router"
	check "gopkg.in/check.v1"
)

func (s *S) TestAddRouteAndRemoteRouteAreSafe(c *check.C) {
	var wg sync.WaitGroup
	ctx := context.TODO()
	fake := newFakeRouter()
	originalMaxProcs := runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(originalMaxProcs)
	for i := 1; i < 256; i++ {
		wg.Add(2)
		name := fmt.Sprintf("route-%d", i)
		app := FakeApp{Name: name}
		go func() {
			fake.EnsureBackend(ctx, app, router.EnsureBackendOpts{})
			wg.Done()
		}()
		go func() {
			fake.RemoveBackend(ctx, app)
			wg.Done()
		}()
	}
	wg.Wait()
}
