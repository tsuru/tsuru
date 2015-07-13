// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"net/http"
	"strings"

	"github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

const (
	pullOutputDigest = `{"status":"Pulling from tsuru/bs","id":"latest"}
{"status":"Already exists","progressDetail":{},"id":"428b411c28f0"}{"status":"Already exists","progressDetail":{},"id":"435050075b3f"}{"status":"Already exists","progressDetail":{},"id":"9fd3c8c9af32"}{"status":"Already exists","progressDetail":{},"id":"6d4946999d4f"}{"status":"Already exists","progressDetail":{},"id":"ad1fc4a2d1ca"}{"status":"Already exists","progressDetail":{},"id":"c5f8e17b5f1c"}{"status":"Already exists","progressDetail":{},"id":"c5f8e17b5f1c"}{"status":"Digest: sha256:7f75ad504148650f26429543007607dd84886b54ffc9cdf8879ea8ba4c5edb7d"}
{"status":"Status: Image is up to date for tsuru/bs"}`
	pullOutputNoDigest = `{"status":"Pulling from tsuru/bs","id":"latest"}
{"status":"Already exists","progressDetail":{},"id":"428b411c28f0"}{"status":"Already exists","progressDetail":{},"id":"435050075b3f"}{"status":"Already exists","progressDetail":{},"id":"9fd3c8c9af32"}{"status":"Already exists","progressDetail":{},"id":"6d4946999d4f"}{"status":"Already exists","progressDetail":{},"id":"ad1fc4a2d1ca"}{"status":"Already exists","progressDetail":{},"id":"c5f8e17b5f1c"}{"status":"Already exists","progressDetail":{},"id":"c5f8e17b5f1c"}
{"status":"Status: Image is up to date for tsuru/bs"}`
	digest = "sha256:7f75ad504148650f26429543007607dd84886b54ffc9cdf8879ea8ba4c5edb7d"
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
	server.CustomHandler("/images/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		server.DefaultHandler().ServeHTTP(w, r)
		w.Write([]byte(pullOutputDigest))
	}))
	client, err := docker.NewClient(server.URL())
	c.Assert(err, check.IsNil)
	err = client.PullImage(docker.PullImageOptions{
		Repository: "base",
	}, docker.AuthConfiguration{})
	_, err = client.CreateContainer(docker.CreateContainerOptions{
		Name:       "big-sibling",
		Config:     &docker.Config{Image: "base"},
		HostConfig: &docker.HostConfig{},
	})
	c.Assert(err, check.IsNil)
	config.Set("host", "127.0.0.1:8080")
	config.Set("docker:bs:image", "myregistry/tsuru/bs")
	err = createBsContainer(server.URL(), "pool1")
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, "big-sibling")
	expectedBinding := []docker.PortBinding{{HostIP: "0.0.0.0", HostPort: "1514"}}
	c.Assert(container.HostConfig.PortBindings[docker.Port("514/udp")], check.DeepEquals, expectedBinding)
	_, ok := container.Config.ExposedPorts[docker.Port("1514/udp")]
	c.Assert(ok, check.Equals, true)
	c.Assert(container.Config.Image, check.Equals, "myregistry/tsuru/bs")
	c.Assert(container.HostConfig.RestartPolicy, check.Equals, docker.AlwaysRestart())
	c.Assert(container.State.Running, check.Equals, true)
	expectedEnv := map[string]string{
		"DOCKER_ENDPOINT":       server.URL(),
		"TSURU_ENDPOINT":        "http://127.0.0.1:8080/",
		"TSURU_TOKEN":           "abc123",
		"SYSLOG_LISTEN_ADDRESS": "udp://0.0.0.0:514",
	}
	gotEnv := parseEnvs(container.Config.Env)
	_, ok = gotEnv["TSURU_TOKEN"]
	c.Assert(ok, check.Equals, true)
	gotEnv["TSURU_TOKEN"] = expectedEnv["TSURU_TOKEN"]
	c.Assert(gotEnv, check.DeepEquals, expectedEnv)
	conf, err := loadBsConfig()
	c.Assert(err, check.IsNil)
	c.Assert(conf.Image, check.Equals, "myregistry/tsuru/bs@"+digest)
}

func (s *S) TestCreateBsContainerTaggedBs(c *check.C) {
	server, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.CustomHandler("/images/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		server.DefaultHandler().ServeHTTP(w, r)
		w.Write([]byte(pullOutputDigest))
	}))
	client, err := docker.NewClient(server.URL())
	c.Assert(err, check.IsNil)
	err = client.PullImage(docker.PullImageOptions{
		Repository: "base",
	}, docker.AuthConfiguration{})
	_, err = client.CreateContainer(docker.CreateContainerOptions{
		Name:       "big-sibling",
		Config:     &docker.Config{Image: "base"},
		HostConfig: &docker.HostConfig{},
	})
	c.Assert(err, check.IsNil)
	config.Set("host", "127.0.0.1:8080")
	config.Set("docker:bs:image", "localhost:5000/myregistry/tsuru/bs:v1")
	err = createBsContainer(server.URL(), "pool1")
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, "big-sibling")
	expectedBinding := []docker.PortBinding{{HostIP: "0.0.0.0", HostPort: "1514"}}
	c.Assert(container.HostConfig.PortBindings[docker.Port("514/udp")], check.DeepEquals, expectedBinding)
	_, ok := container.Config.ExposedPorts[docker.Port("1514/udp")]
	c.Assert(ok, check.Equals, true)
	c.Assert(container.Config.Image, check.Equals, "localhost:5000/myregistry/tsuru/bs:v1")
	c.Assert(container.HostConfig.RestartPolicy, check.Equals, docker.AlwaysRestart())
	c.Assert(container.State.Running, check.Equals, true)
	expectedEnv := map[string]string{
		"DOCKER_ENDPOINT":       server.URL(),
		"TSURU_ENDPOINT":        "http://127.0.0.1:8080/",
		"TSURU_TOKEN":           "abc123",
		"SYSLOG_LISTEN_ADDRESS": "udp://0.0.0.0:514",
	}
	gotEnv := parseEnvs(container.Config.Env)
	_, ok = gotEnv["TSURU_TOKEN"]
	c.Assert(ok, check.Equals, true)
	gotEnv["TSURU_TOKEN"] = expectedEnv["TSURU_TOKEN"]
	c.Assert(gotEnv, check.DeepEquals, expectedEnv)
	conf, err := loadBsConfig()
	c.Assert(err, check.IsNil)
	c.Assert(conf.Image, check.Equals, "localhost:5000/myregistry/tsuru/bs:v1")
}

func (s *S) TestCreateBsContainerSocketAndCustomSysLogPort(c *check.C) {
	server, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.CustomHandler("/images/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		server.DefaultHandler().ServeHTTP(w, r)
		w.Write([]byte(pullOutputNoDigest))
	}))
	config.Set("host", "127.0.0.1:8080")
	config.Set("docker:bs:image", "myregistry/tsuru/bs")
	config.Set("docker:bs:socket", "/tmp/docker.sock")
	defer config.Unset("docker:bs:socket")
	config.Set("docker:bs:syslog-port", 1519)
	defer config.Unset("docker:bs:syslog-port")
	err = saveBsEnvs(bsEnvMap{
		"VAR1": "VALUE1",
		"VAR2": "VALUE2",
	}, bsPoolEnvMap{
		"pool1": bsEnvMap{
			"VAR2": "VALUE_FOR_POOL1",
		},
		"pool2": bsEnvMap{
			"VAR2": "VALUE_FOR_POOL2",
		},
	})
	c.Assert(err, check.IsNil)
	err = createBsContainer(server.URL(), "pool1")
	c.Assert(err, check.IsNil)
	client, err := docker.NewClient(server.URL())
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, "big-sibling")
	c.Assert(container.HostConfig.Binds, check.DeepEquals, []string{"/tmp/docker.sock:/var/run/docker.sock:rw"})
	expectedBinding := []docker.PortBinding{{HostIP: "0.0.0.0", HostPort: "1519"}}
	c.Assert(container.HostConfig.PortBindings[docker.Port("514/udp")], check.DeepEquals, expectedBinding)
	_, ok := container.Config.ExposedPorts[docker.Port("1519/udp")]
	c.Assert(ok, check.Equals, true)
	c.Assert(container.Config.Image, check.Equals, "myregistry/tsuru/bs")
	c.Assert(container.State.Running, check.Equals, true)
	expectedEnv := map[string]string{
		"DOCKER_ENDPOINT":       "unix:///var/run/docker.sock",
		"TSURU_ENDPOINT":        "http://127.0.0.1:8080/",
		"TSURU_TOKEN":           "abc123",
		"SYSLOG_LISTEN_ADDRESS": "udp://0.0.0.0:514",
		"VAR1":                  "VALUE1",
		"VAR2":                  "VALUE_FOR_POOL1",
	}
	gotEnv := parseEnvs(container.Config.Env)
	_, ok = gotEnv["TSURU_TOKEN"]
	c.Assert(ok, check.Equals, true)
	gotEnv["TSURU_TOKEN"] = expectedEnv["TSURU_TOKEN"]
	c.Assert(gotEnv, check.DeepEquals, expectedEnv)
	conf, err := loadBsConfig()
	c.Assert(err, check.IsNil)
	c.Assert(conf.Image, check.Equals, "myregistry/tsuru/bs")
}

func parseEnvs(envs []string) map[string]string {
	result := make(map[string]string, len(envs))
	for _, env := range envs {
		parts := strings.SplitN(env, "=", 2)
		result[parts[0]] = parts[1]
	}
	return result
}
