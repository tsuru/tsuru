// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestListNodes(c *check.C) {
	coll, _ := nodeAddrCollection()
	defer coll.Close()
	coll.Insert(bson.M{"_id": uniqueDocumentID, "addresses": []string{"192.168.99.100"}})
	defer coll.RemoveId(uniqueDocumentID)
	nodes, err := s.p.ListNodes([]string{})
	c.Assert(err, check.IsNil)
	c.Assert(nodes, check.HasLen, 1)
	c.Assert(nodes[0].Address(), check.Equals, "192.168.99.100")
}

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

func (s *S) TestImageDeploy(c *check.C) {
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
	err := app.CreateApp(a, s.user)
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
