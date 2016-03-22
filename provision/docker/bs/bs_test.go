// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bs

import (
	"fmt"
	"runtime"
	"sort"
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision/docker/dockertest"
	"github.com/tsuru/tsuru/safe"
	"github.com/tsuru/tsuru/scopedconfig"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestLoadConfigPool(c *check.C) {
	conf := scopedconfig.FindNScopedConfig(bsConfigCollection)
	err := conf.SaveMerge("", BSConfigEntry{Envs: map[string]string{"USER": "root"}})
	c.Assert(err, check.IsNil)
	err = conf.SaveMerge("pool1", BSConfigEntry{Envs: map[string]string{"USER": "nonroot"}})
	c.Assert(err, check.IsNil)
	err = conf.SaveMerge("pool2", BSConfigEntry{Envs: map[string]string{"USER": "superroot"}})
	c.Assert(err, check.IsNil)
	err = conf.SaveMerge("pool3", BSConfigEntry{Envs: map[string]string{"USER": "watroot"}})
	c.Assert(err, check.IsNil)
	err = conf.SaveMerge("pool4", BSConfigEntry{Envs: map[string]string{"USER": "kindaroot"}})
	c.Assert(err, check.IsNil)
	conf, err = LoadConfig()
	c.Assert(err, check.IsNil)
	allVal := map[string]BSConfigEntry{}
	err = conf.LoadAll(allVal)
	c.Assert(err, check.IsNil)
	expectedConfig := map[string]BSConfigEntry{
		"":      {Envs: map[string]string{"USER": "root"}},
		"pool1": {Envs: map[string]string{"USER": "nonroot"}},
		"pool2": {Envs: map[string]string{"USER": "superroot"}},
		"pool3": {Envs: map[string]string{"USER": "watroot"}},
		"pool4": {Envs: map[string]string{"USER": "kindaroot"}},
	}
	c.Assert(allVal, check.DeepEquals, expectedConfig)
}

func (s *S) TestGetImageFromDatabase(c *check.C) {
	imageName := "tsuru/bsss"
	err := SaveImage(imageName)
	c.Assert(err, check.IsNil)
	conf, err := LoadConfig()
	c.Assert(err, check.IsNil)
	var entry BSConfigEntry
	err = conf.LoadBase(&entry)
	c.Assert(err, check.IsNil)
	image := getImage(entry.Image)
	c.Assert(image, check.Equals, imageName)
}

func (s *S) TestGetImageFromConfig(c *check.C) {
	imageName := "tsuru/bs:v10"
	config.Set("docker:bs:image", imageName)
	image := getImage("")
	c.Assert(image, check.Equals, imageName)
}

func (s *S) TestGetImageDefaultValue(c *check.C) {
	config.Unset("docker:bs:image")
	image := getImage("")
	c.Assert(image, check.Equals, "tsuru/bs:v1")
}

func (s *S) TestSaveImage(c *check.C) {
	err := SaveImage("tsuru/bs@sha1:afd533420cf")
	c.Assert(err, check.IsNil)
	conf, err := LoadConfig()
	c.Assert(err, check.IsNil)
	var entry BSConfigEntry
	err = conf.LoadBase(&entry)
	c.Assert(err, check.IsNil)
	c.Assert(entry.Image, check.Equals, "tsuru/bs@sha1:afd533420cf")
	err = SaveImage("tsuru/bs@sha1:afd533420d0")
	c.Assert(err, check.IsNil)
	err = conf.LoadBase(&entry)
	c.Assert(err, check.IsNil)
	c.Assert(entry.Image, check.Equals, "tsuru/bs@sha1:afd533420d0")
}

func (s *S) TestBsGetToken(c *check.C) {
	conf, err := LoadConfig()
	c.Assert(err, check.IsNil)
	var entry BSConfigEntry
	err = conf.LoadBase(&entry)
	c.Assert(err, check.IsNil)
	token, err := getToken(conf, entry.Token)
	c.Assert(err, check.IsNil)
	c.Assert(token, check.Not(check.Equals), "")
	err = conf.LoadBase(&entry)
	c.Assert(err, check.IsNil)
	c.Assert(entry.Token, check.Equals, token)
	token2, err := getToken(conf, entry.Token)
	c.Assert(token2, check.Equals, token)
}

func (s *S) TestBsGetTokenStress(c *check.C) {
	runtime.GOMAXPROCS(runtime.GOMAXPROCS(10))
	var tokens []string
	var mutex sync.Mutex
	var wg sync.WaitGroup
	getTokenRoutine := func(wg *sync.WaitGroup) {
		defer wg.Done()
		conf, err := LoadConfig()
		c.Assert(err, check.IsNil)
		var entry BSConfigEntry
		err = conf.LoadBase(&entry)
		c.Assert(err, check.IsNil)
		t, err := getToken(conf, entry.Token)
		c.Assert(err, check.IsNil)
		mutex.Lock()
		tokens = append(tokens, t)
		mutex.Unlock()
	}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go getTokenRoutine(&wg)
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
}

func (s *S) TestRecreateBsContainers(c *check.C) {
	p, err := dockertest.StartMultipleServersCluster()
	c.Assert(err, check.IsNil)
	defer p.Destroy()
	var buf safe.Buffer
	err = RecreateContainers(p, &buf)
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
	// It runs in parallel, so we check both ordering
	output1 := fmt.Sprintf(`relaunching bs container in the node %s []
relaunching bs container in the node %s []
`, nodes[0].Address, nodes[1].Address)
	output2 := fmt.Sprintf(`relaunching bs container in the node %s []
relaunching bs container in the node %s []
`, nodes[1].Address, nodes[0].Address)
	if got := buf.String(); got != output1 && got != output2 {
		c.Errorf("Wrong output:\n%s", got)
	}
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
	var buf safe.Buffer
	err = RecreateContainers(p, &buf)
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
	var buf safe.Buffer
	err = RecreateContainers(p, &buf)
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
