// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/hc"
	"launchpad.net/gocheck"
)

func (s *S) TestHealthCheckDockerRegistry(c *gocheck.C) {
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
	c.Assert(err, gocheck.IsNil)
	c.Assert(request.URL.Path, gocheck.Equals, "/v1/_ping")
	c.Assert(request.Method, gocheck.Equals, "GET")
}

func (s *S) TestHealthCheckDockerRegistryConfiguredWithoutScheme(c *gocheck.C) {
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
	c.Assert(err, gocheck.IsNil)
	c.Assert(request.URL.Path, gocheck.Equals, "/v1/_ping")
	c.Assert(request.Method, gocheck.Equals, "GET")
}

func (s *S) TestHealthCheckDockerRegistryFailure(c *gocheck.C) {
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
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "unexpected status - not pong")
}

func (s *S) TestHealthCheckDockerRegistryUnconfigured(c *gocheck.C) {
	if old, err := config.Get("docker:registry"); err == nil {
		defer config.Set("docker:registry", old)
	}
	config.Unset("docker:registry")
	err := healthCheckDockerRegistry()
	c.Assert(err, gocheck.Equals, hc.ErrDisabledComponent)
}
