// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
	mgo "gopkg.in/mgo.v2"
)

func (s *S) TestChooseDBSwarmNode(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	coll, err := nodeAddrCollection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	err = coll.Insert(NodeAddrs{UniqueID: uniqueDocumentID, Addresses: []string{srv.URL()}})
	c.Assert(err, check.IsNil)
	cli, err := chooseDBSwarmNode()
	c.Assert(err, check.IsNil)
	err = cli.Ping()
	c.Assert(err, check.IsNil)
}

func (s *S) TestChooseDBSwarmNodeFallback(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	err = s.p.AddNode(provision.AddNodeOptions{Address: srv.URL()})
	c.Assert(err, check.IsNil)
	coll, err := nodeAddrCollection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	_, err = coll.UpsertId(uniqueDocumentID, NodeAddrs{UniqueID: uniqueDocumentID, Addresses: []string{"invalid", "invalid", srv.URL()}})
	c.Assert(err, check.IsNil)
	cli, err := chooseDBSwarmNode()
	c.Assert(err, check.IsNil)
	err = cli.Ping()
	c.Assert(err, check.IsNil)
	var nodeAddrs NodeAddrs
	err = coll.FindId(uniqueDocumentID).One(&nodeAddrs)
	c.Assert(err, check.IsNil)
	c.Assert(nodeAddrs.Addresses, check.DeepEquals, []string{srv.URL()})
}

func (s *S) TestChooseDBSwarmNodeOnlyInvalid(c *check.C) {
	coll, err := nodeAddrCollection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	err = coll.Insert(NodeAddrs{UniqueID: uniqueDocumentID, Addresses: []string{"invalid"}})
	c.Assert(err, check.IsNil)
	_, err = chooseDBSwarmNode()
	c.Assert(err, check.Not(check.IsNil))
}

func (s *S) TestChooseDBSwarmNodeEmpty(c *check.C) {
	cli, err := chooseDBSwarmNode()
	c.Assert(errors.Cause(err), check.Equals, errNoSwarmNode)
	c.Assert(cli, check.IsNil)
}

func (s *S) TestRemoveDBSwarmNodes(c *check.C) {
	coll, err := nodeAddrCollection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	err = coll.Insert(NodeAddrs{UniqueID: uniqueDocumentID, Addresses: []string{"node1", "node2"}})
	c.Assert(err, check.IsNil)
	err = removeDBSwarmNodes()
	c.Assert(err, check.IsNil)
	var addrs NodeAddrs
	err = coll.FindId(uniqueDocumentID).One(&addrs)
	c.Assert(err, check.Equals, mgo.ErrNotFound)
	c.Assert(len(addrs.Addresses), check.Equals, 0)
}

func (s *S) TestRemoveDBSwarmNodesEmpty(c *check.C) {
	err := removeDBSwarmNodes()
	c.Assert(err, check.IsNil)
}
