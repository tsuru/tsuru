// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build race

package testing

import (
	"fmt"
	"runtime"
	"sync"

	"launchpad.net/gocheck"
)

func (s *S) TestAddRouteAndRemoteRouteAreSafe(c *gocheck.C) {
	var wg sync.WaitGroup
	fake := fakeRouter{backends: make(map[string][]string)}
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(4))
	for i := 1; i < 256; i++ {
		wg.Add(5)
		name := fmt.Sprintf("route-%d", i)
		ip := fmt.Sprintf("10.10.10.%d", i)
		go func(i int) {
			fake.AddBackend(name)
			wg.Done()
		}(i)
		go func(i int) {
			fake.AddRoute(name, ip)
			wg.Done()
		}(i)
		go func() {
			fake.RemoveRoute(name, ip)
			wg.Done()
		}()
		go func() {
			fake.HasRoute(name, ip)
			wg.Done()
		}()
		go func(i int) {
			fake.RemoveBackend(name)
			wg.Done()
		}(i)
	}
	wg.Wait()
}
