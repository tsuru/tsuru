// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package autoscale

import (
	"sort"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/iaas"
	iaasTesting "github.com/tsuru/tsuru/iaas/testing"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func Test(t *testing.T) { check.TestingT(t) }

var _ = check.Suite(&S{})

type S struct {
	appInstance *provisiontest.FakeApp
	p           *provisiontest.FakeProvisioner
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "autoscale_tests_s")
	config.Set("docker:collection", "docker")
}

func (s *S) SetUpTest(c *check.C) {
	iaas.ResetAll()
	routertest.FakeRouter.Reset()
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	dbtest.ClearAllCollections(conn.Apps().Database)
	opts := provision.AddPoolOptions{Name: "pool1"}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	s.p = provisiontest.NewFakeProvisioner()
	s.appInstance = provisiontest.NewFakeApp("myapp", "python", 0)
	s.p.Provision(s.appInstance)
	appStruct := &app.App{
		Name: s.appInstance.GetName(),
		Pool: "pool1",
		Plan: app.Plan{Memory: 4194304},
	}
	err = conn.Apps().Insert(appStruct)
	c.Assert(err, check.IsNil)
	err = s.p.AddNode(provision.AddNodeOptions{
		Address: "http://n1:1",
		Metadata: map[string]string{
			"pool":     "pool1",
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
}

func (s *S) TearDownTest(c *check.C) {
	config.Unset("docker:auto-scale:max-container-count")
	config.Unset("docker:auto-scale:prevent-rebalance")
	config.Unset("docker:auto-scale:metadata-filter")
	config.Unset("docker:auto-scale:scale-down-ratio")
}

func (s *S) TestAutoScaleConfigRunOnce(c *check.C) {
	_, err := s.p.AddUnitsToNode(s.appInstance, 4, "web", nil, "n1:1")
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:        make(chan bool),
		provisioner: s.p,
	}
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address(), check.Not(check.Equals), nodes[1].Address())
	u0, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u0, check.HasLen, 2)
	u1, err := nodes[1].Units()
	c.Assert(err, check.IsNil)
	c.Assert(u1, check.HasLen, 2)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: "pool", Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toadd":       1,
			"result.torebalance": true,
			"result.reason":      "number of free slots is -2",
			"nodes": bson.M{"$elemMatch": bson.M{
				"_id": "http://n2:2",
			}},
		},
		LogMatches: `(?s).*running scaler.*countScaler.*pool1.*new machine created.*`,
	}, eventtest.HasEvent)
	a.runOnce()
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
	a := autoScaleConfig{
		done:        make(chan bool),
		provisioner: s.p,
	}
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
	a := autoScaleConfig{
		done:        make(chan bool),
		provisioner: s.p,
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
		Metadata: map[string]string{
			"pool":     "pool1",
			"iaas":     "my-scale-iaas",
			"totalMem": "25165824",
		},
	})
	c.Assert(err, check.IsNil)
	a := autoScaleConfig{
		done:        make(chan bool),
		provisioner: s.p,
	}
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
		Target: event.Target{Type: "pool", Value: "pool1"},
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
	a := autoScaleConfig{
		done:        make(chan bool),
		provisioner: s.p,
	}
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 3)
	c.Assert(nodes[0].Address(), check.Equals, "http://n1:1")
	c.Assert(nodes[1].Address(), check.Equals, "http://n2:2")
	c.Assert(nodes[2].Address(), check.Equals, "http://n3:3")
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: "pool", Value: "pool1"},
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
	a := autoScaleConfig{
		done:        make(chan bool),
		provisioner: s.p,
	}
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 3)
	c.Assert(nodes[0].Address(), check.Equals, "http://n1:1")
	c.Assert(nodes[1].Address(), check.Equals, "http://n2:2")
	c.Assert(nodes[2].Address(), check.Equals, "http://n3:3")
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: "pool", Value: "pool1"},
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
	a := autoScaleConfig{
		done:        make(chan bool),
		provisioner: s.p,
	}
	err = a.runOnce()
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	c.Assert(nodes[0].Address(), check.Equals, "http://n1:1")
	c.Assert(nodes[1].Address(), check.Equals, "http://n2:2")
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: "pool", Value: "pool1"},
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
