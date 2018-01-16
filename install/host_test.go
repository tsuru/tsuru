// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package install

import (
	"github.com/globalsign/mgo/bson"
	check "gopkg.in/check.v1"
)

func (s *S) TestAddHost(c *check.C) {
	host := &Host{Name: "my-host", DriverName: "amazonec2", Driver: make(map[string]interface{})}
	err := AddHost(host)
	c.Assert(err, check.IsNil)
	var h *Host
	err = s.conn.InstallHosts().Find(bson.M{"name": "my-host"}).One(&h)
	c.Assert(err, check.IsNil)
	c.Assert(h, check.DeepEquals, host)
}

func (s *S) TestAddHostAlreadyExistsReturnsError(c *check.C) {
	host := &Host{Name: "my-host", DriverName: "amazonec2", Driver: make(map[string]interface{})}
	err := AddHost(host)
	c.Assert(err, check.IsNil)
	err = AddHost(host)
	c.Assert(err, check.DeepEquals, &ErrHostAlreadyExists{Name: "my-host"})
}

func (s *S) TestGetHostByName(c *check.C) {
	host := &Host{Name: "my-host", DriverName: "amazonec2", Driver: make(map[string]interface{})}
	err := AddHost(host)
	c.Assert(err, check.IsNil)
	h, err := GetHostByName("my-host")
	c.Assert(err, check.IsNil)
	c.Assert(h, check.DeepEquals, host)
}

func (s *S) TestGetHostByNameReturnsErrorWhenHostDoesNotExist(c *check.C) {
	h, err := GetHostByName("unknow-host")
	c.Assert(err, check.DeepEquals, &ErrHostNotFound{Name: "unknow-host"})
	c.Assert(h, check.IsNil)
}

func (s *S) TestListHosts(c *check.C) {
	host1 := &Host{Name: "my-host-1", DriverName: "amazonec2", Driver: make(map[string]interface{})}
	host2 := &Host{Name: "my-host-2", DriverName: "amazonec2", Driver: make(map[string]interface{})}
	err := AddHost(host1)
	c.Assert(err, check.IsNil)
	err = AddHost(host2)
	c.Assert(err, check.IsNil)
	hosts, err := ListHosts()
	c.Assert(err, check.IsNil)
	c.Assert(hosts, check.DeepEquals, []*Host{host1, host2})
}
