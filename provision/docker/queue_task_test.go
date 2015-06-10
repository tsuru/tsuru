// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"strings"

	"github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

func (s *S) TestWaitDocker(c *check.C) {
	server, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server.Stop()
	var task runBs
	err = task.waitDocker(server.URL())
	c.Assert(err, check.IsNil)
	config.Set("docker:api-timeout", 1)
	defer config.Unset("docker:api-timeout")
	err = task.waitDocker("http://169.254.169.254:2375/")
	c.Assert(err, check.NotNil)
	expectedMsg := `Docker API at "http://169.254.169.254:2375/" didn't respond after 1 seconds`
	c.Assert(err.Error(), check.Equals, expectedMsg)
}

func (s *S) TestCreateBsContainer(c *check.C) {
	server, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server.Stop()
	config.Set("host", "127.0.0.1:8080")
	config.Set("docker:bs-image", "myregistry/tsuru/bs")
	var task runBs
	err = task.createBsContainer(server.URL())
	c.Assert(err, check.IsNil)
	client, err := docker.NewClient(server.URL())
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, "big-sibling")
	c.Assert(container.Config.Image, check.Equals, "myregistry/tsuru/bs")
	c.Assert(container.State.Running, check.Equals, true)
	expectedEnv := map[string]string{
		"DOCKER_ENDPOINT":        server.URL(),
		"TSURU_ENDPOINT":         "http://127.0.0.1:8080/",
		"TSURU_TOKEN":            "abc123",
		"TSURU_SENTINEL_ENV_VAR": "TSURU_APPNAME",
	}
	gotEnv := parseEnvs(container.Config.Env)
	_, ok := gotEnv["TSURU_TOKEN"]
	c.Assert(ok, check.Equals, true)
	gotEnv["TSURU_TOKEN"] = expectedEnv["TSURU_TOKEN"]
	c.Assert(gotEnv, check.DeepEquals, expectedEnv)
}

func parseEnvs(envs []string) map[string]string {
	result := make(map[string]string, len(envs))
	for _, env := range envs {
		parts := strings.SplitN(env, "=", 2)
		result[parts[0]] = parts[1]
	}
	return result
}
