// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"gopkg.in/check.v1"
	"k8s.io/client-go/pkg/api/v1"
)

func (s *S) TestAddress(c *check.C) {
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
