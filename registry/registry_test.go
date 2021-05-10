// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	registrytest "github.com/tsuru/tsuru/registry/testing"
	check "gopkg.in/check.v1"
)

type S struct {
	server *registrytest.RegistryServer
}

var suiteInstance = &S{}
var _ = check.Suite(suiteInstance)

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) SetUpSuite(c *check.C) {
	var err error
	s.server, err = registrytest.NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	config.Set("registry", "docker")
	config.Set("docker:registry", s.server.Addr())
}

func (s *S) SetUpTest(c *check.C) {
	config.Set("docker:registry", s.server.Addr())
}

func (s *S) TearDownSuite(c *check.C) {
	s.server.Stop()
}

func (s *S) TearDownTest(c *check.C) {
	s.server.Reset()
}

func (s *S) TestRegistryRemoveAppImagesErrorImageNotFound(c *check.C) {
	err := RemoveAppImages(context.TODO(), "test")
	c.Assert(err, check.NotNil)
}

func (s *S) TestRegistryRemoveAppImagesErrorErrDeleteDisabled(c *check.C) {
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-test", Tags: map[string]string{"v1": "abcdefg"}})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
	s.server.SetStorageDelete(false)
	err := RemoveAppImages(context.TODO(), "test")
	c.Assert(errors.Cause(err), check.Equals, ErrDeleteDisabled)
}

func (s *S) TestRegistryRemoveAppImages(c *check.C) {
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-test", Tags: map[string]string{"v1": "abcdefg", "v2": "hijklmn"}})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 2)
	err := RemoveAppImages(context.TODO(), "test")
	c.Assert(err, check.IsNil)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 0)
}

func (s *S) TestRegistryRemoveImage(c *check.C) {
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-test", Tags: map[string]string{"v1": "abcdefg", "v2": "hijklmn"}})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 2)
	err := RemoveImage(context.TODO(), s.server.Addr()+"/tsuru/app-test:v1")
	c.Assert(err, check.IsNil)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
}

func (s *S) TestRegistryRemoveImageWithAuth(c *check.C) {
	config.Set("docker:registry-auth:username", "user")
	defer config.Unset("docker:registry-auth:username")
	config.Set("docker:registry-auth:password", "pwd")
	defer config.Unset("docker:registry-auth:password")
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-test", Tags: map[string]string{"v1": "abcdefg", "v2": "hijklmn"}, Username: "user", Password: "pwd"})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 2)
	err := RemoveImage(context.TODO(), s.server.Addr()+"/tsuru/app-test:v1")
	c.Assert(err, check.IsNil)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
}

func (s *S) TestRegistryRemoveImageWithAuthBadCredentials(c *check.C) {
	config.Set("docker:registry-auth:username", "user")
	defer config.Unset("docker:registry-auth:username")
	config.Set("docker:registry-auth:password", "wrong-pwd")
	defer config.Unset("docker:registry-auth:password")
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-test", Tags: map[string]string{"v1": "abcdefg", "v2": "hijklmn"}, Username: "user", Password: "pwd"})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 2)
	err := RemoveImage(context.TODO(), s.server.Addr()+"/tsuru/app-test:v1")
	c.Assert(err, check.NotNil)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 2)
}

func (s *S) TestRegistryRemoveImageNoRegistry(c *check.C) {
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-test", Tags: map[string]string{"v1": "abcdefg"}})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
	err := RemoveImage(context.TODO(), "tsuru/app-test:v1")
	c.Assert(err, check.IsNil)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 0)
}

func (s *S) TestRegistryRemoveImageUnknownRegistry(c *check.C) {
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-test", Tags: map[string]string{"v1": "abcdefg"}})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
	err := RemoveImage(context.TODO(), "fake-registry:5000/tsuru/app-test:v1")
	c.Assert(err, check.ErrorMatches, `.*failed to get digest for image.*`)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
}

func (s *S) TestRegistryRemoveImageUnknownTag(c *check.C) {
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-test", Tags: map[string]string{"v1": "abcdefg"}})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
	err := RemoveImage(context.TODO(), s.server.Addr()+"/tsuru/app-test:v0")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "failed to get digest for image "+s.server.Addr()+"/tsuru/app-test:v0 on registry: digest not found")
	c.Assert(errors.Cause(err), check.Equals, ErrDigestNotFound)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
}

func (s *S) TestRegistryRemoveImageEmpty(c *check.C) {
	err := RemoveImage(context.TODO(), "")
	c.Assert(err, check.ErrorMatches, `empty image.*`)
}

func (s *S) TestRegistryRemoveImageDigestNotFound(c *check.C) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Docker-Content-Digest", "xyz")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	u, _ := url.Parse(srv.URL)
	err := RemoveImage(context.TODO(), u.Host+"/tsuru/app-test:v1")
	c.Assert(err, check.ErrorMatches, `failed to remove image .* on registry: image not found`)
	c.Assert(errors.Cause(err), check.Equals, ErrImageNotFound)
}

func (s *S) TestRegistryRemoveImageEmptyDigest(c *check.C) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	u, _ := url.Parse(srv.URL)
	err := RemoveImage(context.TODO(), u.Host+"/tsuru/app-test:v1")
	c.Assert(err, check.ErrorMatches, `.*empty digest returned for image tsuru/app-test:v1.*`)
}

func (s *S) TestDockerRegistryDoRequest(c *check.C) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	r := dockerRegistry{
		server: srv.URL,
		client: srv.Client(),
	}
	rsp, err := r.doRequest(context.TODO(), "GET", "/", nil)
	c.Assert(err, check.IsNil)
	c.Assert(rsp.StatusCode, check.Equals, http.StatusOK)
}

func (s *S) TestDockerRegistryDoRequestTLS(c *check.C) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	r := dockerRegistry{
		server: srv.URL,
		client: srv.Client(),
	}
	rsp, err := r.doRequest(context.TODO(), "GET", "/", nil)
	c.Assert(err, check.IsNil)
	c.Assert(rsp.StatusCode, check.Equals, http.StatusOK)
}
