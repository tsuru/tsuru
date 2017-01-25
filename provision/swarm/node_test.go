// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"github.com/docker/docker/api/types/swarm"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
)

func (s *S) TestSwarmNodeWrapper(c *check.C) {
	swarmNode := &swarm.Node{
		Spec: swarm.NodeSpec{
			Annotations: swarm.Annotations{
				Labels: map[string]string{
					labelNodeDockerAddr.String(): "myaddr:1234",
					labelNodePoolName.String():   "p1",
					"l1": "v1",
				},
			},
		},
		Status: swarm.NodeStatus{
			State: swarm.NodeStateReady,
		},
	}
	node := swarmNodeWrapper{Node: swarmNode}
	c.Assert(node.Address(), check.Equals, "myaddr:1234")
	c.Assert(node.Metadata(), check.DeepEquals, map[string]string{labelNodePoolName.String(): "p1", "l1": "v1"})
	c.Assert(node.Pool(), check.Equals, "p1")
	c.Assert(node.Status(), check.Equals, "ready")
	swarmNode.Status.Message = "msg1"
	c.Assert(node.Status(), check.Equals, "ready (msg1)")
}

func (s *S) TestSwarmNodeWrapperEmpty(c *check.C) {
	empty := swarmNodeWrapper{Node: &swarm.Node{}}
	c.Assert(empty.Address(), check.Equals, "")
	c.Assert(empty.Metadata(), check.DeepEquals, map[string]string{})
	c.Assert(empty.Pool(), check.Equals, "")
	c.Assert(empty.Status(), check.Equals, "")
}

func (s *S) TestSwarmNodeUnits(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
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
