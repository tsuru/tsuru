// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mesos

import (
	"gopkg.in/check.v1"
	"sort"

	"github.com/tsuru/tsuru/provision"
)

func (s *S) TestListNodes(c *check.C) {
	s.addFakeNodes()
	nodes, err := s.p.ListNodes([]string{})
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 2)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Address() < nodes[j].Address() })
	c.Assert(nodes[0].Address(), check.Equals, "m1")
	c.Assert(nodes[1].Address(), check.Equals, "m2")
}

func (s *S) TestListNodesWithoutNodes(c *check.C) {
	nodes, err := s.p.ListNodes([]string{})
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
}

func (s *S) TestListNodesFilteringByAddress(c *check.C) {
	s.addFakeNodes()
	nodes, err := s.p.ListNodes([]string{"m1"})
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	nodes, err = s.p.ListNodes([]string{"invalid"})
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
}

func (s *S) TestGetNode(c *check.C) {
	s.addFakeNodes()
	node, err := s.p.GetNode("m1")
	c.Assert(err, check.IsNil)
	c.Assert(node.Address(), check.Equals, "m1")
	node, err = s.p.GetNode("doesnotexist.com")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, provision.ErrNodeNotFound)
	c.Assert(node, check.IsNil)
}
