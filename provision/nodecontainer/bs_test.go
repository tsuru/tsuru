// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodecontainer

import (
	"runtime"
	"sync"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	check "gopkg.in/check.v1"
)

func (s *S) TestInitializeBS(c *check.C) {
	config.Set("host", "127.0.0.1:8080")
	config.Set("docker:bs:image", "tsuru/bs:v10")
	defer config.Unset("host")
	defer config.Unset("docker:bs:image")
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	initialized, err := InitializeBS(nativeScheme, "tsr")
	c.Assert(err, check.IsNil)
	c.Assert(initialized, check.Equals, true)
	nodeContainer, err := LoadNodeContainer("", BsDefaultName)
	c.Assert(err, check.IsNil)
	c.Assert(nodeContainer.Config.Env[0], check.Matches, `^TSURU_TOKEN=.{40}$`)
	nodeContainer.Config.Env = nodeContainer.Config.Env[1:]
	c.Assert(nodeContainer, check.DeepEquals, &NodeContainerConfig{
		Name: BsDefaultName,
		Config: docker.Config{
			Image: "tsuru/bs:v10",
			Env: []string{
				"TSURU_ENDPOINT=http://127.0.0.1:8080/",
				"HOST_PROC=/prochost",
				"SYSLOG_LISTEN_ADDRESS=udp://0.0.0.0:1514",
			},
		},
		HostConfig: docker.HostConfig{
			RestartPolicy: docker.AlwaysRestart(),
			Privileged:    true,
			NetworkMode:   "host",
			Binds:         []string{"/proc:/prochost:ro"},
		},
	})
	initialized, err = InitializeBS(nativeScheme, "tsr")
	c.Assert(err, check.IsNil)
	c.Assert(initialized, check.Equals, false)
}

func (s *S) TestInitializeBSStress(c *check.C) {
	originalMaxProcs := runtime.GOMAXPROCS(10)
	defer runtime.GOMAXPROCS(originalMaxProcs)
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	nTimes := 100
	initializedCh := make(chan bool, nTimes)
	wg := sync.WaitGroup{}
	for i := 0; i < nTimes; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			initialized, err := InitializeBS(nativeScheme, "tsr")
			c.Assert(err, check.IsNil)
			initializedCh <- initialized
		}()
	}
	wg.Wait()
	close(initializedCh)
	var initOk bool
	for ok := range initializedCh {
		if ok {
			c.Assert(initOk, check.Equals, false)
			initOk = ok
		}
	}
	c.Assert(initOk, check.Equals, true)
}
