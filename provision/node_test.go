// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision_test

import (
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
)

type S struct{}

var _ = check.Suite(&S{})

func (s *S) TestFindNodeByAddrs(c *check.C) {
	p := provisiontest.NewFakeProvisioner()
	err := p.AddNode(provision.AddNodeOptions{
		Address: "http://addr1",
	})
	c.Assert(err, check.IsNil)
	node, err := provision.FindNodeByAddrs(p, []string{"addr1", "notfound"})
	c.Assert(err, check.IsNil)
	c.Assert(node.Address(), check.Equals, "http://addr1")
	_, err = provision.FindNodeByAddrs(p, []string{"addr2"})
	c.Assert(err, check.Equals, provision.ErrNodeNotFound)
}

func (s *S) TestFindNodeByAddrsAmbiguous(c *check.C) {
	p := provisiontest.NewFakeProvisioner()
	err := p.AddNode(provision.AddNodeOptions{
		Address: "http://addr1",
	})
	c.Assert(err, check.IsNil)
	err = p.AddNode(provision.AddNodeOptions{
		Address: "http://addr2",
	})
	c.Assert(err, check.IsNil)
	_, err = provision.FindNodeByAddrs(p, []string{"addr1", "addr2"})
	c.Assert(err, check.ErrorMatches, `addrs match multiple nodes: \[addr1 addr2\]`)
}
