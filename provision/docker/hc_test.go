// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/hc"
	tsuruNet "github.com/tsuru/tsuru/net"
	"gopkg.in/check.v1"
)

func (s *S) TestHealthCheckDockerRegistryV2(c *check.C) {
	var request *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request = r
		w.Write([]byte("{}"))
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
	c.Assert(request.URL.Path, check.Equals, "/v2/")
	c.Assert(request.Method, check.Equals, http.MethodGet)
}

func (s *S) TestHealthCheckDockerRegistryV1(c *check.C) {
	var request *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
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
	c.Assert(request.Method, check.Equals, http.MethodGet)
}

func (s *S) TestHealthCheckDockerRegistryConfiguredWithoutScheme(c *check.C) {
	var request *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request = r
		w.Write([]byte("{}"))
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
	c.Assert(request.URL.Path, check.Equals, "/v2/")
	c.Assert(request.Method, check.Equals, http.MethodGet)
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

func (s *S) TestHealthCheckDockerRegistryV2WithAuth(c *check.C) {
	var request *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request = r
		w.Write([]byte("{}"))
	}))
	defer server.Close()
	oldRegistry, err := config.Get("docker:registry")
	config.Set("docker:registry", server.URL+"/")
	if err == nil {
		defer config.Set("docker:registry", oldRegistry)
	} else {
		defer config.Unset("docker:registry")
	}
	oldRegistryUsername, err := config.Get("docker:registry-auth:username")
	config.Set("docker:registry-auth:username", "tsuru")
	if err == nil {
		defer config.Set("docker:registry-auth:username", oldRegistryUsername)
	} else {
		defer config.Unset("docker:registry-auth:username")
	}
	oldRegistryPassword, err := config.Get("docker:registry-auth:password")
	config.Set("docker:registry-auth:password", "pwd")
	if err == nil {
		defer config.Set("docker:registry-auth:password", oldRegistryPassword)
	} else {
		defer config.Unset("docker:registry-auth:password")
	}

	err = healthCheckDockerRegistry()
	c.Assert(err, check.IsNil)
	c.Assert(request.URL.Path, check.Equals, "/v2/")
	c.Assert(request.Method, check.Equals, http.MethodGet)
	c.Assert(request.Header.Get("Authorization"), check.Matches, "Basic .*")
}

func (s *S) TestHealthCheckDockerRegistryV2WithAuthError(c *check.C) {
	var request *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Scheme == "https" {
			c.Assert(r.Header.Get("Authorization"), check.Matches, "Basic .*")
			w.WriteHeader(http.StatusUnauthorized)
		} else {
			request = r
			w.Write([]byte("{}"))
		}
	}))
	defer server.Close()
	oldRegistry, err := config.Get("docker:registry")
	config.Set("docker:registry", server.URL+"/")
	if err == nil {
		defer config.Set("docker:registry", oldRegistry)
	} else {
		defer config.Unset("docker:registry")
	}
	oldRegistryUsername, err := config.Get("docker:registry-auth:username")
	config.Set("docker:registry-auth:username", "tsuru")
	if err == nil {
		defer config.Set("docker:registry-auth:username", oldRegistryUsername)
	} else {
		defer config.Unset("docker:registry-auth:username")
	}
	oldRegistryPassword, err := config.Get("docker:registry-auth:password")
	config.Set("docker:registry-auth:password", "wrongpwd")
	if err == nil {
		defer config.Set("docker:registry-auth:password", oldRegistryPassword)
	} else {
		defer config.Unset("docker:registry-auth:password")
	}

	err = healthCheckDockerRegistry()
	c.Assert(err, check.IsNil)
	c.Assert(request.URL.Path, check.Equals, "/v2/")
	c.Assert(request.Method, check.Equals, http.MethodGet)
}

func (s *S) TestHealthCheckDocker(c *check.C) {
	var request *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request = r
		w.Write([]byte("OK"))
	}))
	defer server.Close()
	var err error
	mainDockerProvisioner.cluster, err = cluster.New(nil, &cluster.MapStorage{}, "", cluster.Node{Address: server.URL})
	c.Assert(err, check.IsNil)
	err = healthCheckDocker()
	c.Assert(err, check.IsNil)
	c.Assert(request.Method, check.Equals, http.MethodGet)
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
	var err error
	mainDockerProvisioner.cluster, err = cluster.New(nil, &cluster.MapStorage{}, "",
		cluster.Node{Address: server1.URL}, cluster.Node{Address: server2.URL})
	c.Assert(err, check.IsNil)
	err = healthCheckDocker()
	c.Assert(err, check.Equals, hc.ErrDisabledComponent)
	c.Assert(request, check.IsNil)
}

func (s *S) TestHealthCheckDockerNoNodes(c *check.C) {
	var err error
	mainDockerProvisioner.cluster, err = cluster.New(nil, &cluster.MapStorage{}, "")
	c.Assert(err, check.IsNil)
	err = healthCheckDocker()
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "error - no nodes available for running containers")
}

func (s *S) TestHealthCheckDockerFailure(c *check.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("something went wrong"))
	}))
	defer server.Close()
	var err error
	mainDockerProvisioner.cluster, err = cluster.New(nil, &cluster.MapStorage{}, "", cluster.Node{Address: server.URL})
	c.Assert(err, check.IsNil)
	err = healthCheckDocker()
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "ping failed: API error (500): something went wrong")
}

func (s *S) TestHealthCheckDockerRegistryV2TLS(c *check.C) {
	var request *http.Request
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request = r
		w.Write([]byte("{}"))
	}))
	defer server.Close()
	oldTransport := tsuruNet.Dial5Full60ClientNoKeepAlive.Transport
	defer func() { tsuruNet.Dial5Full60ClientNoKeepAlive.Transport = oldTransport }()
	tsuruNet.Dial5Full60ClientNoKeepAlive.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	if old, err := config.Get("docker:registry"); err == nil {
		defer config.Set("docker:registry", old)
	} else {
		defer config.Unset("docker:registry")
	}
	config.Set("docker:registry", server.URL+"/")
	err := healthCheckDockerRegistry()
	c.Assert(err, check.IsNil)
	c.Assert(request.URL.Path, check.Equals, "/v2/")
	c.Assert(request.Method, check.Equals, http.MethodGet)
}
