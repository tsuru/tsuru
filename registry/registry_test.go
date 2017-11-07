// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package registry

import (
	"net/http"
	"testing"

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

func (s *S) TearDownSuite(c *check.C) {
	s.server.Stop()
}

func (s *S) TearDownTest(c *check.C) {
	s.server.Reset()
}

func (s *S) TestRegistryRemoveAppImagesErrorImageNotFound(c *check.C) {
	err := RemoveAppImages("teste")
	c.Assert(err, check.NotNil)
}

func (s *S) TestRegistryRemoveAppImagesErrorStorageDeleteDisabled(c *check.C) {
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-teste", Tags: map[string]string{"v1": "abcdefg"}})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
	s.server.SetStorageDelete(false)
	err := RemoveAppImages("teste")
	c.Assert(err, check.NotNil)
	e, ok := err.(*StorageDeleteDisabledError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.StatusCode, check.Equals, http.StatusMethodNotAllowed)
}

func (s *S) TestRegistryRemoveAppImages(c *check.C) {
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-teste", Tags: map[string]string{"v1": "abcdefg", "v2": "hijklmn"}})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 2)
	err := RemoveAppImages("teste")
	c.Assert(err, check.IsNil)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 0)
}

func (s *S) TestRegistryRemoveImage(c *check.C) {
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-teste", Tags: map[string]string{"v1": "abcdefg", "v2": "hijklmn"}})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 2)
	err := RemoveImage(s.server.Addr() + "/tsuru/app-teste:v1")
	c.Assert(err, check.IsNil)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
}

func (s *S) TestRegistryRemoveImageNoRegistry(c *check.C) {
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-teste", Tags: map[string]string{"v1": "abcdefg"}})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
	err := RemoveImage("tsuru/app-teste:v1")
	c.Assert(err, check.IsNil)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 0)
}

func (s *S) TestRegistryRemoveImageUnknownRegistry(c *check.C) {
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-teste", Tags: map[string]string{"v1": "abcdefg"}})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
	err := RemoveImage("fake-registry:5000/tsuru/app-teste:v1")
	c.Assert(err, check.NotNil)
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
}

func (s *S) TestRegistryRemoveImageUnknownTag(c *check.C) {
	s.server.AddRepo(registrytest.Repository{Name: "tsuru/app-teste", Tags: map[string]string{"v1": "abcdefg"}})
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
	err := RemoveImage(s.server.Addr() + "/tsuru/app-teste:v0")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "failed to remove image "+s.server.Addr()+"/tsuru/app-teste:v0/ on registry: repository not found (404)\n")
	c.Assert(s.server.Repos, check.HasLen, 1)
	c.Assert(s.server.Repos[0].Tags, check.HasLen, 1)
}
