// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"io"
	"net"
	"net/url"
	"sync"

	"github.com/tsuru/tsuru/provision/docker/container"
	"gopkg.in/mgo.v2/bson"
)

type DockerProvisioner interface {
	container.DockerProvisioner
	MoveOneContainer(container.Container, string, chan error, *sync.WaitGroup, io.Writer, container.AppLocker) container.Container
	MoveContainers(fromHost, toHost string, w io.Writer) error
	HandleMoveErrors(errors chan error, w io.Writer) error
	GetContainer(id string) (*container.Container, error)
	ListContainers(query bson.M) ([]container.Container, error)
}

type AppLocker interface {
	Lock(appName string) bool
	Unlock(appName string)
}

func urlToHost(urlStr string) string {
	url, _ := url.Parse(urlStr)
	if url == nil || url.Host == "" {
		return urlStr
	}
	host, _, _ := net.SplitHostPort(url.Host)
	if host == "" {
		return url.Host
	}
	return host
}
