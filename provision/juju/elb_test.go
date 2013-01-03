// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	. "launchpad.net/gocheck"
)

type ELBSuite struct{}

var _ = Suite(&ELBSuite{})

func (s *ELBSuite) SetUpSuite(c *C) {
	var err error
	db.Session, err = db.Open("127.0.0.1:27017", "juju_tests")
	c.Assert(err, IsNil)
}

func (s *ELBSuite) TearDownSuite(c *C) {
	db.Session.Close()
}

func (s *ELBSuite) TestGetCollection(c *C) {
	old, err := config.Get("juju:elb-collection")
	if err == nil {
		defer config.Set("juju:elb-collection", old)
	}
	name := "juju_test_elbs"
	config.Set("juju:elb-collection", name)
	manager := ELBManager{}
	coll := manager.collection()
	other := db.Session.Collection(name)
	c.Assert(coll, DeepEquals, other)
}
