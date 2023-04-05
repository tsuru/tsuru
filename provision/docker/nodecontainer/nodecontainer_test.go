// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodecontainer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/dockertest"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/safe"
	check "gopkg.in/check.v1"
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

func (s *S) TestRecreateNamedContainers(c *check.C) {
	config.Set("docker:bs:image", "myregistry/tsuru/bs")
	_, err := nodecontainer.InitializeBS(context.TODO(), s.authScheme, "tsr")
	c.Assert(err, check.IsNil)
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	for i, n := range nodes {
		n.Metadata["pool"] = fmt.Sprintf("p-%d", i)
		_, err = p.Cluster().UpdateNode(n)
		c.Assert(err, check.IsNil)
	}
	server := p.Servers()[0]
	var paths []string
	server.CustomHandler("/containers/.*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		server.DefaultHandler().ServeHTTP(w, r)
	}))
	server2 := p.Servers()[1]
	var paths2 []string
	server2.CustomHandler("/containers/.*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths2 = append(paths2, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		server2.DefaultHandler().ServeHTTP(w, r)
	}))
	expectedPaths := []string{
		"POST /containers/create?name=big-sibling",
		"POST /containers/big-sibling/start?",
	}
	err = RecreateNamedContainers(p, io.Discard, "big-sibling", "")
	c.Assert(err, check.IsNil)
	c.Assert(paths, check.DeepEquals, expectedPaths)
	c.Assert(paths2, check.DeepEquals, expectedPaths)
	err = RemoveNamedContainers(p, io.Discard, "big-sibling", "")
	c.Assert(err, check.IsNil)
	paths = nil
	paths2 = nil
	err = RecreateNamedContainers(p, io.Discard, "big-sibling", "p-1")
	c.Assert(err, check.IsNil)
	c.Assert(paths, check.DeepEquals, []string(nil))
	c.Assert(paths2, check.DeepEquals, expectedPaths)
}

func (s *S) TestEnsureContainersStarted(c *check.C) {
	c1 := nodecontainer.NodeContainerConfig{
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
	err := nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	c2 := c1
	c2.Name = "sysdig"
	c2.Config.Image = "sysdigimg"
	c2.Config.Env = []string{"X=Z"}
	err = nodecontainer.AddNewContainer("", &c2)
	c.Assert(err, check.IsNil)
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	for i, n := range nodes {
		n.Metadata["pool"] = fmt.Sprintf("p-%d", i)
		_, err = p.Cluster().UpdateNode(n)
		c.Assert(err, check.IsNil)
	}
	var createBodies []string
	var names []string
	var mut sync.Mutex
	server := p.Servers()[0]
	server.CustomHandler("/containers/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mut.Lock()
		defer mut.Unlock()
		data, _ := io.ReadAll(r.Body)
		createBodies = append(createBodies, string(data))
		names = append(names, r.URL.Query().Get("name"))
		r.Body = io.NopCloser(bytes.NewBuffer(data))
		server.DefaultHandler().ServeHTTP(w, r)
	}))
	defer p.Destroy()
	buf := safe.NewBuffer(nil)
	err = ensureContainersStarted(p, buf, true, nil)
	c.Assert(err, check.IsNil)
	parts := strings.Split(buf.String(), "\n")
	c.Assert(parts, check.HasLen, 5)
	sort.Strings(parts)
	c.Assert(parts[1], check.Matches, `relaunching node container "bs" in the node http://127.0.0.1:\d+/ \[p-0\]`)
	c.Assert(parts[2], check.Matches, `relaunching node container "bs" in the node http://localhost:\d+/ \[p-1\]`)
	c.Assert(parts[3], check.Matches, `relaunching node container "sysdig" in the node http://127.0.0.1:\d+/ \[p-0\]`)
	c.Assert(parts[4], check.Matches, `relaunching node container "sysdig" in the node http://localhost:\d+/ \[p-1\]`)
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
			Config: docker.Config{Env: []string{"DOCKER_ENDPOINT=" + server.URL(), "A=1", "B=2"}, Image: "bsimg",
				Labels: provision.NodeContainerLabels(provision.NodeContainerLabelsOpts{Name: c1.Name, Pool: "p-0", Provisioner: "fake"}).ToLabels(),
			},
			HostConfig: docker.HostConfig{
				Binds:         []string{"/xyz:/abc:rw"},
				Privileged:    true,
				RestartPolicy: docker.RestartPolicy{Name: "always"},
				LogConfig:     docker.LogConfig{},
			},
		},
		{
			Config: docker.Config{Env: []string{"DOCKER_ENDPOINT=" + server.URL(), "X=Z"}, Image: "sysdigimg",
				Labels: provision.NodeContainerLabels(provision.NodeContainerLabelsOpts{Name: c2.Name, Pool: "p-0", Provisioner: "fake"}).ToLabels(),
			},
			HostConfig: docker.HostConfig{
				Binds:         []string{"/xyz:/abc:rw"},
				Privileged:    true,
				RestartPolicy: docker.RestartPolicy{Name: "always"},
				LogConfig:     docker.LogConfig{},
			},
		},
	})
	nodeContainer, err := nodecontainer.LoadNodeContainer("", nodecontainer.BsDefaultName)
	c.Assert(err, check.IsNil)
	c.Assert(nodeContainer.PinnedImage, check.Equals, "")
	client, err := docker.NewClient(p.Servers()[0].URL())
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
	client, err = docker.NewClient(p.Servers()[1].URL())
	c.Assert(err, check.IsNil)
	containers, err = client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
}

func (s *S) TestEnsureContainersStartedMaxWorkers(c *check.C) {
	config.Set("docker:nodecontainer:max-workers", 1)
	defer config.Unset("docker:nodecontainer:max-workers")
	c1 := nodecontainer.NodeContainerConfig{Name: "bs", Config: docker.Config{Image: "bsimg"}}
	err := nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	done := make(chan struct{})
	begin := make(chan struct{}, 2)
	server := p.Servers()[0]
	var calls int
	server.CustomHandler("/containers/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		begin <- struct{}{}
		server.DefaultHandler().ServeHTTP(w, r)
		<-done
	}))
	server2 := p.Servers()[1]
	server2.CustomHandler("/containers/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		begin <- struct{}{}
		server.DefaultHandler().ServeHTTP(w, r)
	}))
	defer p.Destroy()
	go ensureContainersStarted(p, nil, true, nil)
	<-begin
	select {
	case <-begin:
		c.Fatal("second call should only happen after first finishes")
	case <-time.After(200 * time.Millisecond):
	}
	c.Assert(calls, check.Equals, 1)
	done <- struct{}{}
	select {
	case <-time.After(5 * time.Second):
		c.Fatal("second call should been triggered")
	case <-begin:
	}
	c.Assert(calls, check.Equals, 2)
}

func (s *S) TestEnsureContainersStartedPinImg(c *check.C) {
	config.Set("docker:bs:image", "myregistry/tsuru/bs")
	_, err := nodecontainer.InitializeBS(context.TODO(), s.authScheme, "tsr")
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
		Name:       nodecontainer.BsDefaultName,
		Config:     &docker.Config{Image: "base"},
		HostConfig: &docker.HostConfig{},
	})
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	err = ensureContainersStarted(p, buf, true, nil)
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: containers[0].ID})
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, nodecontainer.BsDefaultName)
	c.Assert(container.Config.Image, check.Equals, "myregistry/tsuru/bs")
	c.Assert(container.HostConfig.RestartPolicy, check.Equals, docker.AlwaysRestart())
	c.Assert(container.State.Running, check.Equals, true)
	nodeContainer, err := nodecontainer.LoadNodeContainer("", nodecontainer.BsDefaultName)
	c.Assert(err, check.IsNil)
	c.Assert(nodeContainer.PinnedImage, check.Equals, "myregistry/tsuru/bs@"+digest)
}

func (s *S) TestEnsureContainersStartedNoDigestNoPin(c *check.C) {
	config.Set("docker:bs:image", "myregistry/tsuru/bs")
	_, err := nodecontainer.InitializeBS(context.TODO(), s.authScheme, "tsr")
	c.Assert(err, check.IsNil)
	server, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.CustomHandler("/images/create", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		server.DefaultHandler().ServeHTTP(w, r)
		w.Write([]byte(pullOutputNoDigest))
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
		Name:       nodecontainer.BsDefaultName,
		Config:     &docker.Config{Image: "base"},
		HostConfig: &docker.HostConfig{},
	})
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	err = ensureContainersStarted(p, buf, true, nil)
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: containers[0].ID})
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, nodecontainer.BsDefaultName)
	c.Assert(container.Config.Image, check.Equals, "myregistry/tsuru/bs")
	c.Assert(container.HostConfig.RestartPolicy, check.Equals, docker.AlwaysRestart())
	c.Assert(container.State.Running, check.Equals, true)
	nodeContainer, err := nodecontainer.LoadNodeContainer("", nodecontainer.BsDefaultName)
	c.Assert(err, check.IsNil)
	c.Assert(nodeContainer.PinnedImage, check.Equals, "")
}

func (s *S) TestEnsureContainersStartedPinImgInParent(c *check.C) {
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "myregistry/tsuru/bs",
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{
		Name: "c1",
	})
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
	node, err := p.Cluster().GetNode(server.URL())
	c.Assert(err, check.IsNil)
	node.Metadata["pool"] = "p1"
	_, err = p.Cluster().UpdateNode(node)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	err = ensureContainersStarted(p, buf, true, nil)
	c.Assert(err, check.IsNil)
	all, err := nodecontainer.LoadNodeContainersForPoolsMerge("c1", false)
	c.Assert(err, check.IsNil)
	c.Assert(all, check.DeepEquals, map[string]nodecontainer.NodeContainerConfig{
		"":   {Name: "c1", PinnedImage: "myregistry/tsuru/bs@" + digest, Config: docker.Config{Image: "myregistry/tsuru/bs"}},
		"p1": {Name: "c1", PinnedImage: ""},
	})
}

func (s *S) TestEnsureContainersStartedPinImgInChild(c *check.C) {
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "myrootimg",
		},
	})
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "myregistry/tsuru/bs",
		},
	})
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
	node, err := p.Cluster().GetNode(server.URL())
	c.Assert(err, check.IsNil)
	node.Metadata["pool"] = "p1"
	_, err = p.Cluster().UpdateNode(node)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	err = ensureContainersStarted(p, buf, true, nil)
	c.Assert(err, check.IsNil)
	all, err := nodecontainer.LoadNodeContainersForPoolsMerge("c1", false)
	c.Assert(err, check.IsNil)
	c.Assert(all, check.DeepEquals, map[string]nodecontainer.NodeContainerConfig{
		"":   {Name: "c1", PinnedImage: "", Config: docker.Config{Image: "myrootimg"}},
		"p1": {Name: "c1", PinnedImage: "myregistry/tsuru/bs@" + digest, Config: docker.Config{Image: "myregistry/tsuru/bs"}},
	})
}

func (s *S) TestEnsureContainersStartedAlreadyPinned(c *check.C) {
	config.Set("docker:bs:image", "myregistry/tsuru/bs")
	_, err := nodecontainer.InitializeBS(context.TODO(), s.authScheme, "tsr")
	c.Assert(err, check.IsNil)
	cont, err := nodecontainer.LoadNodeContainer("", nodecontainer.BsDefaultName)
	c.Assert(err, check.IsNil)
	cont.PinnedImage = "myregistry/tsuru/bs@" + digest
	err = nodecontainer.AddNewContainer("", cont)
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
		Name:       nodecontainer.BsDefaultName,
		Config:     &docker.Config{Image: "base"},
		HostConfig: &docker.HostConfig{},
	})
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	err = ensureContainersStarted(p, buf, true, nil)
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: containers[0].ID})
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, nodecontainer.BsDefaultName)
	c.Assert(container.Config.Image, check.Equals, "myregistry/tsuru/bs@"+digest)
	c.Assert(container.HostConfig.RestartPolicy, check.Equals, docker.AlwaysRestart())
	c.Assert(container.State.Running, check.Equals, true)
	nodeContainer, err := nodecontainer.LoadNodeContainer("", nodecontainer.BsDefaultName)
	c.Assert(err, check.IsNil)
	c.Assert(nodeContainer.PinnedImage, check.Equals, "myregistry/tsuru/bs@"+digest)
}

func (s *S) TestEnsureContainersStartedOnlyChild(c *check.C) {
	err := nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "myregistry/tsuru/bs",
		},
	})
	c.Assert(err, check.IsNil)
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	clust := p.Cluster()
	nodes, err := clust.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	nodes[0].Metadata["pool"] = "p1"
	_, err = p.Cluster().UpdateNode(nodes[0])
	c.Assert(err, check.IsNil)
	nodes[1].Metadata["pool"] = "p2"
	_, err = p.Cluster().UpdateNode(nodes[1])
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	err = ensureContainersStarted(p, buf, true, nil)
	c.Assert(err, check.IsNil)
	client, err := docker.NewClient(nodes[0].Address)
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	client2, err := docker.NewClient(nodes[1].Address)
	c.Assert(err, check.IsNil)
	containers2, err := client2.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers2, check.HasLen, 0)
}

func (s *S) TestClusterHookBeforeCreateContainer(c *check.C) {
	_, err := nodecontainer.InitializeBS(context.TODO(), s.authScheme, "tsr")
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
	container, err := client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: containers[0].ID})
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, nodecontainer.BsDefaultName)
	client, err = nodes[1].Client()
	c.Assert(err, check.IsNil)
	containers, err = client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 0)
}

func (s *S) TestClusterHookBeforeCreateContainerIgnoresExistingError(c *check.C) {
	_, err := nodecontainer.InitializeBS(context.TODO(), s.authScheme, "tsr")
	c.Assert(err, check.IsNil)
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	var buf safe.Buffer
	err = recreateContainers(p, &buf)
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
	container, err := client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: containers[0].ID})
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, nodecontainer.BsDefaultName)
	client, err = nodes[1].Client()
	c.Assert(err, check.IsNil)
	containers, err = client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err = client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: containers[0].ID})
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, nodecontainer.BsDefaultName)
}

func (s *S) TestClusterHookBeforeCreateContainerStartsStopped(c *check.C) {
	_, err := nodecontainer.InitializeBS(context.TODO(), s.authScheme, "tsr")
	c.Assert(err, check.IsNil)
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	var buf safe.Buffer
	err = recreateContainers(p, &buf)
	c.Assert(err, check.IsNil)
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	client, err := nodes[0].Client()
	c.Assert(err, check.IsNil)
	err = client.StopContainer(nodecontainer.BsDefaultName, 1)
	c.Assert(err, check.IsNil)
	contData, err := client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: nodecontainer.BsDefaultName})
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
	container, err := client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: containers[0].ID})
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, nodecontainer.BsDefaultName)
	c.Assert(container.State.Running, check.Equals, true)
	client, err = nodes[1].Client()
	c.Assert(err, check.IsNil)
	containers, err = client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err = client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: containers[0].ID})
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, nodecontainer.BsDefaultName)
	c.Assert(container.State.Running, check.Equals, true)
}

func (s *S) TestLoadNodeContainersForPools(c *check.C) {
	err := nodecontainer.AddNewContainer("p1", &nodecontainer.NodeContainerConfig{
		Name: "c1",
		Config: docker.Config{
			Image: "myregistry/tsuru/bs",
		},
	})
	c.Assert(err, check.IsNil)
	result, err := nodecontainer.LoadNodeContainersForPools("c1")
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, map[string]nodecontainer.NodeContainerConfig{
		"p1": {
			Name: "c1",
			Config: docker.Config{
				Image: "myregistry/tsuru/bs",
			},
		},
	})
}

func (s *S) TestLoadNodeContainersForPoolsNotFound(c *check.C) {
	_, err := nodecontainer.LoadNodeContainersForPools("notfound")
	c.Assert(err, check.Equals, nodecontainer.ErrNodeContainerNotFound)
}

func (s *S) TestEnsureContainersStartedWithoutRelaunch(c *check.C) {
	config.Set("docker:bs:image", "myregistry/tsuru/bs")
	_, err := nodecontainer.InitializeBS(context.TODO(), s.authScheme, "tsr")
	c.Assert(err, check.IsNil)
	var reqs []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		reqs = append(reqs, r)
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	p, err := dockertest.NewFakeDockerProvisioner(server.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	client, err := docker.NewClient(server.URL())
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	err = ensureContainersStarted(p, buf, false, nil)
	c.Assert(err, check.IsNil)
	var paths []string
	for _, r := range reqs {
		paths = append(paths, r.Method+" "+r.URL.Path)
	}
	c.Assert(paths, check.DeepEquals, []string{
		"POST /images/create",
		"POST /containers/create",
		"GET /version",
		"POST /containers/big-sibling/start",
	})
	reqs = nil
	err = ensureContainersStarted(p, buf, false, nil)
	c.Assert(err, check.IsNil)
	paths = nil
	for _, r := range reqs {
		paths = append(paths, r.Method+" "+r.URL.Path)
	}
	c.Assert(paths, check.DeepEquals, []string{
		"POST /images/create",
		"POST /containers/create",
		"GET /version",
		"POST /containers/big-sibling/start",
	})
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: containers[0].ID})
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, nodecontainer.BsDefaultName)
}

func (s *S) TestEnsureContainersStartedGracefullyStop(c *check.C) {
	config.Set("docker:bs:image", "myregistry/tsuru/bs")
	_, err := nodecontainer.InitializeBS(context.TODO(), s.authScheme, "tsr")
	c.Assert(err, check.IsNil)
	var reqs []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		reqs = append(reqs, r)
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	p, err := dockertest.NewFakeDockerProvisioner(server.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	client, err := docker.NewClient(server.URL())
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	err = ensureContainersStarted(p, buf, true, nil)
	c.Assert(err, check.IsNil)
	var paths []string
	for _, r := range reqs {
		paths = append(paths, r.Method+" "+r.URL.Path)
	}
	c.Assert(paths, check.DeepEquals, []string{
		"POST /images/create",
		"POST /containers/create",
		"GET /version",
		"POST /containers/big-sibling/start",
	})
	reqs = nil
	err = ensureContainersStarted(p, buf, true, nil)
	c.Assert(err, check.IsNil)
	paths = nil
	for _, r := range reqs {
		paths = append(paths, r.Method+" "+r.URL.Path)
	}
	c.Assert(paths, check.DeepEquals, []string{
		"POST /images/create",
		"POST /containers/create",
		"POST /containers/big-sibling/stop",
		"DELETE /containers/big-sibling",
		"POST /containers/create",
		"GET /version",
		"POST /containers/big-sibling/start",
	})
	c.Assert(reqs[3].URL.Query().Get("force"), check.Equals, "")
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: containers[0].ID})
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, nodecontainer.BsDefaultName)
}

func (s *S) TestEnsureContainersStartedForceStopOnlyOnFailure(c *check.C) {
	config.Set("docker:bs:image", "myregistry/tsuru/bs")
	_, err := nodecontainer.InitializeBS(context.TODO(), s.authScheme, "tsr")
	c.Assert(err, check.IsNil)
	var reqs []*http.Request
	server, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		reqs = append(reqs, r)
	})
	c.Assert(err, check.IsNil)
	defer server.Stop()
	errCount := 0
	server.CustomHandler("/containers/big-sibling", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, r)
		if r.Method != "DELETE" {
			server.DefaultHandler().ServeHTTP(w, r)
			return
		}
		errCount++
		if errCount > 2 {
			server.DefaultHandler().ServeHTTP(w, r)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	p, err := dockertest.NewFakeDockerProvisioner(server.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	client, err := docker.NewClient(server.URL())
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer(nil)
	err = ensureContainersStarted(p, buf, true, nil)
	c.Assert(err, check.IsNil)
	reqs = nil
	err = ensureContainersStarted(p, buf, true, nil)
	c.Assert(err, check.IsNil)
	var paths []string
	for _, r := range reqs {
		paths = append(paths, r.Method+" "+r.URL.Path)
	}
	c.Assert(paths, check.DeepEquals, []string{
		"POST /images/create",
		"POST /containers/create",
		"POST /containers/big-sibling/stop",
		"DELETE /containers/big-sibling",
		"DELETE /containers/big-sibling",
		"DELETE /containers/big-sibling",
		"POST /containers/create",
		"GET /version",
		"POST /containers/big-sibling/start",
	})
	c.Assert(reqs[3].URL.Query().Get("force"), check.Equals, "")
	c.Assert(reqs[4].URL.Query().Get("force"), check.Equals, "1")
	c.Assert(reqs[5].URL.Query().Get("force"), check.Equals, "1")
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: containers[0].ID})
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, nodecontainer.BsDefaultName)
}

func (s *S) TestEnsureContainersStartedTryCreatingAfterRmFailure(c *check.C) {
	config.Set("docker:bs:image", "myregistry/tsuru/bs")
	_, err := nodecontainer.InitializeBS(context.TODO(), s.authScheme, "tsr")
	c.Assert(err, check.IsNil)
	server, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.CustomHandler("/containers/big-sibling", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("my error"))
			return
		}
		server.DefaultHandler().ServeHTTP(w, r)
	}))
	p, err := dockertest.NewFakeDockerProvisioner(server.URL())
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	buf := safe.NewBuffer(nil)
	err = ensureContainersStarted(p, buf, true, nil)
	c.Assert(err, check.IsNil)
	err = ensureContainersStarted(p, buf, true, nil)
	c.Assert(err, check.ErrorMatches, `(?s).*API error \(500\): my error.*unable to remove old node-container.*container already exists.*unable to create new node-container.*`)
}

func (s *S) TestRecreateBsContainers(c *check.C) {
	_, err := nodecontainer.InitializeBS(context.TODO(), s.authScheme, "tsr")
	c.Assert(err, check.IsNil)
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	var buf safe.Buffer
	err = recreateContainers(p, &buf)
	c.Assert(err, check.IsNil)
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	client, err := nodes[0].Client()
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: containers[0].ID})
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, nodecontainer.BsDefaultName)
	client, err = nodes[1].Client()
	c.Assert(err, check.IsNil)
	containers, err = client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err = client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: containers[0].ID})
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, nodecontainer.BsDefaultName)
	// It runs in parallel, so we check both ordering
	output1 := fmt.Sprintf(`relaunching node container "big-sibling" in the node %s []
relaunching node container "big-sibling" in the node %s []
`, nodes[0].Address, nodes[1].Address)
	output2 := fmt.Sprintf(`relaunching node container "big-sibling" in the node %s []
relaunching node container "big-sibling" in the node %s []
`, nodes[1].Address, nodes[0].Address)
	if got := buf.String(); got != output1 && got != output2 {
		c.Errorf("Wrong output:\n%s", got)
	}
}

func (s *S) TestRecreateBsContainersErrorInSomeContainers(c *check.C) {
	_, err := nodecontainer.InitializeBS(context.TODO(), s.authScheme, "tsr")
	c.Assert(err, check.IsNil)
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	servers := p.Servers()
	servers[0].PrepareFailure("failure-create", "/containers/create")
	defer servers[1].ResetFailure("failure-create")
	var buf safe.Buffer
	err = recreateContainers(p, &buf)
	c.Assert(err, check.ErrorMatches, `(?s)API error \(400\): failure-create.*failed to create container in .* \[.*\].*`)
	sort.Sort(cluster.NodeList(nodes))
	client, err := nodes[0].Client()
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 0)
	client, err = nodes[1].Client()
	c.Assert(err, check.IsNil)
	containers, err = client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: containers[0].ID})
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, nodecontainer.BsDefaultName)
}

func (s *S) TestRemoveNamedContainers(c *check.C) {
	config.Set("docker:bs:image", "myregistry/tsuru/bs")
	_, err := nodecontainer.InitializeBS(context.TODO(), s.authScheme, "tsr")
	c.Assert(err, check.IsNil)
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	for i, n := range nodes {
		n.Metadata["pool"] = fmt.Sprintf("p-%d", i)
		_, err = p.Cluster().UpdateNode(n)
		c.Assert(err, check.IsNil)
	}
	server := p.Servers()[0]
	var paths []string
	server.CustomHandler("/containers/.*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		server.DefaultHandler().ServeHTTP(w, r)
	}))
	server2 := p.Servers()[1]
	var paths2 []string
	server2.CustomHandler("/containers/.*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths2 = append(paths2, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		server2.DefaultHandler().ServeHTTP(w, r)
	}))
	err = ensureContainersStarted(p, io.Discard, true, nil)
	c.Assert(err, check.IsNil)
	paths = nil
	paths2 = nil
	expectedPaths := []string{"POST /containers/big-sibling/stop?t=10", "DELETE /containers/big-sibling?force=1"}
	err = RemoveNamedContainers(p, io.Discard, "big-sibling", "")
	c.Assert(err, check.IsNil)
	c.Assert(paths, check.DeepEquals, expectedPaths)
	c.Assert(paths2, check.DeepEquals, expectedPaths)
	err = ensureContainersStarted(p, io.Discard, true, nil)
	c.Assert(err, check.IsNil)
	paths = nil
	paths2 = nil
	err = RemoveNamedContainers(p, io.Discard, "big-sibling", "p-1")
	c.Assert(err, check.IsNil)
	c.Assert(paths, check.DeepEquals, []string(nil))
	c.Assert(paths2, check.DeepEquals, expectedPaths)
}
