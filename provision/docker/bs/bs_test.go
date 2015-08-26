// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bs

import (
	"runtime"
	"sort"
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision/docker/dockertest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestGetImageFromDatabase(c *check.C) {
	imageName := "tsuru/bsss"
	coll, err := collection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	err = coll.Insert(Config{ID: bsUniqueID, Image: imageName})
	c.Assert(err, check.IsNil)
	defer coll.Remove(bson.M{"image": imageName})
	conf, err := LoadConfig()
	c.Assert(err, check.IsNil)
	image := conf.getImage()
	c.Assert(image, check.Equals, imageName)
}

func (s *S) TestGetImageFromConfig(c *check.C) {
	imageName := "tsuru/bs:v10"
	config.Set("docker:bs:image", imageName)
	conf := Config{}
	image := conf.getImage()
	c.Assert(image, check.Equals, imageName)
}

func (s *S) TestGetImageDefaultValue(c *check.C) {
	config.Unset("docker:bs:image")
	conf := Config{}
	image := conf.getImage()
	c.Assert(image, check.Equals, "tsuru/bs")
}

func (s *S) TestSaveImage(c *check.C) {
	coll, err := collection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	err = SaveImage("tsuru/bs@sha1:afd533420cf")
	c.Assert(err, check.IsNil)
	var configs []Config
	err = coll.Find(nil).All(&configs)
	c.Assert(err, check.IsNil)
	c.Assert(configs, check.HasLen, 1)
	c.Assert(configs[0].Image, check.Equals, "tsuru/bs@sha1:afd533420cf")
	err = SaveImage("tsuru/bs@sha1:afd533420d0")
	c.Assert(err, check.IsNil)
	err = coll.Find(nil).All(&configs)
	c.Assert(err, check.IsNil)
	c.Assert(configs, check.HasLen, 1)
	c.Assert(configs[0].Image, check.Equals, "tsuru/bs@sha1:afd533420d0")
}

func (s *S) TestBsGetToken(c *check.C) {
	conf := Config{}
	token, err := conf.getToken()
	c.Assert(err, check.IsNil)
	c.Assert(token, check.Equals, conf.Token)
	c.Assert(token, check.Not(check.Equals), "")
	token2, err := conf.getToken()
	c.Assert(token2, check.Equals, token)
}

func (s *S) TestBsGetTokenStress(c *check.C) {
	runtime.GOMAXPROCS(runtime.GOMAXPROCS(10))
	var tokens []string
	var mutex sync.Mutex
	var wg sync.WaitGroup
	getToken := func(wg *sync.WaitGroup) {
		defer wg.Done()
		conf := Config{}
		t, err := conf.getToken()
		c.Assert(err, check.IsNil)
		mutex.Lock()
		tokens = append(tokens, t)
		mutex.Unlock()
	}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go getToken(&wg)
	}
	wg.Wait()
	for i := 1; i < len(tokens); i++ {
		c.Assert(tokens[i-1], check.Equals, tokens[i])
	}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	n, err := conn.Tokens().Find(bson.M{"appname": app.InternalAppName}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 1)
	coll, err := collection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	n, err = coll.Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 1)
}

func (s *S) TestRecreateBsContainers(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	err = RecreateContainers(p)
	c.Assert(err, check.IsNil)
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	client, err := nodes[0].Client()
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, "big-sibling")
	client, err = nodes[1].Client()
	c.Assert(err, check.IsNil)
	containers, err = client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err = client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, "big-sibling")
}

func (s *S) TestRecreateBsContainersErrorInSomeContainers(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	servers := p.Servers()
	servers[0].PrepareFailure("failure-create", "/containers/create")
	defer servers[1].ResetFailure("failure-create")
	err = RecreateContainers(p)
	c.Assert(err, check.ErrorMatches, `(?s).*failed to create container in .* \[.*\]: API error \(400\): failure-create.*`)
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
	container, err := client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, "big-sibling")
}

func (s *S) TestClusterHookBeforeCreateContainer(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	hook := ClusterHook{Provisioner: p}
	err = hook.BeforeCreateContainer(nodes[0])
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	client, err := nodes[0].Client()
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, "big-sibling")
	client, err = nodes[1].Client()
	c.Assert(err, check.IsNil)
	containers, err = client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 0)
}

func (s *S) TestClusterHookBeforeCreateContainerIgnoresExistingError(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	err = RecreateContainers(p)
	c.Assert(err, check.IsNil)
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	hook := ClusterHook{Provisioner: p}
	err = hook.BeforeCreateContainer(nodes[0])
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	client, err := nodes[0].Client()
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, "big-sibling")
	client, err = nodes[1].Client()
	c.Assert(err, check.IsNil)
	containers, err = client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err = client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, "big-sibling")
}

func (s *S) TestClusterHookBeforeCreateContainerStartsStopped(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	err = RecreateContainers(p)
	c.Assert(err, check.IsNil)
	nodes, err := p.Cluster().Nodes()
	c.Assert(err, check.IsNil)
	client, err := nodes[0].Client()
	c.Assert(err, check.IsNil)
	err = client.StopContainer("big-sibling", 1)
	c.Assert(err, check.IsNil)
	contData, err := client.InspectContainer("big-sibling")
	c.Assert(err, check.IsNil)
	c.Assert(contData.State.Running, check.Equals, false)
	hook := ClusterHook{Provisioner: p}
	err = hook.BeforeCreateContainer(nodes[0])
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	client, err = nodes[0].Client()
	c.Assert(err, check.IsNil)
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err := client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, "big-sibling")
	c.Assert(container.State.Running, check.Equals, true)
	client, err = nodes[1].Client()
	c.Assert(err, check.IsNil)
	containers, err = client.ListContainers(docker.ListContainersOptions{All: true})
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 1)
	container, err = client.InspectContainer(containers[0].ID)
	c.Assert(err, check.IsNil)
	c.Assert(container.Name, check.Equals, "big-sibling")
	c.Assert(container.State.Running, check.Equals, true)
}
