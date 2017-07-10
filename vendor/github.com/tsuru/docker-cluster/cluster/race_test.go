// Copyright 2015 docker-cluster authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build race

package cluster

import (
	"runtime"
	"sync"
	"testing"

	"github.com/fsouza/go-dockerclient"
)

func TestRoundRobinScheduleIsRaceFree(t *testing.T) {
	const tasks = 8
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(tasks))
	c, err := New(&roundRobin{}, &MapStorage{}, "")
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	c.Register(Node{Address: "url1"})
	c.Register(Node{Address: "url2"})
	opts := docker.CreateContainerOptions{Config: &docker.Config{}}
	var wg sync.WaitGroup
	wg.Add(8)
	for i := 0; i < tasks; i++ {
		go func() {
			defer wg.Done()
			node, err := c.scheduler.Schedule(c, opts, nil)
			if err != nil {
				t.Fatal(err)
			}
			if node.Address != "url1" && node.Address != "url2" {
				t.Errorf("Wrong node. Wanted url1 or url2. Got %q.", node.Address)
			}
		}()
	}
	wg.Wait()
}
