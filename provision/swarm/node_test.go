// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"github.com/docker/docker/api/types/swarm"
	"gopkg.in/check.v1"
)

func (s *S) TestSwarmNodeWrapper(c *check.C) {
	swarmNode := &swarm.Node{
		Spec: swarm.NodeSpec{
			Annotations: swarm.Annotations{
				Labels: map[string]string{
					labelDockerAddr: "myaddr:1234",
					"pool":          "p1",
					"l1":            "v1",
				},
			},
		},
		Status: swarm.NodeStatus{
			State: swarm.NodeStateReady,
		},
	}
	node := swarmNodeWrapper{Node: swarmNode}
	c.Assert(node.Address(), check.Equals, "myaddr:1234")
	c.Assert(node.Metadata(), check.DeepEquals, map[string]string{"pool": "p1", "l1": "v1"})
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
