// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"context"
	"io"
	"sync"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/container"
)

type DockerProvisioner interface {
	ClusterClient() provision.BuilderDockerClient
	Cluster() *cluster.Cluster
	ActionLimiter() provision.ActionLimiter
	MoveOneContainer(context.Context, container.Container, string, chan error, *sync.WaitGroup, io.Writer, container.AppLocker) container.Container
	MoveContainers(ctx context.Context, fromHost, toHost string, w io.Writer) error
	HandleMoveErrors(errors chan error, w io.Writer) error
	GetContainer(id string) (*container.Container, error)
	ListContainers(query bson.M) ([]container.Container, error)
}

type AppLocker interface {
	Lock(appName string) bool
	Unlock(appName string)
}
