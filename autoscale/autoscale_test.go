// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package autoscale

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/iaas"
	iaasTesting "github.com/tsuru/tsuru/iaas/testing"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/safe"
	"github.com/tsuru/tsuru/servicemanager"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

var _ = check.Suite(&S{})

type S struct {
	appInstance *provisiontest.FakeApp
	p           *provisiontest.FakeProvisioner
	logBuf      *safe.Buffer
	conn        *db.Storage
	mockService struct {
		Plan *appTypes.MockPlanService
	}
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "autoscale_tests_s")
}

func (s *S) SetUpTest(c *check.C) {
	iaas.ResetAll()
	routertest.FakeRouter.Reset()
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	provision.Unregister("fake-extensible")
	provisiontest.ProvisionerInstance.Reset()
	s.p = provisiontest.ProvisionerInstance
	opts := pool.AddPoolOptions{Name: "pool1", Provisioner: "fake"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	s.appInstance = provisiontest.NewFakeApp("myapp", "python", 0)
	s.appInstance.Pool = "pool1"
	s.p.Provision(s.appInstance)
	plan := appTypes.Plan{Memory: 4194304, Name: "default", CpuShare: 10}
	s.mockService.Plan = &appTypes.MockPlanService{
		OnList: func() ([]appTypes.Plan, error) {
			return []appTypes.Plan{plan}, nil
		},
		OnDefaultPlan: func() (*appTypes.Plan, error) {
			return &plan, nil
		},
	}
	servicemanager.Plan = s.mockService.Plan
	appStruct := &app.App{
		Name: s.appInstance.GetName(),
		Pool: "pool1",
		Plan: plan,
	}
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	err = s.p.AddNode(provision.AddNodeOptions{
		Address: "http://n1:1",
		Pool:    "pool1",
		Metadata: map[string]string{
			"iaas":     "my-scale-iaas",
			"totalMem": "25165824",
		},
	})
	c.Assert(err, check.IsNil)
	healerConst := iaasTesting.NewMultiHealerIaaSConstructor(
		[]string{"n2", "n3"},
		[]int{2, 3},
		nil,
	)
	iaas.RegisterIaasProvider("my-scale-iaas", healerConst)
	config.Set("docker:auto-scale:max-container-count", 2)
	config.Set("docker:auto-scale:enabled", true)
	s.logBuf = safe.NewBuffer(nil)
	log.SetLogger(log.NewWriterLogger(s.logBuf, true))
}

func (s *S) TearDownTest(c *check.C) {
	app.GetAppRouterUpdater().Shutdown(context.Background())
	s.conn.Close()
	config.Unset("docker:auto-scale:max-container-count")
	config.Unset("docker:auto-scale:prevent-rebalance")
	config.Unset("docker:auto-scale:metadata-filter")
	config.Unset("docker:auto-scale:scale-down-ratio")
	config.Unset("docker:scheduler:max-used-memory")
	config.Unset("docker:auto-scale:enabled")
	config.Unset("docker:scheduler:total-memory-metadata")
}

func (s *S) TestAutoScaleConfigRunOnce(c *check.C) {
	_, err := s.p.AddUnitsToNode(s.appInstance, 4, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	a := newConfig()
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2, check.Commentf("log: %s", s.logBuf.String()))
	c.Assert(nodes[0].Address(), check.Not(check.Equals), nodes[1].Address())
	u0, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u0, check.HasLen, 2)
	u1, err := nodes[1].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u1, check.HasLen, 2)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toadd":       1,
			"result.torebalance": true,
			"result.reason":      "number of free slots is -2",
			"nodes": bson.M{"$elemMatch": bson.M{
				"_id": "http://n2:2",
			}},
		},
		LogMatches: `(?s).*running scaler.*countScaler.*pool1.*new machine created.*rebalancing - dry: false, force: true.*`,
	}, eventtest.HasEvent)
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	newNodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(newNodes, check.HasLen, 2)
	evts, err := event.All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	u0Again, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	u1Again, err := nodes[1].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u0, check.DeepEquals, u0Again)
	c.Assert(u1, check.DeepEquals, u1Again)
	locked, err := app.AcquireApplicationLock(s.appInstance.GetName(), "x", "y")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
}

func (s *S) TestAutoScaleConfigRunOnceNoRebalance(c *check.C) {
	config.Set("docker:auto-scale:prevent-rebalance", true)
	defer config.Unset("docker:auto-scale:prevent-rebalance")
	_, err := s.p.AddUnitsToNode(s.appInstance, 4, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	a := newConfig()
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address(), check.Equals, "http://n1:1")
	c.Assert(nodes[1].Address(), check.Equals, "http://n2:2")
	u0, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u0, check.HasLen, 4)
	u1, err := nodes[1].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u1, check.HasLen, 0)
}

func (s *S) TestAutoScaleConfigRunOnceNoContainers(c *check.C) {
	a := Config{
		done: make(chan bool),
	}
	err := a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	evts, err := event.All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
}

func (s *S) TestAutoScaleConfigRunOnceNoContainersMultipleNodes(c *check.C) {
	err := s.p.AddNode(provision.AddNodeOptions{
		Address: "http://n2:2",
		Pool:    "pool1",
		Metadata: map[string]string{
			"iaas":     "my-scale-iaas",
			"totalMem": "25165824",
		},
	})
	c.Assert(err, check.IsNil)
	a := newConfig()
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	removedAddr := "http://n1:1"
	if nodes[0].Address() == removedAddr {
		removedAddr = "http://n2:2"
	}
	nodeMatch := bson.M{"$elemMatch": bson.M{
		"_id": removedAddr,
	}}
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toadd":       0,
			"result.torebalance": false,
			"result.reason":      "number of free slots is 4",
			"result.toremove":    nodeMatch,
			"nodes":              nodeMatch,
		},
		LogMatches: `(?s).*running scaler.*countScaler.*pool1.*running event "remove".*pool1.*`,
	}, eventtest.HasEvent)
}

func (s *S) TestAutoScaleConfigRunOnceMultipleNodes(c *check.C) {
	_, err := s.p.AddUnitsToNode(s.appInstance, 6, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	a := newConfig()
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 3)
	c.Assert(nodes[0].Address(), check.Equals, "http://n1:1")
	c.Assert(nodes[1].Address(), check.Equals, "http://n2:2")
	c.Assert(nodes[2].Address(), check.Equals, "http://n3:3")
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toadd":       2,
			"result.torebalance": true,
			"result.reason":      "number of free slots is -4",
			"nodes":              bson.M{"$size": 2},
		},
	}, eventtest.HasEvent)
	u0, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u0, check.HasLen, 2)
	u1, err := nodes[1].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u1, check.HasLen, 2)
	u2, err := nodes[2].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u2, check.HasLen, 2)
}

func (s *S) TestAutoScaleConfigRunOnceMultipleNodesRoundUp(c *check.C) {
	_, err := s.p.AddUnitsToNode(s.appInstance, 5, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	a := newConfig()
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 3)
	c.Assert(nodes[0].Address(), check.Equals, "http://n1:1")
	c.Assert(nodes[1].Address(), check.Equals, "http://n2:2")
	c.Assert(nodes[2].Address(), check.Equals, "http://n3:3")
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toadd":       2,
			"result.torebalance": true,
			"result.reason":      "number of free slots is -3",
			"nodes":              bson.M{"$size": 2},
		},
	}, eventtest.HasEvent)
	u0, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	u1, err := nodes[1].Units()
	c.Assert(err, check.IsNil)
	u2, err := nodes[2].Units()
	c.Assert(err, check.IsNil)
	lens := []int{len(u0), len(u1), len(u2)}
	sort.Ints(lens)
	c.Assert(lens, check.DeepEquals, []int{1, 2, 2})
}

func (s *S) TestAutoScaleConfigRunOnceAddsAtLeastOne(c *check.C) {
	_, err := s.p.AddUnitsToNode(s.appInstance, 3, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	a := newConfig()
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address(), check.Equals, "http://n1:1")
	c.Assert(nodes[1].Address(), check.Equals, "http://n2:2")
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toadd":       1,
			"result.torebalance": true,
			"result.reason":      "number of free slots is -1",
			"nodes":              bson.M{"$size": 1},
		},
	}, eventtest.HasEvent)
	u0, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	u1, err := nodes[1].Units()
	c.Assert(err, check.IsNil)
	lens := []int{len(u0), len(u1)}
	sort.Ints(lens)
	c.Assert(lens, check.DeepEquals, []int{1, 2})
}

func (s *S) TestAutoScaleConfigRunOnceMultipleNodesPartialError(c *check.C) {
	s.p.PrepareFailure("AddNode:http://n3:3", errors.New("error adding node"))
	_, err := s.p.AddUnitsToNode(s.appInstance, 6, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	a := newConfig()
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address(), check.Equals, "http://n1:1")
	c.Assert(nodes[1].Address(), check.Equals, "http://n2:2")
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toadd":       2,
			"result.torebalance": true,
			"result.reason":      "number of free slots is -4",
			"nodes":              bson.M{"$size": 1},
		},
		LogMatches: `(?s).*not all required nodes were created: error adding new node*`,
	}, eventtest.HasEvent)
	u0, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u0, check.HasLen, 3)
	u1, err := nodes[1].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u1, check.HasLen, 3)
}

func (s *S) TestAutoScaleConfigRunOnceMultipleNodesAddNodesErrorRunRebalance(c *check.C) {
	machine, err := iaas.CreateMachineForIaaS("my-scale-iaas", map[string]string{})
	c.Assert(err, check.IsNil)
	err = s.p.AddNode(provision.AddNodeOptions{
		Address: machine.FormatNodeAddress(),
		Pool:    "pool1",
		Metadata: map[string]string{
			"iaas":     "my-scale-iaas",
			"totalMem": "25165824",
		},
	})
	c.Assert(err, check.IsNil)
	s.p.PrepareFailure("AddNode:http://n3:3", errors.New("my error adding node"))
	_, err = s.p.AddUnitsToNode(s.appInstance, 6, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	a := newConfig()
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address(), check.Equals, "http://n1:1")
	c.Assert(nodes[1].Address(), check.Equals, "http://n2:2")
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toadd":       1,
			"result.torebalance": true,
			"result.reason":      "number of free slots is -2",
			"nodes":              bson.M{"$size": 0},
		},
		ErrorMatches: `error adding new node http://n3:3: my error adding node`,
		LogMatches:   `(?s).*running scaler.*countScaler.*pool1.*new machine created.*.*error adding new node http://n3:3: my error adding node.*rebalancing - dry: false, force: false.*`,
	}, eventtest.HasEvent)
	u0, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u0, check.HasLen, 3)
	u1, err := nodes[1].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u1, check.HasLen, 3)
}

func (s *S) TestAutoScaleConfigRunOnceSingleNodeAddNodesErrorNoRebalance(c *check.C) {
	s.p.PrepareFailure("AddNode:http://n2:2", errors.New("my error adding node"))
	_, err := s.p.AddUnitsToNode(s.appInstance, 4, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	a := newConfig()
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, "http://n1:1")
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toadd":       1,
			"result.torebalance": false,
			"result.reason":      "number of free slots is -2",
			"nodes":              bson.M{"$size": 0},
		},
		ErrorMatches: `error adding new node http://n2:2: my error adding node`,
		LogMatches:   `(?s).*running scaler.*countScaler.*pool1.*new machine created.*.*error adding new node http://n2:2: my error adding node.*rebalancing - dry: false, force: false.*`,
	}, eventtest.HasEvent)
	u0, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u0, check.HasLen, 4)
}

func (s *S) TestAutoScaleConfigRunRebalanceOnly(c *check.C) {
	err := s.p.AddNode(provision.AddNodeOptions{
		Address: "http://n2:2",
		Pool:    "pool1",
		Metadata: map[string]string{
			"iaas":     "my-scale-iaas",
			"totalMem": "25165824",
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.AddUnitsToNode(s.appInstance, 4, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	a := newConfig()
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toadd":       0,
			"result.torebalance": true,
		},
		LogMatches: `(?s).*running scaler.*countScaler.*pool1.*rebalancing - dry: false, force: false.*`,
	}, eventtest.HasEvent)
	u0, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u0, check.HasLen, 2)
	u1, err := nodes[1].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u1, check.HasLen, 2)
}

func (s *S) TestAutoScaleConfigRunNoMatch(c *check.C) {
	err := s.p.RemoveNode(provision.RemoveNodeOptions{
		Address: "http://n1:1",
	})
	c.Assert(err, check.IsNil)
	err = s.p.AddNode(provision.AddNodeOptions{
		Address: "http://n1:1",
		Metadata: map[string]string{
			"iaas":     "my-scale-iaas",
			"totalMem": "25165824",
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.AddUnitsToNode(s.appInstance, 4, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	a := newConfig()
	a.runOnce()
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, "http://n1:1")
	evts, err := event.All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
	err = s.p.RemoveNode(provision.RemoveNodeOptions{
		Address: "http://n1:1",
	})
	c.Assert(err, check.IsNil)
	err = s.p.AddNode(provision.AddNodeOptions{
		Address: "http://n1:1",
		Pool:    "pool1",
		Metadata: map[string]string{
			"iaas":     "my-scale-iaas",
			"totalMem": "25165824",
		},
	})
	c.Assert(err, check.IsNil)
	config.Set("docker:auto-scale:metadata-filter", "pool2")
	defer config.Unset("docker:auto-scale:metadata-filter")
	a.runOnce()
	nodes, err = s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, "http://n1:1")
	evts, err = event.All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
	config.Set("docker:auto-scale:metadata-filter", "pool1")
	a.runOnce()
	nodes, err = s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	evts, err = event.All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
}

func (s *S) TestAutoScaleConfigRunStress(c *check.C) {
	_, err := s.p.AddUnitsToNode(s.appInstance, 4, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	wg := sync.WaitGroup{}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			a := Config{
				done: make(chan bool),
			}
			defer wg.Done()
			runErr := a.runOnce()
			c.Assert(runErr, check.IsNil)
		}()
	}
	wg.Wait()
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address(), check.Equals, "http://n1:1")
	c.Assert(nodes[1].Address(), check.Equals, "http://n2:2")
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toadd":       1,
			"result.torebalance": true,
			"result.reason":      "number of free slots is -2",
			"nodes":              bson.M{"$size": 1},
		},
	}, eventtest.HasEvent)
	u0, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u0, check.HasLen, 2)
	u1, err := nodes[1].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u1, check.HasLen, 2)
}

func (s *S) TestAutoScaleConfigRunMemoryBased(c *check.C) {
	config.Unset("docker:auto-scale:max-container-count")
	config.Set("docker:scheduler:max-used-memory", 0.8)
	config.Set("docker:scheduler:total-memory-metadata", "totalMem")
	_, err := s.p.AddUnitsToNode(s.appInstance, 4, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	a := newConfig()
	a.runOnce()
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2, check.Commentf("log: %s", s.logBuf.String()))
	c.Assert(nodes[0].Address(), check.Equals, "http://n1:1")
	c.Assert(nodes[1].Address(), check.Equals, "http://n2:2")
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toadd":       1,
			"result.torebalance": true,
			"result.reason":      "can't add 4194304 bytes to an existing node",
			"nodes":              bson.M{"$size": 1},
		},
	}, eventtest.HasEvent)
	// Also should have rebalanced
	u0, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u0, check.HasLen, 2)
	u1, err := nodes[1].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u1, check.HasLen, 2)
	// Should do nothing if calling on already scaled
	a.runOnce()
	nodes, err = s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	evts, err := event.All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 1)
	locked, err := app.AcquireApplicationLock(s.appInstance.GetName(), "x", "y")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
}

func (s *S) TestAutoScaleConfigRunMemoryBasedMultipleNodes(c *check.C) {
	config.Unset("docker:auto-scale:max-container-count")
	config.Set("docker:scheduler:max-used-memory", 0.8)
	config.Set("docker:scheduler:total-memory-metadata", "totalMem")
	_, err := s.p.AddUnitsToNode(s.appInstance, 9, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	a := newConfig()
	a.runOnce()
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 3)
	c.Assert(nodes[0].Address(), check.Equals, "http://n1:1")
	c.Assert(nodes[1].Address(), check.Equals, "http://n2:2")
	c.Assert(nodes[2].Address(), check.Equals, "http://n3:3")
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toadd":       2,
			"result.torebalance": true,
			"result.reason":      "can't add 4194304 bytes to an existing node",
			"nodes":              bson.M{"$size": 2},
		},
	}, eventtest.HasEvent)
	// Also should have rebalanced
	u0, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u0, check.HasLen, 3)
	u1, err := nodes[1].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u1, check.HasLen, 3)
	u2, err := nodes[2].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u2, check.HasLen, 3)
}

func (s *S) TestAutoScaleConfigRunOnceMemoryBasedNoContainersMultipleNodes(c *check.C) {
	config.Unset("docker:auto-scale:max-container-count")
	config.Set("docker:scheduler:max-used-memory", 0.8)
	config.Set("docker:scheduler:total-memory-metadata", "totalMem")
	err := s.p.AddNode(provision.AddNodeOptions{
		Address: "http://n2:2",
		Pool:    "pool1",
		Metadata: map[string]string{
			"iaas":     "my-scale-iaas",
			"totalMem": "25165824",
		},
	})
	c.Assert(err, check.IsNil)
	a := newConfig()
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toremove":    bson.M{"$size": 1},
			"result.torebalance": false,
			"result.reason":      "containers can be distributed in only 1 nodes",
			"nodes":              bson.M{"$size": 1},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAutoScaleConfigRunPriorityToCountBased(c *check.C) {
	config.Set("docker:scheduler:max-used-memory", 0.8)
	config.Set("docker:scheduler:total-memory-metadata", "totalMem")
	_, err := s.p.AddUnitsToNode(s.appInstance, 4, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	a := newConfig()
	a.runOnce()
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address(), check.Equals, "http://n1:1")
	c.Assert(nodes[1].Address(), check.Equals, "http://n2:2")
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toadd":       1,
			"result.torebalance": true,
			"result.reason":      "number of free slots is -2",
			"nodes":              bson.M{"$size": 1},
		},
	}, eventtest.HasEvent)
	// Also should have rebalanced
	u0, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u0, check.HasLen, 2)
	u1, err := nodes[1].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u1, check.HasLen, 2)
}

func (s *S) TestAutoScaleConfigRunMemoryBasedPlanTooBig(c *check.C) {
	config.Unset("docker:auto-scale:max-container-count")
	config.Set("docker:scheduler:max-used-memory", 0.8)
	config.Set("docker:scheduler:total-memory-metadata", "totalMem")
	plan := appTypes.Plan{Memory: 25165824, Name: "default", CpuShare: 10}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{plan}, nil
	}
	_, err := s.p.AddUnitsToNode(s.appInstance, 4, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	a := newConfig()
	a.runOnce()
	c.Assert(s.logBuf, check.Matches, `(?s).*error scaling group pool1: aborting, impossible to fit max plan memory of 25165824 bytes, node max available memory is 20132659.*`)
	c.Assert(eventtest.EventDesc{
		Target:       event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:         "autoscale",
		ErrorMatches: `error scaling group pool1: aborting, impossible to fit max plan memory of 25165824 bytes, node max available memory is 20132659`,
	}, eventtest.HasEvent)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
}

func (s *S) TestAutoScaleConfigRunScaleDown(c *check.C) {
	config.Set("docker:auto-scale:max-container-count", 4)
	err := s.p.AddNode(provision.AddNodeOptions{
		Address: "http://n2:2",
		Pool:    "pool1",
		Metadata: map[string]string{
			"iaas":     "my-scale-iaas",
			"totalMem": "25165824",
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.AddUnitsToNode(s.appInstance, 1, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	_, err = s.p.AddUnitsToNode(s.appInstance, 1, "web", nil, "n2:2")
	c.Assert(err, check.IsNil)
	a := newConfig()
	a.runOnce()
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toremove":    bson.M{"$size": 1},
			"result.torebalance": false,
			"result.reason":      "number of free slots is 6",
			"nodes":              bson.M{"$size": 1},
		},
	}, eventtest.HasEvent)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	u0, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u0, check.HasLen, 2)
}

func (s *S) TestAutoScaleConfigRunScaleDownMultipleNodes(c *check.C) {
	config.Set("docker:auto-scale:max-container-count", 5)
	err := s.p.AddNode(provision.AddNodeOptions{
		Address: "http://n2:2",
		Pool:    "pool1",
		Metadata: map[string]string{
			"iaas":     "my-scale-iaas",
			"totalMem": "25165824",
		},
	})
	c.Assert(err, check.IsNil)
	err = s.p.AddNode(provision.AddNodeOptions{
		Address: "http://n3:3",
		Pool:    "pool1",
		Metadata: map[string]string{
			"iaas":     "my-scale-iaas",
			"totalMem": "25165824",
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.AddUnitsToNode(s.appInstance, 1, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	_, err = s.p.AddUnitsToNode(s.appInstance, 1, "web", nil, "n2:2")
	c.Assert(err, check.IsNil)
	_, err = s.p.AddUnitsToNode(s.appInstance, 1, "web", nil, "n3:3")
	c.Assert(err, check.IsNil)
	a := newConfig()
	a.runOnce()
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toremove":    bson.M{"$size": 2},
			"result.torebalance": false,
			"result.reason":      "number of free slots is 12",
			"nodes":              bson.M{"$size": 2},
		},
	}, eventtest.HasEvent)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	u0, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u0, check.HasLen, 3)
}

func (s *S) TestAutoScaleConfigRunScaleDownMemoryScaler(c *check.C) {
	config.Unset("docker:auto-scale:max-container-count")
	config.Set("docker:scheduler:max-used-memory", 0.8)
	config.Set("docker:scheduler:total-memory-metadata", "totalMem")
	err := s.p.AddNode(provision.AddNodeOptions{
		Address: "http://n2:2",
		Pool:    "pool1",
		Metadata: map[string]string{
			"iaas":     "my-scale-iaas",
			"totalMem": "25165824",
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.AddUnitsToNode(s.appInstance, 1, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	_, err = s.p.AddUnitsToNode(s.appInstance, 1, "web", nil, "n2:2")
	c.Assert(err, check.IsNil)
	a := newConfig()
	a.runOnce()
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toremove":    bson.M{"$size": 1},
			"result.torebalance": false,
			"result.reason":      "containers can be distributed in only 1 nodes",
			"nodes":              bson.M{"$size": 1},
		},
	}, eventtest.HasEvent)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	u0, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u0, check.HasLen, 2)
}

func (s *S) TestAutoScaleConfigRunScaleDownMemoryScalerMultipleNodes(c *check.C) {
	config.Set("docker:scheduler:max-used-memory", 0.8)
	config.Unset("docker:auto-scale:max-container-count")
	config.Set("docker:scheduler:total-memory-metadata", "totalMem")
	err := s.p.AddNode(provision.AddNodeOptions{
		Address: "http://n2:2",
		Pool:    "pool1",
		Metadata: map[string]string{
			"iaas":     "my-scale-iaas",
			"totalMem": "25165824",
		},
	})
	c.Assert(err, check.IsNil)
	err = s.p.AddNode(provision.AddNodeOptions{
		Address: "http://n3:3",
		Pool:    "pool1",
		Metadata: map[string]string{
			"iaas":     "my-scale-iaas",
			"totalMem": "25165824",
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.AddUnitsToNode(s.appInstance, 1, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	_, err = s.p.AddUnitsToNode(s.appInstance, 1, "web", nil, "n2:2")
	c.Assert(err, check.IsNil)
	_, err = s.p.AddUnitsToNode(s.appInstance, 1, "web", nil, "n3:3")
	c.Assert(err, check.IsNil)
	a := newConfig()
	a.runOnce()
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toremove":    bson.M{"$size": 2},
			"result.torebalance": false,
			"result.reason":      "containers can be distributed in only 1 nodes",
			"nodes":              bson.M{"$size": 2},
		},
	}, eventtest.HasEvent)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	u0, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u0, check.HasLen, 3)
}

func (s *S) TestAutoScaleConfigRunScaleDownRespectsMinNodes(c *check.C) {
	config.Set("docker:auto-scale:max-container-count", 4)
	err := s.p.RemoveNode(provision.RemoveNodeOptions{
		Address: "http://n1:1",
	})
	c.Assert(err, check.IsNil)
	err = s.p.AddNode(provision.AddNodeOptions{
		Address: "http://n1:1",
		Metadata: map[string]string{
			"iaas":    "my-scale-iaas",
			"network": "net1",
		},
	})
	c.Assert(err, check.IsNil)
	err = s.p.AddNode(provision.AddNodeOptions{
		Address: "http://n2:2",
		Metadata: map[string]string{
			"iaas":    "my-scale-iaas",
			"network": "net2",
		},
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.AddUnitsToNode(s.appInstance, 1, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	_, err = s.p.AddUnitsToNode(s.appInstance, 1, "web", nil, "n2:2")
	c.Assert(err, check.IsNil)
	a := newConfig()
	a.runOnce()
	evts, err := event.All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
}

func (s *S) TestAutoScaleConfigRunLockedApp(c *check.C) {
	_, err := s.p.AddUnitsToNode(s.appInstance, 4, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	locked, err := app.AcquireApplicationLock(s.appInstance.GetName(), "tsurud", "something")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
	s.logBuf.Reset()
	a := newConfig()
	a.runOnce()
	c.Assert(s.logBuf.String(), check.Matches, `(?s).*aborting scaler for now, gonna retry later: unable to lock app "myapp".*`)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	evts, err := event.All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
}

func (s *S) TestAutoScaleConfigRunMemoryBasedLockedApp(c *check.C) {
	config.Unset("docker:auto-scale:max-container-count")
	config.Set("docker:scheduler:max-used-memory", 0.8)
	config.Set("docker:scheduler:total-memory-metadata", "totalMem")
	_, err := s.p.AddUnitsToNode(s.appInstance, 4, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	locked, err := app.AcquireApplicationLock(s.appInstance.GetName(), "tsurud", "something")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
	a := newConfig()
	a.runOnce()
	c.Assert(s.logBuf.String(), check.Matches, `(?s).*aborting scaler for now, gonna retry later: unable to lock app "myapp".*`)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	evts, err := event.All()
	c.Assert(err, check.IsNil)
	c.Assert(evts, check.HasLen, 0)
}

func (s *S) TestAutoScaleConfigRunOnceRulesPerPool(c *check.C) {
	config.Unset("docker:auto-scale:max-container-count")
	config.Set("docker:scheduler:total-memory-metadata", "totalMem")
	err := s.p.AddNode(provision.AddNodeOptions{
		Address: "http://nx:9",
		Pool:    "pool2",
		Metadata: map[string]string{
			"iaas":     "my-scale-iaas",
			"totalMem": "25165824",
		},
	})
	c.Assert(err, check.IsNil)
	coll, err := autoScaleRuleCollection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	rule1 := Rule{
		MetadataFilter:    "pool1",
		Enabled:           true,
		MaxContainerCount: 2,
		ScaleDownRatio:    1.333,
	}
	rule2 := Rule{
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
	appInstance2.Pool = "pool2"
	s.p.Provision(appInstance2)
	err = pool.AddPool(pool.AddPoolOptions{Name: "pool2"})
	c.Assert(err, check.IsNil)
	appStruct := &app.App{
		Name: appInstance2.GetName(),
		Pool: "pool2",
		Plan: appTypes.Plan{Memory: 4194304},
	}
	err = s.conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	_, err = s.p.AddUnitsToNode(s.appInstance, 4, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	_, err = s.p.AddUnitsToNode(appInstance2, 6, "web", nil, "nx:9")
	c.Assert(err, check.IsNil)
	a := newConfig()
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 4, check.Commentf("log: %s", s.logBuf.String()))
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool2"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toadd":       1,
			"result.torebalance": true,
			"result.reason":      "can't add 4194304 bytes to an existing node",
			"nodes":              bson.M{"$size": 1},
		},
	}, eventtest.HasEvent)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: provision.PoolMetadataName, Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toadd":       1,
			"result.torebalance": true,
			"result.reason":      "number of free slots is -2",
			"nodes":              bson.M{"$size": 1},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAutoScaleConfigRunParamsError(c *check.C) {
	config.Set("docker:auto-scale:max-container-count", 0)
	a := newConfig()
	a.runOnce()
	c.Assert(s.logBuf.String(), check.Matches, `(?s).*invalid rule, either memory information or max container count must be set.*`)
	config.Set("docker:auto-scale:max-container-count", 10)
	config.Set("docker:auto-scale:scale-down-ratio", 0.9)
	defer config.Unset("docker:auto-scale:scale-down-ratio")
	a = newConfig()
	a.runOnce()
	c.Assert(s.logBuf.String(), check.Matches, `(?s).*scale down ratio needs to be greater than 1.0, got .+`)
}

func (s *S) TestAutoScaleConfigRunDefaultValues(c *check.C) {
	config.Set("docker:auto-scale:max-container-count", 10)
	a := newConfig()
	a.runOnce()
	c.Assert(a.RunInterval, check.Equals, 1*time.Hour)
	c.Assert(a.WaitTimeNewMachine, check.Equals, 5*time.Minute)
	rule, err := AutoScaleRuleForMetadata("")
	c.Assert(err, check.IsNil)
	c.Assert(rule.ScaleDownRatio > 1.332 && rule.ScaleDownRatio < 1.334, check.Equals, true)
}

func (s *S) TestAutoScaleConfigRunConfigValues(c *check.C) {
	config.Set("docker:auto-scale:max-container-count", 10)
	config.Set("docker:auto-scale:scale-down-ratio", 1.5)
	defer config.Unset("docker:auto-scale:scale-down-ratio")
	a := Config{
		done:               make(chan bool),
		RunInterval:        10 * time.Minute,
		WaitTimeNewMachine: 7 * time.Minute,
	}
	a.runOnce()
	c.Assert(a.RunInterval, check.Equals, 10*time.Minute)
	c.Assert(a.WaitTimeNewMachine, check.Equals, 7*time.Minute)
	rule, err := AutoScaleRuleForMetadata("")
	c.Assert(err, check.IsNil)
	c.Assert(rule.ScaleDownRatio > 1.49 && rule.ScaleDownRatio < 1.51, check.Equals, true)
}

func (s *S) TestAutoScaleCanRemoveNode(c *check.C) {
	nodes := []provision.Node{
		&provisiontest.FakeNode{Addr: "", Meta: map[string]string{
			provision.PoolMetadataName: "pool1",
			"zone": "zone1",
		}},
		&provisiontest.FakeNode{Addr: "", Meta: map[string]string{
			provision.PoolMetadataName: "pool1",
			"zone": "zone1",
		}},
		&provisiontest.FakeNode{Addr: "", Meta: map[string]string{
			provision.PoolMetadataName: "pool2",
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
	nodes = []provision.Node{
		&provisiontest.FakeNode{Addr: "", Meta: map[string]string{
			provision.PoolMetadataName: "pool1",
			"zone": "zone1",
		}},
		&provisiontest.FakeNode{Addr: "", Meta: map[string]string{
			provision.PoolMetadataName: "pool1",
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

func (s *S) TestChooseMetadataFromNodes(c *check.C) {
	nodes := []provision.Node{
		&provisiontest.FakeNode{Addr: "", Meta: map[string]string{
			provision.PoolMetadataName: "pool1",
			"zone": "zone1",
		}},
	}
	metadata, err := chooseMetadataFromNodes(nodes)
	c.Assert(err, check.IsNil)
	c.Assert(metadata, check.DeepEquals, map[string]string{
		provision.PoolMetadataName: "pool1",
		"zone": "zone1",
	})
	nodes = []provision.Node{
		&provisiontest.FakeNode{Addr: "", Meta: map[string]string{
			provision.PoolMetadataName: "pool1",
			"zone": "zone1",
		}},
		&provisiontest.FakeNode{Addr: "", Meta: map[string]string{
			provision.PoolMetadataName: "pool1",
			"zone": "zone1",
		}},
		&provisiontest.FakeNode{Addr: "", Meta: map[string]string{
			provision.PoolMetadataName: "pool1",
			"zone": "zone2",
		}},
	}
	metadata, err = chooseMetadataFromNodes(nodes)
	c.Assert(err, check.IsNil)
	c.Assert(metadata, check.DeepEquals, map[string]string{
		provision.PoolMetadataName: "pool1",
		"zone": "zone2",
	})
	nodes = []provision.Node{
		&provisiontest.FakeNode{Addr: "", Meta: map[string]string{
			provision.PoolMetadataName: "pool1",
			"zone": "zone1",
		}},
		&provisiontest.FakeNode{Addr: "", Meta: map[string]string{
			provision.PoolMetadataName: "pool2",
			"zone": "zone2",
		}},
		&provisiontest.FakeNode{Addr: "", Meta: map[string]string{
			provision.PoolMetadataName: "pool2",
			"zone": "zone2",
		}},
	}
	metadata, err = chooseMetadataFromNodes(nodes)
	c.Assert(err, check.IsNil)
	c.Assert(metadata, check.DeepEquals, map[string]string{
		provision.PoolMetadataName: "pool1",
		"zone": "zone1",
	})
	nodes = []provision.Node{
		&provisiontest.FakeNode{Addr: "", Meta: map[string]string{
			provision.PoolMetadataName: "pool1",
			"zone": "zone1",
		}},
		&provisiontest.FakeNode{Addr: "", Meta: map[string]string{
			provision.PoolMetadataName: "pool2",
			"zone": "zone2",
		}},
		&provisiontest.FakeNode{Addr: "", Meta: map[string]string{
			provision.PoolMetadataName: "pool2",
			"zone": "zone3",
		}},
	}
	_, err = chooseMetadataFromNodes(nodes)
	c.Assert(err, check.ErrorMatches, "unbalanced metadata for node group:.*")
}
