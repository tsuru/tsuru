// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"fmt"
	"net"

	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
)

func (s *S) TestInitialize(c *check.C) {
	config.Set("swarm:swarm-port", 0)
	err := s.p.Initialize()
	c.Assert(err, check.IsNil)
	c.Assert(swarmConfig.swarmPort, check.Equals, 0)
	config.Unset("swarm:swarm-port")
	err = s.p.Initialize()
	c.Assert(err, check.IsNil)
	c.Assert(swarmConfig.swarmPort, check.Equals, 2377)
	config.Unset("swarm:swarm-port")
	err = s.p.Initialize()
	c.Assert(err, check.IsNil)
	c.Assert(swarmConfig.swarmPort, check.Equals, 2377)
	// TODO(cezarsa): check TLSConfig loading
}

func (s *S) TestAddNode(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	metadata := map[string]string{"m1": "v1", "m2": "v2", "pool": "p1"}
	opts := provision.AddNodeOptions{
		Address:  srv.URL(),
		Metadata: metadata,
	}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	node, err := s.p.GetNode(srv.URL())
	c.Assert(err, check.IsNil)
	c.Assert(node.Address(), check.Equals, srv.URL())
	c.Assert(node.Metadata(), check.DeepEquals, metadata)
	c.Assert(node.Pool(), check.Equals, "p1")
	c.Assert(node.Status(), check.Equals, "ready")
	coll, err := nodeAddrCollection()
	c.Assert(err, check.IsNil)
	var all []NodeAddr
	err = coll.Find(nil).All(&all)
	c.Assert(err, check.IsNil)
	_, port, _ := net.SplitHostPort(srv.SwarmAddress())
	c.Assert(all, check.DeepEquals, []NodeAddr{{DockerAddress: srv.URL(), SwarmAddress: "127.0.0.1:" + port}})
}

func (s *S) TestAddNodeMultiple(c *check.C) {
	for i := 0; i < 5; i++ {
		srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
		c.Assert(err, check.IsNil)
		metadata := map[string]string{"count": fmt.Sprintf("%d", i), "pool": "p1"}
		opts := provision.AddNodeOptions{
			Address:  srv.URL(),
			Metadata: metadata,
		}
		err = s.p.AddNode(opts)
		c.Assert(err, check.IsNil)
	}
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 5)
	for i, n := range nodes {
		c.Assert(n.Metadata(), check.DeepEquals, map[string]string{
			"count": fmt.Sprintf("%d", i),
			"pool":  "p1",
		})
	}
}

func (s *S) TestListNodes(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	metadata := map[string]string{"m1": "v1", "pool": "p1"}
	opts := provision.AddNodeOptions{
		Address:  srv.URL(),
		Metadata: metadata,
	}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, srv.URL())
	c.Assert(nodes[0].Metadata(), check.DeepEquals, metadata)
	c.Assert(nodes[0].Pool(), check.DeepEquals, "p1")
	c.Assert(nodes[0].Status(), check.DeepEquals, "ready")
}

func (s *S) TestListNodesEmpty(c *check.C) {
	nodes, err := s.p.ListNodes(nil)
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 0)
}

func (s *S) TestGetNode(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	metadata := map[string]string{"m1": "v1", "pool": "p1"}
	opts := provision.AddNodeOptions{
		Address:  srv.URL(),
		Metadata: metadata,
	}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	node, err := s.p.GetNode(srv.URL())
	c.Assert(err, check.IsNil)
	c.Assert(node.Address(), check.Equals, srv.URL())
	c.Assert(node.Metadata(), check.DeepEquals, metadata)
	c.Assert(node.Pool(), check.DeepEquals, "p1")
	c.Assert(node.Status(), check.DeepEquals, "ready")
}

func (s *S) TestGetNodeNotFound(c *check.C) {
	_, err := s.p.GetNode("http://tai.shar.malkier")
	c.Assert(err, check.Equals, provision.ErrNodeNotFound)
}
