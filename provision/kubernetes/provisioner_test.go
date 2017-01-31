// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
)

func (s *S) TestListNodes(c *check.C) {
	url := "https://192.168.99.100:8443"
	opts := provision.AddNodeOptions{
		Address: url,
	}
	err := s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	defer s.p.RemoveNode(provision.RemoveNodeOptions{})
	nodes, err := s.p.ListNodes([]string{})
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, url)
}

func (s *S) TestListNodesWithoutNodes(c *check.C) {
	nodes, err := s.p.ListNodes([]string{})
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
}

func (s *S) TestListNodesFilteringByAddress(c *check.C) {
	url := "https://192.168.99.100:8443"
	opts := provision.AddNodeOptions{
		Address: url,
	}
	err := s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	defer s.p.RemoveNode(provision.RemoveNodeOptions{})
	nodes, err := s.p.ListNodes([]string{"https://192.168.99.101"})
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
}

func (s *S) TestAddNode(c *check.C) {
	url := "https://192.168.99.100:8443"
	opts := provision.AddNodeOptions{
		Address: url,
	}
	err := s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	defer s.p.RemoveNode(provision.RemoveNodeOptions{})
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	node := nodes[0]
	c.Assert(node.Address(), check.Equals, url)
}

func (s *S) TestRemoveNode(c *check.C) {
	url := "https://192.168.99.100:8443"
	addNodeOpts := provision.AddNodeOptions{
		Address: url,
	}
	err := s.p.AddNode(addNodeOpts)
	c.Assert(err, check.IsNil)
	removeNodeOpts := provision.RemoveNodeOptions{
		Address: url,
	}
	err = s.p.RemoveNode(removeNodeOpts)
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
}

func (s *S) TestImageDeploy(c *check.C) {
	url := "https://192.168.99.100:8443"
	opts := provision.AddNodeOptions{
		Address: url,
	}
	err := s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	defer s.p.RemoveNode(provision.RemoveNodeOptions{})
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	s.p.ImageDeploy(a, "imageName", evt)
	c.Assert(err, check.IsNil)
}

func (s *S) TestUnits(c *check.C) {
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.IsNil)
}

func (s *S) TestGetNode(c *check.C) {
	url := "https://192.168.99.100:8443"
	opts := provision.AddNodeOptions{
		Address: url,
	}
	err := s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	defer s.p.RemoveNode(provision.RemoveNodeOptions{})
	node, err := s.p.GetNode(url)
	c.Assert(err, check.IsNil)
	c.Assert(node.Address(), check.Equals, url)
	node, err = s.p.GetNode("http://doesnotexist.com")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, provision.ErrNodeNotFound)
	c.Assert(node, check.IsNil)
}
