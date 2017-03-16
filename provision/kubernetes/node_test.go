// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"gopkg.in/check.v1"
	"k8s.io/client-go/pkg/api/v1"
)

func (s *S) TestNodeAddress(c *check.C) {
	node := kubernetesNodeWrapper{
		node: &v1.Node{
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeInternalIP,
						Address: "192.168.99.100",
					},
					{
						Type:    v1.NodeExternalIP,
						Address: "200.0.0.1",
					},
				},
			},
		},
	}
	c.Assert(node.Address(), check.Equals, "192.168.99.100")
}

func (s *S) TestNodePool(c *check.C) {
	node := kubernetesNodeWrapper{
		node: &v1.Node{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{
					"pool": "p1",
				},
			},
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeInternalIP,
						Address: "192.168.99.100",
					},
				},
			},
		},
	}
	c.Assert(node.Pool(), check.Equals, "p1")
}

func (s *S) TestNodeStatus(c *check.C) {
	node := kubernetesNodeWrapper{
		node: &v1.Node{
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:   v1.NodeReady,
						Status: v1.ConditionTrue,
					},
				},
			},
		},
	}
	c.Assert(node.Status(), check.Equals, "Ready")
	node = kubernetesNodeWrapper{
		node: &v1.Node{
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:    v1.NodeReady,
						Status:  v1.ConditionFalse,
						Message: "node pending",
					},
				},
			},
		},
	}
	c.Assert(node.Status(), check.Equals, "node pending")
	node = kubernetesNodeWrapper{
		node: &v1.Node{},
	}
	c.Assert(node.Status(), check.Equals, "Invalid")
}

func (s *S) TestNodeMetadata(c *check.C) {
	node := kubernetesNodeWrapper{
		node: &v1.Node{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{
					"pool": "p1",
					"m1":   "v1",
				},
			},
		},
	}
	c.Assert(node.Metadata(), check.DeepEquals, map[string]string{
		"pool": "p1",
		"m1":   "v1",
	})
}

func (s *S) TestNodeProvisioner(c *check.C) {
	node := kubernetesNodeWrapper{
		prov: s.p,
	}
	c.Assert(node.Provisioner(), check.Equals, s.p)
}

func (s *S) TestClusterNode(c *check.C) {
	node := clusterNode{
		address: "clusterAddr",
		prov:    s.p,
	}
	c.Assert(node.Pool(), check.Equals, "")
	c.Assert(node.Address(), check.Equals, "clusterAddr")
	c.Assert(node.Status(), check.Equals, "Ready")
	c.Assert(node.Metadata(), check.DeepEquals, map[string]string{
		"cluster": "true",
	})
	units, err := node.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.IsNil)
	c.Assert(node.Provisioner(), check.Equals, s.p)
}
