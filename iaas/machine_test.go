// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iaas

import (
	"github.com/tsuru/config"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

func (s *S) TestCreateMachineForIaaS(c *gocheck.C) {
	m, err := CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid", "something": "x"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(m.Id, gocheck.Equals, "myid")
	c.Assert(m.Iaas, gocheck.Equals, "test-iaas")
	coll := collection()
	defer coll.Close()
	var dbMachine Machine
	err = coll.Find(bson.M{"_id": "myid"}).One(&dbMachine)
	c.Assert(err, gocheck.IsNil)
	c.Assert(dbMachine.Id, gocheck.Equals, "myid")
	c.Assert(dbMachine.Iaas, gocheck.Equals, "test-iaas")
	c.Assert(dbMachine.CreationParams, gocheck.DeepEquals, map[string]string{
		"id":        "myid",
		"something": "x",
		"should":    "be in",
	})
}

func (s *S) TestCreateMachine(c *gocheck.C) {
	config.Set("iaas:default", "test-iaas")
	m, err := CreateMachine(map[string]string{"id": "myid"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(m.Id, gocheck.Equals, "myid")
	c.Assert(m.Iaas, gocheck.Equals, "test-iaas")
}

func (s *S) TestCreateMachineIaaSInParams(c *gocheck.C) {
	config.Set("iaas:default", "invalid")
	m, err := CreateMachine(map[string]string{"id": "myid", "iaas": "test-iaas"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(m.Id, gocheck.Equals, "myid")
	c.Assert(m.Iaas, gocheck.Equals, "test-iaas")
}

func (s *S) TestCreateMachineWithTemplate(c *gocheck.C) {
	_, err := NewTemplate("tpl1", "test-iaas", map[string]string{
		"key1": "val1",
		"key2": "val2",
		"key3": "val3",
	})
	c.Assert(err, gocheck.IsNil)
	params := map[string]string{
		"id":       "myid",
		"template": "tpl1",
		"key3":     "override3",
	}
	m, err := CreateMachine(params)
	c.Assert(err, gocheck.IsNil)
	c.Assert(m.Id, gocheck.Equals, "myid")
	c.Assert(m.Iaas, gocheck.Equals, "test-iaas")
	expected := map[string]string{
		"id":     "myid",
		"key1":   "val1",
		"key2":   "val2",
		"key3":   "override3",
		"should": "be in",
	}
	c.Assert(m.CreationParams, gocheck.DeepEquals, expected)
	c.Assert(params, gocheck.DeepEquals, expected)
}

func (s *S) TestListMachines(c *gocheck.C) {
	_, err := CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid1"})
	c.Assert(err, gocheck.IsNil)
	_, err = CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid2"})
	c.Assert(err, gocheck.IsNil)
	machines, err := ListMachines()
	c.Assert(err, gocheck.IsNil)
	c.Assert(machines, gocheck.HasLen, 2)
	c.Assert(machines[0].Id, gocheck.Equals, "myid1")
	c.Assert(machines[1].Id, gocheck.Equals, "myid2")
}

func (s *S) TestFindMachineByAddress(c *gocheck.C) {
	_, err := CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid1"})
	c.Assert(err, gocheck.IsNil)
	_, err = CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid2"})
	c.Assert(err, gocheck.IsNil)
	machine, err := FindMachineByAddress("myid1.somewhere.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(machine.Id, gocheck.Equals, "myid1")
	machine, err = FindMachineByAddress("myid2.somewhere.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(machine.Id, gocheck.Equals, "myid2")
	_, err = FindMachineByAddress("myid3.somewhere.com")
	c.Assert(err, gocheck.Equals, mgo.ErrNotFound)
}

func (s *S) TestDestroy(c *gocheck.C) {
	m, err := CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid1"})
	c.Assert(err, gocheck.IsNil)
	err = m.Destroy()
	c.Assert(err, gocheck.IsNil)
	c.Assert(m.Status, gocheck.Equals, "destroyed")
	machines, err := ListMachines()
	c.Assert(err, gocheck.IsNil)
	c.Assert(machines, gocheck.HasLen, 0)
}

func (s *S) TestFindById(c *gocheck.C) {
	_, err := CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid1"})
	c.Assert(err, gocheck.IsNil)
	_, err = CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid2"})
	c.Assert(err, gocheck.IsNil)
	machine, err := FindMachineById("myid1")
	c.Assert(err, gocheck.IsNil)
	c.Assert(machine.Id, gocheck.Equals, "myid1")
	machine, err = FindMachineById("myid2")
	c.Assert(err, gocheck.IsNil)
	c.Assert(machine.Id, gocheck.Equals, "myid2")
	_, err = FindMachineById("myid3")
	c.Assert(err, gocheck.Equals, mgo.ErrNotFound)
}
