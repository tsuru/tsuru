// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build race
// +build race

package routertest

import (
	"context"
	"fmt"
	"net/url"
	"runtime"
	"sync"

	check "gopkg.in/check.v1"
)

func (s *S) TestAddRouteAndRemoteRouteAreSafe(c *check.C) {
	var wg sync.WaitGroup
	ctx := context.TODO()
	fake := newFakeRouter()
	originalMaxProcs := runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(originalMaxProcs)
	for i := 1; i < 256; i++ {
		wg.Add(5)
		name := fmt.Sprintf("route-%d", i)
		app := FakeApp{Name: name}
		addr, _ := url.Parse(fmt.Sprintf("http://10.10.10.%d", i))
		go func() {
			fake.AddBackend(ctx, app)
			wg.Done()
		}()
		go func() {
			fake.AddRoutes(ctx, app, []*url.URL{addr})
			wg.Done()
		}()
		go func() {
			fake.RemoveRoutes(ctx, app, []*url.URL{addr})
			wg.Done()
		}()
		go func() {
			fake.HasRoute(name, addr.String())
			wg.Done()
		}()
		go func() {
			fake.RemoveBackend(ctx, app)
			wg.Done()
		}()
	}
	wg.Wait()
}
