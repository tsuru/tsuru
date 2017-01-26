// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package autoscale

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/iaas"
	iaasTesting "github.com/tsuru/tsuru/iaas/testing"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func Test(t *testing.T) { check.TestingT(t) }

var _ = check.Suite(&S{})

type S struct {
	testRepoRollback func()
	appInstance      *provisiontest.FakeApp
	imageId          string
	p                *provisiontest.FakeProvisioner
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "autoscale_tests_s")
	config.Set("docker:collection", "docker")
}

func (s *S) SetUpTest(c *check.C) {
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
	err = s.p.AddNode(provision.AddNodeOptions{
		Address: "n1:1",
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

func (s *S) TestAutoScaleConfigRunNoRebalance(c *check.C) {
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
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: "pool", Value: "pool1"},
		Kind:   "autoscale",
		EndCustomData: map[string]interface{}{
			"result.toadd":       1,
			"result.torebalance": false,
			"result.reason":      "number of free slots is -2",
			"nodes": bson.M{"$elemMatch": bson.M{
				"_id": "http://n2:2",
			}},
		},
		LogMatches: `(?s).*running scaler.*countScaler.*pool1.*new machine created.*`,
	}, eventtest.HasEvent)
}
