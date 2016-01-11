// Copyright 2014 docker-cluster authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/fsouza/go-dockerclient"
)

// Arbitrary options to be sent to the scheduler. This options will
// be only read and interpreted by the scheduler itself.
type SchedulerOptions interface{}

// Scheduler represents a scheduling strategy, that will be used when creating
// new containers.
type Scheduler interface {
	// Schedule creates a new container, returning the ID of the node where
	// the container is running, and the container, or an error.
	Schedule(c *Cluster, opts docker.CreateContainerOptions, schedulerOpts SchedulerOptions) (Node, error)
}

type roundRobin struct {
	lastUsed int64
	once     sync.Once
}

func (s *roundRobin) Schedule(c *Cluster, opts docker.CreateContainerOptions, schedulerOpts SchedulerOptions) (Node, error) {
	nodes, _ := c.Nodes()
	if len(nodes) == 0 {
		return Node{}, errors.New("No nodes available")
	}
	s.once.Do(func() {
		s.lastUsed = -1
	})
	value := atomic.AddInt64(&s.lastUsed, 1)
	index := value % int64(len(nodes))
	return nodes[index], nil
}
