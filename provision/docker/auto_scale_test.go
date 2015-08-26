// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	dtesting "github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/docker/dockertest"
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
	node3            *dtesting.DockerServer
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
	s.node3, err = dtesting.NewServer("0.0.0.0:0", nil, nil)
	c.Assert(err, check.IsNil)
	s.p = &dockerProvisioner{}
	err = s.p.Initialize()
	c.Assert(err, check.IsNil)
	mainDockerProvisioner = s.p
	s.p.storage = &cluster.MapStorage{}
	re := regexp.MustCompile(`/\[::.*?\]:|/localhost:`)
	url := re.ReplaceAllString(s.node1.URL(), "/127.0.0.1:")
	sched := &segregatedScheduler{provisioner: s.p}
	s.p.scheduler = sched
	clusterInstance, err := cluster.New(sched, s.p.storage,
		cluster.Node{Address: url, Metadata: map[string]string{
			"pool":     "pool1",
			"iaas":     "my-scale-iaas",
			"totalMem": "125000",
		}},
	)
	c.Assert(err, check.IsNil)
	s.p.cluster = clusterInstance
	healerConst := dockertest.NewMultiHealerIaaSConstructor(
		[]string{"localhost", "[::1]"},
		[]int{dockertest.URLPort(s.node2.URL()), dockertest.URLPort(s.node3.URL())},
		nil,
	)
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
	config.Set("docker:auto-scale:max-container-count", 2)
}

func (s *AutoScaleSuite) TearDownTest(c *check.C) {
	s.S.TearDownTest(c)
	s.node1.Stop()
	s.node2.Stop()
	s.node3.Stop()
	s.testRepoRollback()
	config.Unset("docker:auto-scale:max-container-count")
}

func (s *AutoScaleSuite) TearDownSuite(c *check.C) {
	s.S.TearDownSuite(c)
}

var _ = check.Suite(&AutoScaleSuite{})

func (s *AutoScaleSuite) TestAutoScaleConfigRun(c *check.C) {
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	a.runOnce()
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
	c.Assert(evts[0].Reason, check.Equals, "number of free slots is -2, adding 1 nodes")
	c.Assert(evts[0].Nodes, check.HasLen, 1)
	c.Assert(evts[0].Nodes[0].Address, check.Equals, fmt.Sprintf("http://localhost:%d", dockertest.URLPort(s.node2.URL())))
	c.Assert(evts[0].Nodes[0].Metadata["pool"], check.Equals, "pool1")
	logParts := strings.Split(evts[0].Log, "\n")
	c.Assert(logParts, check.HasLen, 15)
	c.Assert(logParts[0], check.Matches, `.*running scaler.*countScaler.*pool1.*`)
	c.Assert(logParts[2], check.Matches, `.*new machine created.*`)
	c.Assert(logParts[5], check.Matches, `.*Rebalancing 4 units.*`)
	// Also should have rebalanced
	containers1, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 2)
	c.Assert(containers2, check.HasLen, 2)
	// Should do nothing if calling on already scaled
	a.runOnce()
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
	config.Set("docker:auto-scale:prevent-rebalance", true)
	defer config.Unset("docker:auto-scale:prevent-rebalance")
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	a.runOnce()
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
	a.runOnce()
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
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
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

func (s *AutoScaleSuite) TestAutoScaleConfigRunOnceNoContainers(c *check.C) {
	a := autoScaleConfig{
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	err := a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunOnceNoContainersMultipleNodes(c *check.C) {
	otherUrl := fmt.Sprintf("http://localhost:%d", dockertest.URLPort(s.node2.URL()))
	node := cluster.Node{Address: otherUrl, Metadata: map[string]string{
		"pool":     "pool1",
		"iaas":     "my-scale-iaas",
		"totalMem": "125000",
	}}
	err := s.p.cluster.Register(node)
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Nodes, check.HasLen, 1)
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
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
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
	c.Assert(evts[0].Reason, check.Equals, "number of free slots is -4, adding 2 nodes")
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

func (s *AutoScaleSuite) TestAutoScaleConfigRunOnceAddsAtLeastOne(c *check.C) {
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 3}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
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
	c.Assert(evts[0].Reason, check.Equals, "number of free slots is -1, adding 1 nodes")
	c.Assert(evts[0].Nodes, check.HasLen, 1)
	containers1, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(len(containers1) == 2 || len(containers2) == 2, check.Equals, true)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunOnceMultipleNodesPartialError(c *check.C) {
	s.node3.CustomHandler("/_ping", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}))
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 6}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	machines, err := iaas.ListMachines()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.HasLen, 0)
	a := autoScaleConfig{
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.cluster.UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	machines, err = iaas.ListMachines()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.HasLen, 1)
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
	c.Assert(evts[0].Log, check.Matches, `(?s).*not all required nodes were created: error running bs task: API error.*`)
	containers1, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 3)
	c.Assert(containers2, check.HasLen, 3)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunRebalanceOnly(c *check.C) {
	otherUrl := fmt.Sprintf("http://localhost:%d", dockertest.URLPort(s.node2.URL()))
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
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	a.runOnce()
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
		done:        make(chan bool),
		provisioner: s.p,
	}
	a.runOnce()
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
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	a.runOnce()
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
	config.Set("docker:auto-scale:metadata-filter", "pool2")
	defer config.Unset("docker:auto-scale:metadata-filter")
	a.runOnce()
	nodes, err = s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	evts, err = listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
	config.Set("docker:auto-scale:metadata-filter", "pool1")
	a.runOnce()
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
				done:            make(chan bool),
				provisioner:     s.p,
				GroupByMetadata: "pool",
			}
			defer wg.Done()
			err := a.runOnce()
			c.Assert(err, check.IsNil)
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
	config.Set("docker:scheduler:max-used-memory", 0.8)
	config.Unset("docker:auto-scale:max-container-count")
	defer config.Unset("docker:scheduler:max-used-memory")
	config.Set("docker:scheduler:total-memory-metadata", "totalMem")
	defer config.Unset("docker:scheduler:total-memory-metadata")
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	a.runOnce()
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
	c.Assert(evts[0].Reason, check.Equals, "can't add 21000 bytes to an existing node, adding 1 nodes")
	// Also should have rebalanced
	containers1, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	containers2, err := s.p.listContainersByHost(urlToHost(nodes[1].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers1, check.HasLen, 2)
	c.Assert(containers2, check.HasLen, 2)
	// Should do nothing if calling on already scaled
	a.runOnce()
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
	config.Set("docker:scheduler:max-used-memory", 0.8)
	config.Unset("docker:auto-scale:max-container-count")
	defer config.Unset("docker:scheduler:max-used-memory")
	config.Set("docker:scheduler:total-memory-metadata", "totalMem")
	defer config.Unset("docker:scheduler:total-memory-metadata")
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 9}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	a.runOnce()
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 3)
	c.Assert(nodes[0].Address, check.Not(check.Equals), nodes[1].Address)
	c.Assert(nodes[1].Address, check.Not(check.Equals), nodes[2].Address)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	c.Assert(evts[0].Nodes, check.HasLen, 2)
	c.Assert(evts[0].Reason, check.Equals, "can't add 21000 bytes to an existing node, adding 2 nodes")
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
	a.runOnce()
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

func (s *AutoScaleSuite) TestAutoScaleConfigRunOnceMemoryBasedNoContainersMultipleNodes(c *check.C) {
	config.Set("docker:scheduler:max-used-memory", 0.8)
	config.Unset("docker:auto-scale:max-container-count")
	defer config.Unset("docker:scheduler:max-used-memory")
	config.Set("docker:scheduler:total-memory-metadata", "totalMem")
	defer config.Unset("docker:scheduler:total-memory-metadata")
	otherUrl := fmt.Sprintf("http://localhost:%d", dockertest.URLPort(s.node2.URL()))
	node := cluster.Node{Address: otherUrl, Metadata: map[string]string{
		"pool":     "pool1",
		"iaas":     "my-scale-iaas",
		"totalMem": "125000",
	}}
	err := s.p.cluster.Register(node)
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].Nodes, check.HasLen, 1)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunPriorityToCountBased(c *check.C) {
	config.Set("docker:scheduler:max-used-memory", 0.8)
	config.Unset("docker:auto-scale:max-container-count")
	defer config.Unset("docker:scheduler:max-used-memory")
	config.Set("docker:scheduler:total-memory-metadata", "totalMem")
	defer config.Unset("docker:scheduler:total-memory-metadata")
	_, err := addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	a.runOnce()
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
	config.Set("docker:scheduler:max-used-memory", 0.8)
	config.Unset("docker:auto-scale:max-container-count")
	defer config.Unset("docker:scheduler:max-used-memory")
	config.Set("docker:scheduler:total-memory-metadata", "totalMem")
	defer config.Unset("docker:scheduler:total-memory-metadata")
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
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	a.runOnce()
	c.Assert(s.S.logBuf, check.Matches, `(?s).*\[node autoscale\] error scaling group pool1: aborting, impossible to fit max plan memory of 126000 bytes, node max available memory is 100000.*`)
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address, check.Equals, originalNodes[0].Address)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunScaleDown(c *check.C) {
	config.Set("docker:auto-scale:max-container-count", 4)
	otherUrl := fmt.Sprintf("http://localhost:%d/", dockertest.URLPort(s.node2.URL()))
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
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	a.runOnce()
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "remove")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	c.Assert(evts[0].Reason, check.Equals, "number of free slots is 6, removing 1 nodes")
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	containers, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunScaleDownMultipleNodes(c *check.C) {
	config.Set("docker:auto-scale:max-container-count", 5)
	node1 := cluster.Node{Address: fmt.Sprintf("http://localhost:%d/", dockertest.URLPort(s.node2.URL())), Metadata: map[string]string{
		"pool":     "pool1",
		"iaas":     "my-scale-iaas",
		"totalMem": "125000",
	}}
	err := s.p.cluster.Register(node1)
	node2 := cluster.Node{Address: fmt.Sprintf("http://[::1]:%d/", dockertest.URLPort(s.node3.URL())), Metadata: map[string]string{
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
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	a.runOnce()
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
	c.Assert(evts[0].Reason, check.Equals, "number of free slots is 12, removing 2 nodes")
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	containers, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 3)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunScaleDownMemoryScaler(c *check.C) {
	config.Set("docker:scheduler:max-used-memory", 0.8)
	config.Unset("docker:auto-scale:max-container-count")
	defer config.Unset("docker:scheduler:max-used-memory")
	config.Set("docker:scheduler:total-memory-metadata", "totalMem")
	defer config.Unset("docker:scheduler:total-memory-metadata")
	otherUrl := fmt.Sprintf("http://localhost:%d/", dockertest.URLPort(s.node2.URL()))
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
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	a.runOnce()
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	c.Assert(evts[0].StartTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].EndTime.IsZero(), check.Equals, false)
	c.Assert(evts[0].MetadataValue, check.Equals, "pool1")
	c.Assert(evts[0].Action, check.Equals, "remove")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	c.Assert(evts[0].Reason, check.Equals, "containers can be distributed in only 1 nodes, removing 1 nodes")
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	containers, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 2)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunScaleDownMemoryScalerMultipleNodes(c *check.C) {
	config.Set("docker:scheduler:max-used-memory", 0.8)
	config.Unset("docker:auto-scale:max-container-count")
	defer config.Unset("docker:scheduler:max-used-memory")
	config.Set("docker:scheduler:total-memory-metadata", "totalMem")
	defer config.Unset("docker:scheduler:total-memory-metadata")
	node1 := cluster.Node{Address: fmt.Sprintf("http://localhost:%d/", dockertest.URLPort(s.node2.URL())), Metadata: map[string]string{
		"pool":     "pool1",
		"iaas":     "my-scale-iaas",
		"totalMem": "125000",
	}}
	err := s.p.cluster.Register(node1)
	node2 := cluster.Node{Address: fmt.Sprintf("http://[::1]:%d/", dockertest.URLPort(s.node3.URL())), Metadata: map[string]string{
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
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	a.runOnce()
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
	c.Assert(evts[0].Reason, check.Equals, "containers can be distributed in only 1 nodes, removing 2 nodes")
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	containers, err := s.p.listContainersByHost(urlToHost(nodes[0].Address))
	c.Assert(err, check.IsNil)
	c.Assert(containers, check.HasLen, 3)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunScaleDownRespectsMinNodes(c *check.C) {
	config.Set("docker:auto-scale:max-container-count", 4)
	oldNodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	otherUrl := fmt.Sprintf("http://localhost:%d/", dockertest.URLPort(s.node2.URL()))
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
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	a.runOnce()
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
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	a.runOnce()
	c.Assert(s.S.logBuf.String(), check.Matches, `(?s).*\[node autoscale\].*unable to lock app myapp, aborting.*`)
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunMemoryBasedLockedApp(c *check.C) {
	config.Set("docker:scheduler:max-used-memory", 0.8)
	config.Unset("docker:auto-scale:max-container-count")
	defer config.Unset("docker:scheduler:max-used-memory")
	config.Set("docker:scheduler:total-memory-metadata", "totalMem")
	defer config.Unset("docker:scheduler:total-memory-metadata")
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
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	a.runOnce()
	c.Assert(s.S.logBuf.String(), check.Matches, `(?s).*\[node autoscale\].*unable to lock app myapp, aborting.*`)
	nodes, err := s.p.cluster.Nodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
}

func (s *AutoScaleSuite) TestAutoScaleConfigRunOnceRulesPerPool(c *check.C) {
	config.Set("docker:scheduler:total-memory-metadata", "totalMem")
	defer config.Unset("docker:scheduler:total-memory-metadata")
	healerConst := dockertest.NewMultiHealerIaaSConstructor(
		[]string{"[::]", "[::1]"},
		[]int{dockertest.URLPort(s.node2.URL()), dockertest.URLPort(s.node3.URL())},
		nil,
	)
	iaas.RegisterIaasProvider("my-scale-iaas", healerConst)
	otherUrl := fmt.Sprintf("http://localhost:%d", dockertest.URLPort(s.node2.URL()))
	node := cluster.Node{Address: otherUrl, Metadata: map[string]string{
		"pool":     "pool2",
		"iaas":     "my-scale-iaas",
		"totalMem": "125000",
	}}
	err := s.p.cluster.Register(node)
	c.Assert(err, check.IsNil)
	config.Unset("docker:auto-scale:max-container-count")
	coll, err := autoScaleRuleCollection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	rule1 := autoScaleRule{
		MetadataFilter:    "pool1",
		Enabled:           true,
		MaxContainerCount: 2,
		ScaleDownRatio:    1.333,
	}
	rule2 := autoScaleRule{
		MetadataFilter: "pool2",
		Enabled:        true,
		ScaleDownRatio: 1.333,
		MaxMemoryRatio: 0.8,
	}
	err = coll.Insert(rule1)
	c.Assert(err, check.IsNil)
	err = coll.Insert(rule2)
	c.Assert(err, check.IsNil)
	appInstance2 := provisiontest.NewFakeApp("myapp2", "python", 0)
	s.p.Provision(appInstance2)
	err = provision.AddPool(provision.AddPoolOptions{Name: "pool2"})
	c.Assert(err, check.IsNil)
	appStruct := &app.App{
		Name: appInstance2.GetName(),
		Pool: "pool2",
		Plan: app.Plan{Memory: 21000},
	}
	err = s.S.storage.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	imageId, err := appCurrentImageName(appInstance2.GetName())
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"procfile": "web: python ./myapp",
	}
	err = saveImageCustomData(imageId, customData)
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 4}},
		app:         s.appInstance,
		imageId:     s.imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	_, err = addContainersWithHost(&changeUnitsPipelineArgs{
		toAdd:       map[string]*containersToAdd{"web": {Quantity: 6}},
		app:         appInstance2,
		imageId:     imageId,
		provisioner: s.p,
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.cluster.UnfilteredNodes()
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 4)
	evts, err := listAutoScaleEvents(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 2)
	c.Assert(evts[0].Action, check.Equals, "add")
	c.Assert(evts[0].Successful, check.Equals, true)
	c.Assert(evts[0].Error, check.Equals, "")
	c.Assert(evts[0].Nodes, check.HasLen, 1)
	c.Assert(evts[1].Action, check.Equals, "add")
	c.Assert(evts[1].Successful, check.Equals, true)
	c.Assert(evts[1].Error, check.Equals, "")
	c.Assert(evts[1].Nodes, check.HasLen, 1)
	metadataValues := []string{evts[0].MetadataValue, evts[1].MetadataValue}
	sort.Strings(metadataValues)
	c.Assert(metadataValues, check.DeepEquals, []string{"pool1", "pool2"})
	reasons := []string{evts[0].Reason, evts[1].Reason}
	sort.Strings(reasons)
	c.Assert(reasons, check.DeepEquals, []string{
		"can't add 21000 bytes to an existing node, adding 1 nodes",
		"number of free slots is -2, adding 1 nodes",
	})
}

func (s *S) TestAutoScaleConfigRunParamsError(c *check.C) {
	config.Set("docker:auto-scale:max-container-count", 0)
	a := autoScaleConfig{
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	a.runOnce()
	c.Assert(s.logBuf.String(), check.Matches, `(?s).*invalid rule, either memory information or max container count must be set.*`)
	config.Set("docker:auto-scale:max-container-count", 10)
	config.Set("docker:auto-scale:scale-down-ratio", 0.9)
	defer config.Unset("docker:auto-scale:scale-down-ratio")
	a = autoScaleConfig{
		done:            make(chan bool),
		provisioner:     s.p,
		GroupByMetadata: "pool",
	}
	a.runOnce()
	c.Assert(s.logBuf.String(), check.Matches, `(?s).*scale down ratio needs to be greater than 1.0, got .+`)
}

func (s *S) TestAutoScaleConfigRunDefaultValues(c *check.C) {
	config.Set("docker:auto-scale:max-container-count", 10)
	a := autoScaleConfig{
		done:        make(chan bool),
		provisioner: s.p,
	}
	a.runOnce()
	c.Assert(a.RunInterval, check.Equals, 1*time.Hour)
	c.Assert(a.WaitTimeNewMachine, check.Equals, 5*time.Minute)
	rule, err := autoScaleRuleForMetadata("")
	c.Assert(err, check.IsNil)
	c.Assert(rule.ScaleDownRatio > 1.332 && rule.ScaleDownRatio < 1.334, check.Equals, true)
}

func (s *S) TestAutoScaleConfigRunConfigValues(c *check.C) {
	config.Set("docker:auto-scale:max-container-count", 10)
	config.Set("docker:auto-scale:scale-down-ratio", 1.5)
	defer config.Unset("docker:auto-scale:scale-down-ratio")
	a := autoScaleConfig{
		done:               make(chan bool),
		provisioner:        s.p,
		RunInterval:        10 * time.Minute,
		WaitTimeNewMachine: 7 * time.Minute,
	}
	a.runOnce()
	c.Assert(a.RunInterval, check.Equals, 10*time.Minute)
	c.Assert(a.WaitTimeNewMachine, check.Equals, 7*time.Minute)
	rule, err := autoScaleRuleForMetadata("")
	c.Assert(err, check.IsNil)
	c.Assert(rule.ScaleDownRatio > 1.49 && rule.ScaleDownRatio < 1.51, check.Equals, true)
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
