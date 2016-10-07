// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import "gopkg.in/check.v1"

func (s *S) TestAddress(c *check.C) {
	node := kubernetesNodeWrapper{Addresses: []string{"192.168.99.100"}}
	c.Assert(node.Address(), check.Equals, "192.168.99.100")
}
