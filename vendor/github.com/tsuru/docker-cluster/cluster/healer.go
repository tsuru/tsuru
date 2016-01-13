// Copyright 2014 docker-cluster authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cluster provides types and functions for management of Docker
// clusters, scheduling container operations among hosts running Docker
// (nodes).

package cluster

import "time"

type Healer interface {
	HandleError(node *Node) time.Duration
}

type DefaultHealer struct{}

func (DefaultHealer) HandleError(node *Node) time.Duration {
	return 1 * time.Minute
}
