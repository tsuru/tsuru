// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
)

func (s *S) TestAddNode(c *check.C) {
	url := "https://192.168.99.100:8443"
	metadata := map[string]string{"m1": "v1", "m2": "v2", "pool": "p1"}
	opts := provision.AddNodeOptions{
		Address:  url,
		Metadata: metadata,
	}
	err := s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	// node, err := s.p.GetNode(url)
	// c.Assert(err, check.IsNil)
	// c.Assert(node.Address(), check.Equals, url)
	// c.Assert(node.Metadata(), check.DeepEquals, metadata)
	// c.Assert(node.Pool(), check.Equals, "p1")
	// c.Assert(node.Status(), check.Equals, "ready")
	// coll, err := nodeAddrCollection()
	// c.Assert(err, check.IsNil)
	// defer coll.Close()
	// var nodeAddrs NodeAddrs
	// err = coll.FindId(uniqueDocumentID).One(&nodeAddrs)
	// c.Assert(err, check.IsNil)
	// c.Assert(nodeAddrs.Addresses, check.DeepEquals, []string{url})
}
