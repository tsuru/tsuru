// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/fsouza/go-dockerclient/testing"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
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
	defer coll.Close()
	var nodeAddrs NodeAddrs
	err = coll.FindId(uniqueDocumentID).One(&nodeAddrs)
	c.Assert(err, check.IsNil)
	c.Assert(nodeAddrs.Addresses, check.DeepEquals, []string{srv.URL()})
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
		c.Assert(err, check.IsNil, check.Commentf("server %d", i))
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

func (s *S) TestRemoveNode(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	metadata := map[string]string{"m1": "v1", "pool": "p1"}
	opts := provision.AddNodeOptions{
		Address:  srv.URL(),
		Metadata: metadata,
	}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	err = s.p.RemoveNode(provision.RemoveNodeOptions{
		Address: srv.URL(),
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.GetNode(srv.URL())
	c.Assert(errors.Cause(err), check.Equals, provision.ErrNodeNotFound)
}

func (s *S) TestRemoveNodeRebalance(c *check.C) {
	var reqs []*http.Request
	srv, err := testing.NewServer("127.0.0.1:0", nil, func(r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/nodes/") {
			reqs = append(reqs, r)
		}
	})
	c.Assert(err, check.IsNil)
	metadata := map[string]string{"m1": "v1", "pool": "p1"}
	opts := provision.AddNodeOptions{
		Address:  srv.URL(),
		Metadata: metadata,
	}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	reqs = nil
	err = s.p.RemoveNode(provision.RemoveNodeOptions{
		Address:   srv.URL(),
		Rebalance: true,
	})
	c.Assert(err, check.IsNil)
	_, err = s.p.GetNode(srv.URL())
	c.Assert(errors.Cause(err), check.Equals, provision.ErrNodeNotFound)
	c.Assert(reqs, check.HasLen, 2)
	c.Assert(reqs[0].Method, check.Equals, "POST")
	c.Assert(reqs[1].Method, check.Equals, "DELETE")
}

func (s *S) TestRemoveNodeNotFound(c *check.C) {
	err := s.p.RemoveNode(provision.RemoveNodeOptions{
		Address: "localhost:1000",
	})
	c.Assert(errors.Cause(err), check.Equals, provision.ErrNodeNotFound)
}

func (s *S) TestImageDeploy(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	opts := provision.AddNodeOptions{Address: srv.URL()}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	image, err := s.p.ImageDeploy(a, "myimg:v1", nil)
	c.Assert(err, check.IsNil)
	c.Assert(image, check.Equals, "myimg:v1")
	units, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(units, check.DeepEquals, []provision.Unit{
		{ID: units[0].ID, AppName: a.Name, Ip: "127.0.0.1", Status: "started", Address: &url.URL{Scheme: "http", Host: "127.0.0.1:0"}},
	})
	image, err = s.p.ImageDeploy(a, "myimg:v2", nil)
	c.Assert(err, check.IsNil)
	c.Assert(image, check.Equals, "myimg:v2")
	unitsAfter, err := s.p.Units(a)
	c.Assert(err, check.IsNil)
	c.Assert(unitsAfter, check.HasLen, 1)
	c.Assert(units[0].ID, check.Not(check.Equals), unitsAfter[0].ID)
	c.Assert(unitsAfter, check.DeepEquals, []provision.Unit{
		{ID: unitsAfter[0].ID, AppName: a.Name, Ip: "127.0.0.1", Status: "started", Address: &url.URL{Scheme: "http", Host: "127.0.0.1:0"}},
	})
}
