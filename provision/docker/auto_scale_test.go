// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
)

type AutoScaleSuite struct {
	S                S
	testRepoRollback func()
	appInstance      *provisiontest.FakeApp
	p                *dockerProvisioner
	imageId          string
	node1            *dtesting.DockerServer
	node2            *dtesting.DockerServer
}

func (s *AutoScaleSuite) SetUpSuite(c *check.C) {
	s.S.SetUpSuite(c)
}

func (s *AutoScaleSuite) SetUpTest(c *check.C) {
	s.S.SetUpTest(c)
	plan := app.Plan{Memory: 21000, Name: "default", CpuShare: 10}
	err := plan.Save()
	c.Assert(err, check.IsNil)
	s.testRepoRollback = startTestRepositoryServer()
	s.node1, err = dtesting.NewServer("0.0.0.0:0", nil, nil)
	c.Assert(err, check.IsNil)
	s.node2, err = dtesting.NewServer("0.0.0.0:0", nil, nil)
	c.Assert(err, check.IsNil)
	config.Set("iaas:node-port", urlPort(s.node2.URL()))
	s.p = &dockerProvisioner{}
	err = s.p.Initialize()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = s.p
	s.p.storage = &cluster.MapStorage{}
	re := regexp.MustCompile(`/\[::.*?\]:|/localhost:`)
	url := re.ReplaceAllString(s.node1.URL(), "/127.0.0.1:")
	clusterInstance, err := cluster.New(nil, s.p.storage,
		cluster.Node{Address: url, Metadata: map[string]string{
			"pool":     "pool1",
			"iaas":     "my-scale-iaas",
			"totalMem": "125000",
		}},
	)
	c.Assert(err, check.IsNil)
	s.p.cluster = clusterInstance
	healerConst := newMultiHealerIaaSConstructor([]string{"localhost", "[::1]"}, nil)
	iaas.RegisterIaasProvider("my-scale-iaas", healerConst)
	s.appInstance = provisiontest.NewFakeApp("myapp", "python", 0)
	s.p.Provision(s.appInstance)
	s.imageId, err = appCurrentImageName(s.appInstance.GetName())
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"procfile": "web: python ./myapp",
	}
	err = saveImageCustomData(s.imageId, customData)
	c.Assert(err, check.IsNil)
	appStruct := &app.App{
		Name: s.appInstance.GetName(),
		Pool: "pool1",
		Plan: app.Plan{Memory: 21000},
	}
	opts := provision.AddPoolOptions{Name: "pool1"}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = s.S.storage.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
}

func (s *AutoScaleSuite) TearDownTest(c *check.C) {
	s.S.TearDownTest(c)
	s.node1.Stop()
	s.node2.Stop()
	s.testRepoRollback()
	config.Unset("iaas:node-port")
}

func (s *AutoScaleSuite) TearDownSuite(c *check.C) {
	s.S.TearDownSuite(c)
}

var _ = check.Suite(&AutoScaleSuite{})

func newHealerIaaSConstructor(addr string, err error) func(string) iaas.IaaS {
	return func(name string) iaas.IaaS {
		return &TestHealerIaaS{addr: addr, err: err}
	}
}

func newMultiHealerIaaSConstructor(addrs []string, err error) func(string) iaas.IaaS {
	return func(name string) iaas.IaaS {
		return &TestHealerIaaS{addrs: addrs, err: err}
	}
}

func newHealerIaaSConstructorWithInst(addr string) (func(string) iaas.IaaS, *TestHealerIaaS) {
	inst := &TestHealerIaaS{addr: addr}
	return func(name string) iaas.IaaS {
		return inst
	}, inst
}

func (s *AutoScaleSuite) TestAutoScaleConfigRun(c *check.C) {
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       s.p,
		groupByMetadata:   "pool",
		maxContainerCount: 2,
	}
	wg1 := sync.WaitGroup{}
	wg1.Add(1)
	go func() {
		defer wg1.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg1.Wait()
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	port, _ := config.GetInt("iaas:node-port")
	c.Assert(evts[0].Nodes, check.HasLen, 1)
	c.Assert(evts[0].Nodes[0].Address, check.Equals, fmt.Sprintf("http://localhost:%d", port))
	c.Assert(evts[0].Nodes[0].Metadata["pool"], check.Equals, "pool1")
	logParts := strings.Split(evts[0].Log, "\n")
	c.Assert(logParts, check.HasLen, 15)
	c.Assert(logParts[0], check.Matches, `\[node autoscale\].*running scaler.*pool1.*`)
	c.Assert(logParts[2], check.Matches, `\[node autoscale\].*new machine created.*`)
	c.Assert(logParts[5], check.Matches, `.*Rebalancing 4 units.*`)
	// Also should have rebalanced
	containers1, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 2)
	c.Assert(containers2, check.HasLen, 2)
	// Should do nothing if calling on already scaled
	wg2 := sync.WaitGroup{}
	wg2.Add(1)
	go func() {
		defer wg2.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg2.Wait()
	nodes, err = s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	evts, err = listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	containers1Again, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2Again, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.DeepEquals, containers1Again)
	c.Assert(containers2, check.DeepEquals, containers2Again)
	locked, err := app.AcquireApplicationLock(s.appInstance.GetName(), "x", "y")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunNoRebalance(c *check.C) {
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		preventRebalance:  true,
		done:              make(chan bool),
		provisioner:       s.p,
		groupByMetadata:   "pool",
		maxContainerCount: 2,
	}
	wg1 := sync.WaitGroup{}
	wg1.Add(1)
	go func() {
		defer wg1.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg1.Wait()
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	containers1, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 4)
	c.Assert(containers2, check.HasLen, 0)
	wg2 := sync.WaitGroup{}
	wg2.Add(1)
	go func() {
		defer wg2.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg2.Wait()
	nodes, err = s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	evts, err = listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	containers1Again, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2Again, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.DeepEquals, containers1Again)
	c.Assert(containers2, check.DeepEquals, containers2Again)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunOnce(c *check.C) {
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       s.p,
		groupByMetadata:   "pool",
		maxContainerCount: 2,
	}
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	containers1, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 2)
	c.Assert(containers2, check.HasLen, 2)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunOnceMultipleNodes(c *check.C) {
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 6}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       s.p,
		groupByMetadata:   "pool",
		maxContainerCount: 2,
	}
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.cluster.UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 3)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	c.Assert(nodes[1].Address, check.Not(check.Equals), nodes[2].Address)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	c.Assert(evts[0].Nodes, check.HasLen, 2)
	containers1, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	containers3, err := s.p.listContainersByHost(urlToHost(nodes[2].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 2)
	c.Assert(containers2, check.HasLen, 2)
	c.Assert(containers3, check.HasLen, 2)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunOnceMultipleNodesPartialError(c *check.C) {
	var count int32
	s.node2.CustomHandler("/_ping", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&count, 1) > 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		s.node2.DefaultHandler().ServeHTTP(w, r)
	}))
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 6}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       s.p,
		groupByMetadata:   "pool",
		maxContainerCount: 2,
	}
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	c.Assert(evts[0].Nodes, check.HasLen, 1)
	// parts := strings.Split(evts[0].Log, "\n")
	c.Assert(evts[0].Log, check.Matches, `(?s).*\[node autoscale\] not all required nodes were created: API error.*`)
	containers1, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 3)
	c.Assert(containers2, check.HasLen, 3)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunRebalanceOnly(c *check.C) {
	port, _ := config.GetInt("iaas:node-port")
	otherUrl := fmt.Sprintf("http://localhost:%d", port)
	node := cluster.Node{Address: otherUrl, Metadata: map[string]string{
		"pool": "pool1",
		"iaas": "my-scale-iaas",
	}}
	err := s.p.cluster.Register(node)
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
		toHost:      "127.0.0.1",
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       s.p,
		groupByMetadata:   "pool",
		maxContainerCount: 2,
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg.Wait()
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "rebalance")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	containers1, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 2)
	c.Assert(containers2, check.HasLen, 2)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunNoGroup(c *check.C) {
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       s.p,
		maxContainerCount: 2,
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg.Wait()
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "")
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	containers1, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 2)
	c.Assert(containers2, check.HasLen, 2)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunNoMatch(c *check.C) {
	originalNodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	s.p.cluster, err = cluster.New(nil, &cluster.MapStorage{}, cluster.Node{
		Address: originalNodes[0].Address,
		Metadata: map[string]string{
			"iaas": "my-scale-iaas",
		},
	})
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       s.p,
		maxContainerCount: 2,
		groupByMetadata:   "pool",
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg.Wait()
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address, check.Equals, originalNodes[0].Address)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
	s.p.cluster, err = cluster.New(nil, &cluster.MapStorage{},
		cluster.Node{Address: nodes[0].Address, Metadata: map[string]string{
			"iaas": "my-scale-iaas",
			"pool": "pool1",
		}},
	)
	c.Assert(err, check.IsNil)
	a.matadataFilter = "pool2"
	wg1 := sync.WaitGroup{}
	wg1.Add(1)
	go func() {
		defer wg1.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg1.Wait()
	nodes, err = s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	evts, err = listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
	a.matadataFilter = "pool1"
	wg2 := sync.WaitGroup{}
	wg2.Add(1)
	go func() {
		defer wg2.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg2.Wait()
	nodes, err = s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	evts, err = listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunStress(c *check.C) {
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	wg := sync.WaitGroup{}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			a := autoScaleConfig{
				done:              make(chan bool),
				provisioner:       s.p,
				groupByMetadata:   "pool",
				maxContainerCount: 2,
			}
			defer wg.Done()
			wgIn := sync.WaitGroup{}
			wgIn.Add(1)
			go func() {
				defer wgIn.Done()
				a.stop()
			}()
			err := a.run()
			c.Assert(err, check.IsNil)
			wgIn.Wait()
		}()
	}
	wg.Wait()
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	containers1, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 2)
	c.Assert(containers2, check.HasLen, 2)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunMemoryBased(c *check.C) {
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:                make(chan bool),
		provisioner:         s.p,
		groupByMetadata:     "pool",
		totalMemoryMetadata: "totalMem",
		maxMemoryRatio:      0.8,
	}
	wg1 := sync.WaitGroup{}
	wg1.Add(1)
	go func() {
		defer wg1.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg1.Wait()
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	// Also should have rebalanced
	containers1, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 2)
	c.Assert(containers2, check.HasLen, 2)
	// Should do nothing if calling on already scaled
	wg2 := sync.WaitGroup{}
	wg2.Add(1)
	go func() {
		defer wg2.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg2.Wait()
	nodes, err = s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	evts, err = listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	containers1Again, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2Again, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.DeepEquals, containers1Again)
	c.Assert(containers2, check.DeepEquals, containers2Again)
	locked, err := app.AcquireApplicationLock(s.appInstance.GetName(), "x", "y")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunMemoryBasedMultipleNodes(c *check.C) {
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 9}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:                make(chan bool),
		provisioner:         s.p,
		groupByMetadata:     "pool",
		totalMemoryMetadata: "totalMem",
		maxMemoryRatio:      0.8,
	}
	wg1 := sync.WaitGroup{}
	wg1.Add(1)
	go func() {
		defer wg1.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg1.Wait()
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 3)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	c.Assert(nodes[1].Address, check.Not(check.Equals), nodes[2].Address)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	c.Assert(evts[0].Nodes, check.HasLen, 2)
	// Also should have rebalanced
	containers1, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	containers3, err := s.p.listContainersByHost(urlToHost(nodes[2].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 3)
	c.Assert(containers2, check.HasLen, 3)
	c.Assert(containers3, check.HasLen, 3)
	// Should do nothing if calling on already scaled
	wg2 := sync.WaitGroup{}
	wg2.Add(1)
	go func() {
		defer wg2.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg2.Wait()
	nodes, err = s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 3)
	evts, err = listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	containers1Again, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2Again, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	containers3Again, err := s.p.listContainersByHost(urlToHost(nodes[2].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.DeepEquals, containers1Again)
	c.Assert(containers2, check.DeepEquals, containers2Again)
	c.Assert(containers3, check.DeepEquals, containers3Again)
	locked, err := app.AcquireApplicationLock(s.appInstance.GetName(), "x", "y")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunPriorityToCountBased(c *check.C) {
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:                make(chan bool),
		provisioner:         s.p,
		groupByMetadata:     "pool",
		maxContainerCount:   2,
		totalMemoryMetadata: "totalMem",
		maxMemoryRatio:      0.8,
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg.Wait()
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	containers1, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 2)
	c.Assert(containers2, check.HasLen, 2)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunMemoryBasedPlanTooBig(c *check.C) {
	err := app.PlanRemove("default")
	c.Assert(err, check.IsNil)
	plan := app.Plan{Memory: 126000, Name: "default", CpuShare: 10}
	err = plan.Save()
	c.Assert(err, check.IsNil)
	originalNodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:                make(chan bool),
		provisioner:         s.p,
		groupByMetadata:     "pool",
		totalMemoryMetadata: "totalMem",
		maxMemoryRatio:      0.8,
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.ErrorMatches, `\[node autoscale\] error scaling group pool1: aborting, impossible to fit max plan memory of 126000 bytes, node max available memory is 100000`)
	wg.Wait()
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address, check.Equals, originalNodes[0].Address)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunScaleDown(c *check.C) {
	port, _ := config.GetInt("iaas:node-port")
	otherUrl := fmt.Sprintf("http://localhost:%d/", port)
	node := cluster.Node{Address: otherUrl, Metadata: map[string]string{
		"pool":     "pool1",
		"iaas":     "my-scale-iaas",
		"totalMem": "125000",
	}}
	err := s.p.cluster.Register(node)
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
		toHost:      "127.0.0.1",
	})
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
		toHost:      "localhost",
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       s.p,
		groupByMetadata:   "pool",
		maxContainerCount: 4,
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg.Wait()
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "remove")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	containers, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunScaleDownMultipleNodes(c *check.C) {
	port, _ := config.GetInt("iaas:node-port")
	node1 := cluster.Node{Address: fmt.Sprintf("http://localhost:%d/", port), Metadata: map[string]string{
		"pool":     "pool1",
		"iaas":     "my-scale-iaas",
		"totalMem": "125000",
	}}
	err := s.p.cluster.Register(node1)
	node2 := cluster.Node{Address: fmt.Sprintf("http://[::1]:%d/", port), Metadata: map[string]string{
		"pool":     "pool1",
		"iaas":     "my-scale-iaas",
		"totalMem": "125000",
	}}
	err = s.p.cluster.Register(node2)
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
		toHost:      "127.0.0.1",
	})
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
		toHost:      "localhost",
	})
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
		toHost:      "::1",
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       s.p,
		groupByMetadata:   "pool",
		maxContainerCount: 5,
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg.Wait()
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "remove")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	c.Assert(evts[0].Nodes, check.HasLen, 2)
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	containers, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 3)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunScaleDownMemoryScaler(c *check.C) {
	port, _ := config.GetInt("iaas:node-port")
	otherUrl := fmt.Sprintf("http://localhost:%d/", port)
	node := cluster.Node{Address: otherUrl, Metadata: map[string]string{
		"pool":     "pool1",
		"iaas":     "my-scale-iaas",
		"totalMem": "125000",
	}}
	err := s.p.cluster.Register(node)
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
		toHost:      "127.0.0.1",
	})
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
		toHost:      "localhost",
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:                make(chan bool),
		provisioner:         s.p,
		groupByMetadata:     "pool",
		totalMemoryMetadata: "totalMem",
		maxMemoryRatio:      0.8,
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg.Wait()
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "remove")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	containers, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunScaleDownMemoryScalerMultipleNodes(c *check.C) {
	port, _ := config.GetInt("iaas:node-port")
	node1 := cluster.Node{Address: fmt.Sprintf("http://localhost:%d/", port), Metadata: map[string]string{
		"pool":     "pool1",
		"iaas":     "my-scale-iaas",
		"totalMem": "125000",
	}}
	err := s.p.cluster.Register(node1)
	node2 := cluster.Node{Address: fmt.Sprintf("http://[::1]:%d/", port), Metadata: map[string]string{
		"pool":     "pool1",
		"iaas":     "my-scale-iaas",
		"totalMem": "125000",
	}}
	err = s.p.cluster.Register(node2)
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
		toHost:      "127.0.0.1",
	})
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
		toHost:      "localhost",
	})
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
		toHost:      "::1",
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:                make(chan bool),
		provisioner:         s.p,
		groupByMetadata:     "pool",
		totalMemoryMetadata: "totalMem",
		maxMemoryRatio:      0.8,
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg.Wait()
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "remove")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	c.Assert(evts[0].Nodes, check.HasLen, 2)
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	containers, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 3)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunScaleDownRespectsMinNodes(c *check.C) {
	oldNodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	port, _ := config.GetInt("iaas:node-port")
	otherUrl := fmt.Sprintf("http://localhost:%d/", port)
	s.p.storage = &cluster.MapStorage{}
	s.p.cluster, err = cluster.New(nil, s.p.storage,
		cluster.Node{Address: oldNodes[0].Address, Metadata: map[string]string{
			"pool":    "pool1",
			"iaas":    "my-scale-iaas",
			"network": "net1",
		}},
		cluster.Node{Address: otherUrl, Metadata: map[string]string{
			"pool":    "pool1",
			"iaas":    "my-scale-iaas",
			"network": "net2",
		}},
	)
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
		toHost:      "127.0.0.1",
	})
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 1}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
		toHost:      "localhost",
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       s.p,
		groupByMetadata:   "pool",
		maxContainerCount: 4,
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.IsNil)
	wg.Wait()
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunLockedApp(c *check.C) {
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	locked, err := app.AcquireApplicationLock(s.appInstance.GetName(), "tsurud", "something")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       s.p,
		groupByMetadata:   "pool",
		maxContainerCount: 2,
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.ErrorMatches, `.*unable to lock app myapp, aborting.*`)
	wg.Wait()
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunMemoryBasedLockedApp(c *check.C) {
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	locked, err := app.AcquireApplicationLock(s.appInstance.GetName(), "tsurud", "something")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
	a := autoScaleConfig{
		done:                make(chan bool),
		provisioner:         s.p,
		groupByMetadata:     "pool",
		totalMemoryMetadata: "totalMem",
		maxMemoryRatio:      0.8,
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.stop()
	}()
	err = a.run()
	c.Assert(err, check.ErrorMatches, `.*unable to lock app myapp, aborting.*`)
	wg.Wait()
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
}

func (s *S) TestAutoScaleConfigRunParamsError(c *check.C) {
	a := autoScaleConfig{
		provisioner:       s.p,
		groupByMetadata:   "pool",
		maxContainerCount: 0,
	}
	err := a.run()
	c.Assert(err, check.ErrorMatches, `\[node autoscale\] aborting node auto scale, either memory information or max container count must be informed in config`)
	a = autoScaleConfig{
		provisioner:       s.p,
		groupByMetadata:   "pool",
		maxContainerCount: 10,
		scaleDownRatio:    0.9,
	}
	err = a.run()
	c.Assert(err, check.ErrorMatches, `\[node autoscale\] scale down ratio needs to be greater than 1.0, got .+`)
}

func (s *S) TestAutoScaleConfigRunDefaultValues(c *check.C) {
	a := autoScaleConfig{
		done:              make(chan bool),
		provisioner:       s.p,
		maxContainerCount: 10,
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.stop()
	}()
	err := a.run()
	c.Assert(err, check.IsNil)
	wg.Wait()
	c.Assert(a.runInterval, check.Equals, 1*time.Hour)
	c.Assert(a.waitTimeNewMachine, check.Equals, 5*time.Minute)
	c.Assert(a.scaleDownRatio > 1.332 && a.scaleDownRatio < 1.334, check.Equals, true)
}

func (s *S) TestAutoScaleConfigRunConfigValues(c *check.C) {
	a := autoScaleConfig{
		done:               make(chan bool),
		provisioner:        s.p,
		maxContainerCount:  10,
		runInterval:        10 * time.Minute,
		waitTimeNewMachine: 7 * time.Minute,
		scaleDownRatio:     1.5,
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.stop()
	}()
	err := a.run()
	c.Assert(err, check.IsNil)
	wg.Wait()
	c.Assert(a.runInterval, check.Equals, 10*time.Minute)
	c.Assert(a.waitTimeNewMachine, check.Equals, 7*time.Minute)
	c.Assert(a.scaleDownRatio > 1.49 && a.scaleDownRatio < 1.51, check.Equals, true)
}

func (s *S) TestAutoScaleCanRemoveNode(c *check.C) {
	nodes := []*cluster.Node{
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
		{Address: "", Metadata: map[string]string{
			"pool": "pool2",
			"zone": "zone2",
		}},
	}
	ok, err := canRemoveNode(nodes[0], nodes)
	c.Assert(err, check.IsNil)
	c.Assert(ok, check.Equals, true)
	ok, err = canRemoveNode(nodes[1], nodes)
	c.Assert(err, check.IsNil)
	c.Assert(ok, check.Equals, true)
	ok, err = canRemoveNode(nodes[2], nodes)
	c.Assert(err, check.IsNil)
	c.Assert(ok, check.Equals, false)
	nodes = []*cluster.Node{
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
	}
	ok, err = canRemoveNode(nodes[0], nodes)
	c.Assert(err, check.IsNil)
	c.Assert(ok, check.Equals, true)
	ok, err = canRemoveNode(nodes[1], nodes)
	c.Assert(err, check.IsNil)
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestSplitMetadata(c *check.C) {
	var err error
	exclusive, common, err := splitMetadata([]map[string]string{
		{"1": "a", "2": "z1", "3": "n1"},
		{"1": "a", "2": "z2", "3": "n2"},
		{"1": "a", "2": "z3", "3": "n3"},
		{"1": "a", "2": "z3", "3": "n3"},
	})
	c.Assert(err, check.IsNil)
	c.Assert(exclusive, check.DeepEquals, metaWithFrequencyList{
		{metadata: map[string]string{"2": "z1", "3": "n1"}, freq: 1},
		{metadata: map[string]string{"2": "z2", "3": "n2"}, freq: 1},
		{metadata: map[string]string{"2": "z3", "3": "n3"}, freq: 2},
	})
	c.Assert(common, check.DeepEquals, map[string]string{
		"1": "a",
	})
	exclusive, common, err = splitMetadata([]map[string]string{
		{"1": "a", "2": "z1", "3": "n1", "4": "b"},
		{"1": "a", "2": "z2", "3": "n2", "4": "b"},
	})
	c.Assert(err, check.IsNil)
	c.Assert(exclusive, check.DeepEquals, metaWithFrequencyList{
		{metadata: map[string]string{"2": "z1", "3": "n1"}, freq: 1},
		{metadata: map[string]string{"2": "z2", "3": "n2"}, freq: 1},
	})
	c.Assert(common, check.DeepEquals, map[string]string{
		"1": "a",
		"4": "b",
	})
	exclusive, common, err = splitMetadata([]map[string]string{
		{"1": "a", "2": "b"},
		{"1": "a", "2": "b"},
	})
	c.Assert(err, check.IsNil)
	c.Assert(exclusive, check.IsNil)
	c.Assert(common, check.DeepEquals, map[string]string{
		"1": "a",
		"2": "b",
	})
	exclusive, common, err = splitMetadata([]map[string]string{})
	c.Assert(err, check.IsNil)
	c.Assert(exclusive, check.IsNil)
	c.Assert(common, check.DeepEquals, map[string]string{})
	_, _, err = splitMetadata([]map[string]string{
		{"1": "a", "2": "z1", "3": "n1", "4": "b"},
		{"1": "a", "2": "z2", "3": "n2", "4": "b"},
		{"1": "a", "2": "z3", "3": "n3", "4": "c"},
	})
	c.Assert(err, check.ErrorMatches, "unbalanced metadata for node group:.*")
	_, _, err = splitMetadata([]map[string]string{
		{"1": "a", "2": "z1", "3": "n1", "4": "b"},
		{"1": "a", "2": "z2", "3": "n2", "4": "b"},
		{"1": "a", "2": "z3", "3": "n1", "4": "b"},
	})
	c.Assert(err, check.ErrorMatches, "unbalanced metadata for node group:.*")
}

func (s *S) TestChooseMetadataFromNodes(c *check.C) {
	nodes := []*cluster.Node{
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
	}
	metadata, err := chooseMetadataFromNodes(nodes)
	c.Assert(err, check.IsNil)
	c.Assert(metadata, check.DeepEquals, map[string]string{
		"pool": "pool1",
		"zone": "zone1",
	})
	nodes = []*cluster.Node{
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone2",
		}},
	}
	metadata, err = chooseMetadataFromNodes(nodes)
	c.Assert(err, check.IsNil)
	c.Assert(metadata, check.DeepEquals, map[string]string{
		"pool": "pool1",
		"zone": "zone2",
	})
	nodes = []*cluster.Node{
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
		{Address: "", Metadata: map[string]string{
			"pool": "pool2",
			"zone": "zone2",
		}},
		{Address: "", Metadata: map[string]string{
			"pool": "pool2",
			"zone": "zone2",
		}},
	}
	metadata, err = chooseMetadataFromNodes(nodes)
	c.Assert(err, check.IsNil)
	c.Assert(metadata, check.DeepEquals, map[string]string{
		"pool": "pool1",
		"zone": "zone1",
	})
	nodes = []*cluster.Node{
		{Address: "", Metadata: map[string]string{
			"pool": "pool1",
			"zone": "zone1",
		}},
		{Address: "", Metadata: map[string]string{
			"pool": "pool2",
			"zone": "zone2",
		}},
		{Address: "", Metadata: map[string]string{
			"pool": "pool2",
			"zone": "zone3",
		}},
	}
	_, err = chooseMetadataFromNodes(nodes)
	c.Assert(err, check.ErrorMatches, "unbalanced metadata for node group:.*")
}
