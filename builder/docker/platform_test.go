// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/check.v1"
)

func (s *S) TestPlatformAdd(c *check.C) {
	var requests []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	err = s.provisioner.AddNode(provision.AddNodeOptions{Address: server.URL()})
	c.Assert(err, check.IsNil)
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	var b dockerBuilder
	dockerfile := "FROM tsuru/java"
	err = b.PlatformAdd(appTypes.PlatformOptions{
		Name:      "test",
		ImageName: "localhost:3030/tsuru/test:v1",
		Args:      map[string]string{"dockerfile": "http://localhost"},
		Output:    ioutil.Discard,
		Input:     strings.NewReader(dockerfile),
	})
	c.Assert(err, check.IsNil)
	c.Assert(len(requests) >= 2, check.Equals, true)
	requests = requests[len(requests)-2:]
	c.Assert(requests[0].URL.Path, check.Equals, "/build")
	queryString := requests[0].URL.Query()
	c.Assert(queryString.Get("t"), check.Equals, "localhost:3030/tsuru/test:v1")
	c.Assert(queryString.Get("remote"), check.Equals, "")
	c.Assert(requests[1].URL.Path, check.Equals, "/images/localhost:3030/tsuru/test/push")
}

func (s *S) TestPlatformAddProvisionerError(c *check.C) {
	var requests []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	b := dockerBuilder{}
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = b.PlatformAdd(appTypes.PlatformOptions{
		Name:   "test",
		Args:   args,
		Output: ioutil.Discard,
	})
	c.Assert(err, check.ErrorMatches, "(?m).*No node found.*")
}

func (s *S) TestPlatformAddNoProvisioner(c *check.C) {
	provision.Unregister("fake")
	defer func() {
		provision.Register("fake", func() (provision.Provisioner, error) {
			return provisiontest.ProvisionerInstance, nil
		})
	}()
	var requests []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	b := dockerBuilder{}
	args := make(map[string]string)
	args["dockerfile"] = "http://localhost/Dockerfile"
	err = b.PlatformAdd(appTypes.PlatformOptions{
		Name:   "test",
		Args:   args,
		Output: ioutil.Discard,
	})
	c.Assert(err, check.ErrorMatches, "No Docker nodes available")
}

func (s *S) TestPlatformRemove(c *check.C) {
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	var requests []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	err = s.provisioner.AddNode(provision.AddNodeOptions{Address: server.URL()})
	c.Assert(err, check.IsNil)
	var buf bytes.Buffer
	var b dockerBuilder
	err = b.PlatformAdd(appTypes.PlatformOptions{
		Name:      "test",
		ImageName: "localhost:3030/tsuru/test:v1",
		Args:      map[string]string{"dockerfile": "http://localhost/Dockerfile"},
		Output:    &buf,
	})
	c.Assert(err, check.IsNil)
	s.mockService.PlatformImage.OnListImages = func(name string) ([]string, error) {
		c.Assert(name, check.Equals, "test")
		return []string{"localhost:3030/tsuru/test:v1"}, nil
	}
	err = b.PlatformRemove("test")
	c.Assert(err, check.IsNil)
	c.Assert(len(requests) >= 4, check.Equals, true)
	requests = requests[len(requests)-4:]
	c.Assert(requests[2].URL.Path, check.Matches, "/images/localhost:3030/tsuru/test:v1/json")
	c.Assert(requests[3].Method, check.Equals, "DELETE")
	c.Assert(requests[3].URL.Path, check.Matches, "/images/[^/]+")
}

func (s *S) TestPlatformRemoveProvisionerError(c *check.C) {
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	var requests []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	var b dockerBuilder
	err = b.PlatformRemove("test")
	c.Assert(err, check.ErrorMatches, "(?m).*No node found.*")
}

func (s *S) TestPlatformRemoveNoProvisioner(c *check.C) {
	provision.Unregister("fake")
	defer func() {
		provision.Register("fake", func() (provision.Provisioner, error) {
			return provisiontest.ProvisionerInstance, nil
		})
	}()
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	var requests []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		requests = append(requests, r)
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	var b dockerBuilder
	err = b.PlatformRemove("test")
	c.Assert(err, check.ErrorMatches, "No Docker nodes available")
}
