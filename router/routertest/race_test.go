// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build race

package routertest

import (
	"fmt"
	"net/url"
	"runtime"
	"sync"

	"gopkg.in/check.v1"
)

func (s *S) TestAddRouteAndRemoteRouteAreSafe(c *check.C) {
	var wg sync.WaitGroup
	fake := fakeRouter{backends: make(map[string][]string)}
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(4))
	for i := 1; i < 256; i++ {
		wg.Add(5)
		name := fmt.Sprintf("route-%d", i)
		addr, _ := url.Parse(fmt.Sprintf("http://10.10.10.%d", i))
		go func() {
			fake.AddBackend(name)
			wg.Done()
		}()
		go func() {
			fake.AddRoute(name, addr)
			wg.Done()
		}()
		go func() {
			fake.RemoveRoute(name, addr)
			wg.Done()
		}()
		go func() {
			fake.HasRoute(name, addr.String())
			wg.Done()
		}()
		go func() {
			fake.RemoveBackend(name)
			wg.Done()
		}()
	}
	wg.Wait()
}
