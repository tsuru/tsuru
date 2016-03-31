// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bs

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/provision/docker/dockertest"
	"github.com/tsuru/tsuru/safe"
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

func (s *S) TestAddNewContainer(c *check.C) {
	config := NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image:        "myimg",
			Memory:       100,
			ExposedPorts: map[docker.Port]struct{}{docker.Port("80/tcp"): {}},
			Env: []string{
				"A=1",
				"B=2",
			},
		},
		HostConfig: docker.HostConfig{
			Privileged: true,
			Binds:      []string{"/xyz:/abc:rw"},
			PortBindings: map[docker.Port][]docker.PortBinding{
				docker.Port("80/tcp"): {{HostIP: "", HostPort: ""}},
			},
			LogConfig: docker.LogConfig{
				Type:   "syslog",
				Config: map[string]string{"a": "b", "c": "d"},
			},
		},
	}
	err := AddNewContainer("", &config)
	c.Assert(err, check.IsNil)
	conf := configFor(config.Name)
	var result1 NodeContainerConfig
	err = conf.Load("", &result1)
	c.Assert(err, check.IsNil)
	c.Assert(result1, check.DeepEquals, config)
	config2 := NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Env: []string{"C=3"},
		},
		HostConfig: docker.HostConfig{
			LogConfig: docker.LogConfig{
				Config: map[string]string{"a": "", "e": "f"},
			},
		},
	}
	err = AddNewContainer("p1", &config2)
	c.Assert(err, check.IsNil)
	var result2 NodeContainerConfig
	err = conf.Load("", &result2)
	c.Assert(err, check.IsNil)
	c.Assert(result2, check.DeepEquals, config)
	var result3 NodeContainerConfig
	err = conf.Load("p1", &result3)
	c.Assert(err, check.IsNil)
	expected2 := config
	expected2.Config.Env = []string{"A=1", "B=2", "C=3"}
	expected2.HostConfig.LogConfig.Config = map[string]string{"a": "", "c": "d", "e": "f"}
	c.Assert(result3, check.DeepEquals, expected2)
}

func (s *S) TestEnsureContainersStarted(c *check.C) {
	c1 := NodeContainerConfig{
		Name: "bs",
		Config: docker.Config{
			Image: "bsimg",
			Env: []string{
				"A=1",
				"B=2",
			},
		},
		HostConfig: docker.HostConfig{
			RestartPolicy: docker.AlwaysRestart(),
			Privileged:    true,
			Binds:         []string{"/xyz:/abc:rw"},
		},
	}
	err := AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	c2 := c1
	c2.Name = "sysdig"
	c2.Config.Image = "sysdigimg"
	c2.Config.Env = []string{"X=Z"}
	err = AddNewContainer("", &c2)
	c.Assert(err, check.IsNil)
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	var createBodies []string
	var names []string
	var mut sync.Mutex
	server := p.Servers()[0]
	server.CustomHandler("/containers/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mut.Lock()
		defer mut.Unlock()
		data, _ := ioutil.ReadAll(r.Body)
		createBodies = append(createBodies, string(data))
		names = append(names, r.URL.Query().Get("name"))
		r.Body = ioutil.NopCloser(bytes.NewBuffer(data))
		server.DefaultHandler().ServeHTTP(w, r)
	}))
	defer p.Destroy()
	buf := safe.NewBuffer(nil)
	err = ensureContainersStarted(p, buf, true)
	c.Assert(err, check.IsNil)
	parts := strings.Split(buf.String(), "\n")
	c.Assert(parts, check.HasLen, 5)
	sort.Strings(parts)
	c.Assert(parts[1], check.Matches, `relaunching node container "bs" in the node http://127.0.0.1:\d+/ \[\]`)
	c.Assert(parts[2], check.Matches, `relaunching node container "bs" in the node http://localhost:\d+/ \[\]`)
	c.Assert(parts[3], check.Matches, `relaunching node container "sysdig" in the node http://127.0.0.1:\d+/ \[\]`)
	c.Assert(parts[4], check.Matches, `relaunching node container "sysdig" in the node http://localhost:\d+/ \[\]`)
	c.Assert(createBodies, check.HasLen, 2)
	c.Assert(names, check.HasLen, 2)
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"bs", "sysdig"})
	sort.Strings(createBodies)
	result := make([]struct {
		docker.Config
		HostConfig docker.HostConfig
	}, 2)
	err = json.Unmarshal([]byte(createBodies[0]), &result[0])
	c.Assert(err, check.IsNil)
	err = json.Unmarshal([]byte(createBodies[1]), &result[1])
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, []struct {
		docker.Config
		HostConfig docker.HostConfig
	}{
		{
			Config: docker.Config{Env: []string{"DOCKER_ENDPOINT=" + server.URL(), "A=1", "B=2"}, Image: "bsimg"},
			HostConfig: docker.HostConfig{
				Binds:         []string{"/xyz:/abc:rw"},
				Privileged:    true,
				RestartPolicy: docker.RestartPolicy{Name: "always"},
				LogConfig:     docker.LogConfig{},
			},
		},
		{
			Config: docker.Config{Env: []string{"DOCKER_ENDPOINT=" + server.URL(), "X=Z"}, Image: "sysdigimg"},
			HostConfig: docker.HostConfig{
				Binds:         []string{"/xyz:/abc:rw"},
				Privileged:    true,
				RestartPolicy: docker.RestartPolicy{Name: "always"},
				LogConfig:     docker.LogConfig{},
			},
		},
	})
	conf := configFor("bs")
	var result1 NodeContainerConfig
	err = conf.Load("", &result1)
	c.Assert(err, check.IsNil)
	c.Assert(result1.PinnedImage, check.Equals, "bsimg")
	client, err := docker.NewClient(p.Servers()[0].URL())
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	client, err = docker.NewClient(p.Servers()[1].URL())
	containers, err = client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
}

func (s *S) TestEnsureContainersStartedPinImg(c *check.C) {
	config.Set("docker:bs:image", "myregistry/tsuru/bs")
	_, err := InitializeBS()
	c.Assert(err, check.IsNil)
	server, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.CustomHandler("/images/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		server.DefaultHandler().ServeHTTP(w, r)
		w.Write([]byte(pullOutputDigest))
	}))
	p, err := dockertest.NewFakeDockerProvisioner(server.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	client, err := docker.NewClient(server.URL())
	c.Assert(err, check.IsNil)
	err = client.PullImage(docker.PullImageOptions{
		Repository: "base",
	}, docker.AuthConfiguration{})
	c.Assert(err, check.IsNil)
	_, err = client.CreateContainer(docker.CreateContainerOptions{
		Name:       BsDefaultName,
		Config:     &docker.Config{Image: "base"},
		HostConfig: &docker.HostConfig{},
	})
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	err = ensureContainersStarted(p, buf, true)
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, BsDefaultName)
	c.Assert(container.Config.Image, check.Equals, "myregistry/tsuru/bs")
	c.Assert(container.HostConfig.RestartPolicy, check.Equals, docker.AlwaysRestart())
	c.Assert(container.State.Running, check.Equals, true)
	nodeContainer, err := LoadNodeContainer("", BsDefaultName)
	c.Assert(err, check.IsNil)
	c.Assert(nodeContainer.PinnedImage, check.Equals, "myregistry/tsuru/bs@"+digest)
}

func (s *S) TestClusterHookBeforeCreateContainer(c *check.C) {
	_, err := InitializeBS()
	c.Assert(err, check.IsNil)
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	hook := ClusterHook{Provisioner: p}
	err = hook.RunClusterHook(cluster.HookEventBeforeContainerCreate, &nodes[0])
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	client, err := nodes[0].Client()
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, BsDefaultName)
	client, err = nodes[1].Client()
	c.Assert(err, check.IsNil)
	containers, err = client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 0)
}

func (s *S) TestClusterHookBeforeCreateContainerIgnoresExistingError(c *check.C) {
	_, err := InitializeBS()
	c.Assert(err, check.IsNil)
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	var buf safe.Buffer
	err = RecreateContainers(p, &buf)
	c.Assert(err, check.IsNil)
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	hook := ClusterHook{Provisioner: p}
	err = hook.RunClusterHook(cluster.HookEventBeforeContainerCreate, &nodes[0])
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	client, err := nodes[0].Client()
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, BsDefaultName)
	client, err = nodes[1].Client()
	c.Assert(err, check.IsNil)
	containers, err = client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err = client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, BsDefaultName)
}

func (s *S) TestClusterHookBeforeCreateContainerStartsStopped(c *check.C) {
	_, err := InitializeBS()
	c.Assert(err, check.IsNil)
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	var buf safe.Buffer
	err = RecreateContainers(p, &buf)
	c.Assert(err, check.IsNil)
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	client, err := nodes[0].Client()
	c.Assert(err, check.IsNil)
	err = client.StopContainer(BsDefaultName, 1)
	c.Assert(err, check.IsNil)
	contData, err := client.InspectContainer(BsDefaultName)
	c.Assert(err, check.IsNil)
	c.Assert(contData.State.Running, check.Equals, false)
	hook := ClusterHook{Provisioner: p}
	err = hook.RunClusterHook(cluster.HookEventBeforeContainerCreate, &nodes[0])
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	client, err = nodes[0].Client()
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, BsDefaultName)
	c.Assert(container.State.Running, check.Equals, true)
	client, err = nodes[1].Client()
	c.Assert(err, check.IsNil)
	containers, err = client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err = client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, BsDefaultName)
	c.Assert(container.State.Running, check.Equals, true)
}
