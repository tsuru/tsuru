// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/hc"
	"gopkg.in/check.v1"
)

func (s *S) TestHealthCheckDockerRegistry(c *check.C) {
	var request *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request = r
		w.Write([]byte("pong"))
	}))
	defer server.Close()
	if old, err := config.Get("docker:registry"); err == nil {
		defer config.Set("docker:registry", old)
	} else {
		defer config.Unset("docker:registry")
	}
	config.Set("docker:registry", server.URL+"/")
	err := healthCheckDockerRegistry()
	c.Assert(err, check.IsNil)
	c.Assert(request.URL.Path, check.Equals, "/v1/_ping")
	c.Assert(request.Method, check.Equals, "GET")
}

func (s *S) TestHealthCheckDockerRegistryConfiguredWithoutScheme(c *check.C) {
	var request *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request = r
		w.Write([]byte("pong"))
	}))
	defer server.Close()
	if old, err := config.Get("docker:registry"); err == nil {
		defer config.Set("docker:registry", old)
	} else {
		defer config.Unset("docker:registry")
	}
	serverURL, _ := url.Parse(server.URL)
	config.Set("docker:registry", serverURL.Host)
	err := healthCheckDockerRegistry()
	c.Assert(err, check.IsNil)
	c.Assert(request.URL.Path, check.Equals, "/v1/_ping")
	c.Assert(request.Method, check.Equals, "GET")
}

func (s *S) TestHealthCheckDockerRegistryFailure(c *check.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("not pong"))
	}))
	defer server.Close()
	if old, err := config.Get("docker:registry"); err == nil {
		defer config.Set("docker:registry", old)
	} else {
		defer config.Unset("docker:registry")
	}
	config.Set("docker:registry", server.URL)
	err := healthCheckDockerRegistry()
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "unexpected status - not pong")
}

func (s *S) TestHealthCheckDockerRegistryUnconfigured(c *check.C) {
	if old, err := config.Get("docker:registry"); err == nil {
		defer config.Set("docker:registry", old)
	}
	config.Unset("docker:registry")
	err := healthCheckDockerRegistry()
	c.Assert(err, check.Equals, hc.ErrDisabledComponent)
}

func (s *S) TestHealthCheckDocker(c *check.C) {
	var request *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request = r
		w.Write([]byte("OK"))
	}))
	defer server.Close()
	oldCluster := dCluster
	defer func() {
		dCluster = oldCluster
	}()
	dCluster, _ = cluster.New(nil, &cluster.MapStorage{}, cluster.Node{Address: server.URL})
	err := healthCheckDocker()
	c.Assert(err, check.IsNil)
	c.Assert(request.Method, check.Equals, "GET")
	c.Assert(request.URL.Path, check.Equals, "/_ping")
}

func (s *S) TestHealthCheckDockerMultipleNodes(c *check.C) {
	var request *http.Request
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request = r
		w.Write([]byte("OK"))
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request = r
		w.Write([]byte("OK"))
	}))
	defer server2.Close()
	oldCluster := dCluster
	defer func() {
		dCluster = oldCluster
	}()
	dCluster, _ = cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: server1.URL}, cluster.Node{Address: server2.URL})
	err := healthCheckDocker()
	c.Assert(err, check.Equals, hc.ErrDisabledComponent)
	c.Assert(request, check.IsNil)
}

func (s *S) TestHealthCheckDockerNoNodes(c *check.C) {
	oldCluster := dCluster
	defer func() {
		dCluster = oldCluster
	}()
	dCluster, _ = cluster.New(nil, &cluster.MapStorage{})
	err := healthCheckDocker()
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "error - no nodes available for running containers")
}

func (s *S) TestHealthCheckDockerFailure(c *check.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("something went wrong"))
	}))
	defer server.Close()
	oldCluster := dCluster
	defer func() {
		dCluster = oldCluster
	}()
	dCluster, _ = cluster.New(nil, &cluster.MapStorage{}, cluster.Node{Address: server.URL})
	err := healthCheckDocker()
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "ping failed - API error (500): something went wrong")
}
