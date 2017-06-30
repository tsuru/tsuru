// Copyright 2014 docker-cluster authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster

import (
	"testing"

	"github.com/fsouza/go-dockerclient"
)

func TestRoundRobinSchedule(t *testing.T) {
	c, err := New(&roundRobin{}, &MapStorage{}, "")
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	c.Register(Node{Address: "url1"})
	c.Register(Node{Address: "url2"})
	opts := docker.CreateContainerOptions{Config: &docker.Config{}}
	node, err := c.scheduler.Schedule(c, opts, nil)
	if err != nil {
		t.Error(err)
	}
	if node.Address != "url1" {
		t.Errorf("roundRobin.Schedule(): wrong node ID. Want %q. Got %q.", "url1", node.Address)
	}
	node, _ = c.scheduler.Schedule(c, opts, nil)
	if node.Address != "url2" {
		t.Errorf("roundRobin.Schedule(): wrong node ID. Want %q. Got %q.", "url2", node.Address)
	}
	node, _ = c.scheduler.Schedule(c, opts, nil)
	if node.Address != "url1" {
		t.Errorf("roundRobin.Schedule(): wrong node ID. Want %q. Got %q.", "url1", node.Address)
	}
}

func TestScheduleEmpty(t *testing.T) {
	c, err := New(&roundRobin{}, &MapStorage{}, "")
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	expected := "No nodes available"
	opts := docker.CreateContainerOptions{Config: &docker.Config{}}
	_, err = c.scheduler.Schedule(c, opts, nil)
	if err == nil || err.Error() != expected {
		t.Fatalf("Schedule(): wrong error message. Want %q. Got %q.", expected, err)
	}
}
