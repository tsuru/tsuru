// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bs

import (
	"net/http"
	"sort"

	"github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/provision/docker/dockertest"
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
	p, err := dockertest.NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	err = createContainer(server.URL(), "pool1", p, true)
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, "big-sibling")
	c.Assert(container.Config.Image, check.Equals, "myregistry/tsuru/bs")
	c.Assert(container.HostConfig.RestartPolicy, check.Equals, docker.AlwaysRestart())
	c.Assert(container.State.Running, check.Equals, true)
	conf, err := LoadConfig(nil)
	c.Assert(err, check.IsNil)
	token, _ := getToken(conf)
	expectedEnv := []string{
		"DOCKER_ENDPOINT=" + server.URL(),
		"TSURU_ENDPOINT=http://127.0.0.1:8080/",
		"TSURU_TOKEN=" + token,
		"SYSLOG_LISTEN_ADDRESS=udp://0.0.0.0:1514",
	}
	sort.Strings(expectedEnv)
	sort.Strings(container.Config.Env)
	c.Assert(container.Config.Env, check.DeepEquals, expectedEnv)
	c.Assert(conf.GetExtraString("image"), check.Equals, "myregistry/tsuru/bs@"+digest)
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
	p, err := dockertest.NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	err = createContainer(server.URL(), "pool1", p, true)
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, "big-sibling")
	c.Assert(container.Config.Image, check.Equals, "localhost:5000/myregistry/tsuru/bs:v1")
	c.Assert(container.HostConfig.RestartPolicy, check.Equals, docker.AlwaysRestart())
	c.Assert(container.HostConfig.Privileged, check.Equals, true)
	c.Assert(container.HostConfig.NetworkMode, check.Equals, "host")
	c.Assert(container.State.Running, check.Equals, true)
	conf, err := LoadConfig(nil)
	c.Assert(err, check.IsNil)
	token, _ := getToken(conf)
	expectedEnv := []string{
		"DOCKER_ENDPOINT=" + server.URL(),
		"TSURU_ENDPOINT=http://127.0.0.1:8080/",
		"TSURU_TOKEN=" + token,
		"SYSLOG_LISTEN_ADDRESS=udp://0.0.0.0:1514",
	}
	sort.Strings(expectedEnv)
	sort.Strings(container.Config.Env)
	c.Assert(container.Config.Env, check.DeepEquals, expectedEnv)
	c.Assert(conf.GetExtraString("image"), check.Equals, "localhost:5000/myregistry/tsuru/bs:v1")
}

func (s *S) TestCreateBsContainerAlreadyPinned(c *check.C) {
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
	config.Set("docker:bs:image", "localhost:5000/myregistry/tsuru/bs@"+digest)
	p, err := dockertest.NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	err = createContainer(server.URL(), "pool1", p, true)
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, "big-sibling")
	c.Assert(container.Config.Image, check.Equals, "localhost:5000/myregistry/tsuru/bs@"+digest)
	c.Assert(container.HostConfig.RestartPolicy, check.Equals, docker.AlwaysRestart())
	c.Assert(container.HostConfig.Privileged, check.Equals, true)
	c.Assert(container.HostConfig.NetworkMode, check.Equals, "host")
	c.Assert(container.State.Running, check.Equals, true)
	conf, err := LoadConfig(nil)
	c.Assert(err, check.IsNil)
	token, _ := getToken(conf)
	expectedEnv := []string{
		"DOCKER_ENDPOINT=" + server.URL(),
		"TSURU_ENDPOINT=http://127.0.0.1:8080/",
		"TSURU_TOKEN=" + token,
		"SYSLOG_LISTEN_ADDRESS=udp://0.0.0.0:1514",
	}
	sort.Strings(expectedEnv)
	sort.Strings(container.Config.Env)
	c.Assert(container.Config.Env, check.DeepEquals, expectedEnv)
	c.Assert(conf.GetExtraString("image"), check.Equals, "localhost:5000/myregistry/tsuru/bs@"+digest)
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
	conf, err := LoadConfig(nil)
	c.Assert(err, check.IsNil)
	conf.Add("VAR1", "VALUE1")
	conf.Add("VAR2", "VALUE2")
	conf.Add("TSURU_ENDPOINT", "ignored")
	conf.AddPool("pool1", "VAR2", "VALUE_FOR_POOL1")
	conf.AddPool("pool2", "VAR2", "VALUE_FOR_POOL2")
	conf.AddPool("pool1", "SYSLOG_LISTEN_ADDRESS", "alsoignored")
	err = conf.SaveEnvs()
	c.Assert(err, check.IsNil)
	p, err := dockertest.NewFakeDockerProvisioner()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	err = createContainer(server.URL(), "pool1", p, true)
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
	c.Assert(container.Config.Image, check.Equals, "myregistry/tsuru/bs")
	c.Assert(container.State.Running, check.Equals, true)
	conf, err = LoadConfig(nil)
	c.Assert(err, check.IsNil)
	token, _ := getToken(conf)
	expectedEnv := []string{
		"DOCKER_ENDPOINT=unix:///var/run/docker.sock",
		"TSURU_ENDPOINT=http://127.0.0.1:8080/",
		"TSURU_TOKEN=" + token,
		"SYSLOG_LISTEN_ADDRESS=udp://0.0.0.0:1519",
		"VAR1=VALUE1",
		"VAR2=VALUE_FOR_POOL1",
	}
	sort.Strings(expectedEnv)
	sort.Strings(container.Config.Env)
	c.Assert(container.Config.Env, check.DeepEquals, expectedEnv)
	c.Assert(conf.GetExtraString("image"), check.Equals, "myregistry/tsuru/bs")
}
