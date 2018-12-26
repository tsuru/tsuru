// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"github.com/docker/docker/api/types/swarm"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	check "gopkg.in/check.v1"
)

func (s *S) TestSwarmNodeWrapper(c *check.C) {
	swarmNode := &swarm.Node{
		Spec: swarm.NodeSpec{
			Annotations: swarm.Annotations{
				Labels: map[string]string{
					"tsuru.internal-node-addr": "myaddr:1234",
					"tsuru.pool":               "p1",
					"l1":                       "v1",
					"tsuru.l2":                 "v2",
				},
			},
		},
		Status: swarm.NodeStatus{
			State: swarm.NodeStateReady,
		},
	}
	node := swarmNodeWrapper{Node: swarmNode}
	c.Assert(node.Address(), check.Equals, "myaddr:1234")
	c.Assert(node.Metadata(), check.DeepEquals, map[string]string{"tsuru.pool": "p1", "tsuru.l2": "v2"})
	c.Assert(node.Pool(), check.Equals, "p1")
	c.Assert(node.Status(), check.Equals, "ready")
	swarmNode.Status.Message = "msg1"
	c.Assert(node.Status(), check.Equals, "ready (msg1)")
	c.Assert(node.ExtraData(), check.DeepEquals, map[string]string{
		"tsuru.internal-node-addr": "myaddr:1234",
		"l1":                       "v1",
	})
	s.addCluster(c)
	var err error
	node.client, err = clusterForPool("")
	c.Assert(err, check.IsNil)
	c.Assert(node.ExtraData(), check.DeepEquals, map[string]string{
		"tsuru.io/cluster":         "c1",
		"tsuru.internal-node-addr": "myaddr:1234",
		"l1":                       "v1",
	})
}

func (s *S) TestSwarmNodeWrapperEmpty(c *check.C) {
	empty := swarmNodeWrapper{Node: &swarm.Node{}}
	c.Assert(empty.Address(), check.Equals, "")
	c.Assert(empty.Metadata(), check.DeepEquals, map[string]string{})
	c.Assert(empty.Pool(), check.Equals, "")
	c.Assert(empty.Status(), check.Equals, "")
}

func (s *S) TestSwarmNodeUnits(c *check.C) {
	s.addCluster(c)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	units, err := nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "myapp:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName(a.GetName(), imgName)
	c.Assert(err, check.IsNil)
	err = s.p.AddUnits(a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err = nodes[0].Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
}
