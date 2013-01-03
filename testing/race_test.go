// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build race

package testing

import (
	"github.com/globocom/tsuru/provision"
	. "launchpad.net/gocheck"
	"sync"
)

func (s *S) TestFakeLBManagerIsThreadSafe(c *C) {
	var wg sync.WaitGroup
	apps := []string{"liquid", "dots", "waves", "race", "web"}
	lb := &FakeLBManager{
		provisioner: NewFakeProvisioner(),
		instances:   make(map[string][]string),
	}
	for _, app := range apps {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			app := NewFakeApp(name, "python", 1)
			lb.Create(app)
			lb.Register(app, provision.Unit{InstanceId: name})
		}(app)
	}
	// spread the chaos
	for _, app := range apps {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			app := NewFakeApp(name, "python", 1)
			lb.Deregister(app, provision.Unit{InstanceId: name})
			lb.Destroy(app)
		}(app)
	}
	wg.Wait()
	c.Check(lb.instances, HasLen, 0)
}
