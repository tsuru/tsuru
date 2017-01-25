// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iaas

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestCreateMachineForIaaS(c *check.C) {
	m, err := CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid", "something": "x"})
	c.Assert(err, check.IsNil)
	c.Assert(m.Id, check.Equals, "myid")
	c.Assert(m.Iaas, check.Equals, "test-iaas")
	coll, err := collection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	var dbMachine Machine
	err = coll.Find(bson.M{"_id": "myid"}).One(&dbMachine)
	c.Assert(err, check.IsNil)
	c.Assert(dbMachine.Id, check.Equals, "myid")
	c.Assert(dbMachine.Iaas, check.Equals, "test-iaas")
	c.Assert(dbMachine.CreationParams, check.DeepEquals, map[string]string{
		"id":        "myid",
		"something": "x",
		"should":    "be in",
		"iaas-id":   "myid",
		"iaas":      "test-iaas",
	})
}

func (s *S) TestCreateMachine(c *check.C) {
	config.Set("iaas:default", "test-iaas")
	m, err := CreateMachine(map[string]string{"id": "myid"})
	c.Assert(err, check.IsNil)
	c.Assert(m.Id, check.Equals, "myid")
	c.Assert(m.Iaas, check.Equals, "test-iaas")
	iaas, err := getIaasProvider("test-iaas")
	c.Assert(err, check.IsNil)
	testIaas := iaas.(*TestIaaS)
	c.Assert(testIaas.cmds, check.DeepEquals, []string{"create"})
}

func (s *S) TestCreateMachineDupAddr(c *check.C) {
	config.Set("iaas:default", "test-iaas")
	m, err := CreateMachine(map[string]string{"id": "myid", "address": "addr1"})
	c.Assert(err, check.IsNil)
	c.Assert(m.Id, check.Equals, "myid")
	c.Assert(m.Iaas, check.Equals, "test-iaas")
	c.Assert(m.Address, check.Equals, "addr1.somewhere.com")
	_, err = CreateMachine(map[string]string{"id": "myid2", "address": "addr1"})
	c.Assert(err, check.ErrorMatches, ".*duplicate key error.*")
}

func (s *S) TestCreateMachineEnsureIdx(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	coll := conn.Collection("iaas_machines")
	c.Assert(err, check.IsNil)
	coll.DropIndex("address")
	err = coll.Insert(Machine{Id: "id1", Address: "addr1"}, Machine{Id: "id2", Address: "addr1"})
	c.Assert(err, check.IsNil)
	config.Set("iaas:default", "test-iaas")
	_, err = CreateMachine(map[string]string{"id": "myid", "address": "addr1"})
	c.Assert(err, check.ErrorMatches, "(?s)Could not create index on address for machines collection.*")
	iaas, err := getIaasProvider("test-iaas")
	c.Assert(err, check.IsNil)
	testIaas := iaas.(*TestIaaS)
	c.Assert(testIaas.cmds, check.DeepEquals, []string{"create", "delete"})
}

func (s *S) TestCollectionEnsureIdxDupEntries(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	coll := conn.Collection("iaas_machines")
	c.Assert(err, check.IsNil)
	coll.DropIndex("address")
	err = coll.Insert(Machine{Id: "id1", Address: "addr1"}, Machine{Id: "id2", Address: "addr1"})
	c.Assert(err, check.IsNil)
	collEnsure, err := collectionEnsureIdx()
	c.Assert(err, check.ErrorMatches, `(?s)Could not create index on address for machines collection.*`)
	coll.RemoveAll(nil)
	collEnsure, err = collectionEnsureIdx()
	c.Assert(err, check.IsNil)
	defer collEnsure.Close()
}

func (s *S) TestCreateMachineIaaSInParams(c *check.C) {
	config.Set("iaas:default", "invalid")
	m, err := CreateMachine(map[string]string{"id": "myid", "iaas": "test-iaas"})
	c.Assert(err, check.IsNil)
	c.Assert(m.Id, check.Equals, "myid")
	c.Assert(m.Iaas, check.Equals, "test-iaas")
}

func (s *S) TestCreateMachineWithTemplate(c *check.C) {
	params := map[string]string{
		"id":   "myid",
		"key3": "override3",
		"iaas": "test-iaas",
	}
	m, err := CreateMachine(params)
	c.Assert(err, check.IsNil)
	c.Assert(m.Id, check.Equals, "myid")
	c.Assert(m.Iaas, check.Equals, "test-iaas")
	expected := map[string]string{
		"id":      "myid",
		"key3":    "override3",
		"iaas":    "test-iaas",
		"should":  "be in",
		"iaas-id": "myid",
	}
	c.Assert(m.CreationParams, check.DeepEquals, expected)
	c.Assert(params, check.DeepEquals, expected)
}

func (s *S) TestListMachines(c *check.C) {
	_, err := CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid1"})
	c.Assert(err, check.IsNil)
	_, err = CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid2"})
	c.Assert(err, check.IsNil)
	machines, err := ListMachines()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.HasLen, 2)
	c.Assert(machines[0].Id, check.Equals, "myid1")
	c.Assert(machines[1].Id, check.Equals, "myid2")
}

func (s *S) TestFindMachineByAddress(c *check.C) {
	_, err := CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid1"})
	c.Assert(err, check.IsNil)
	_, err = CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid2"})
	c.Assert(err, check.IsNil)
	machine, err := FindMachineByAddress("myid1.somewhere.com")
	c.Assert(err, check.IsNil)
	c.Assert(machine.Id, check.Equals, "myid1")
	machine, err = FindMachineByAddress("myid2.somewhere.com")
	c.Assert(err, check.IsNil)
	c.Assert(machine.Id, check.Equals, "myid2")
	_, err = FindMachineByAddress("myid3.somewhere.com")
	c.Assert(err, check.Equals, mgo.ErrNotFound)
}

func (s *S) TestDestroy(c *check.C) {
	m, err := CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid1"})
	c.Assert(err, check.IsNil)
	err = m.Destroy()
	c.Assert(err, check.IsNil)
	c.Assert(m.Status, check.Equals, "destroyed")
	machines, err := ListMachines()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.HasLen, 0)
}

func (s *S) TestFindById(c *check.C) {
	_, err := CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid1"})
	c.Assert(err, check.IsNil)
	_, err = CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid2"})
	c.Assert(err, check.IsNil)
	machine, err := FindMachineById("myid1")
	c.Assert(err, check.IsNil)
	c.Assert(machine.Id, check.Equals, "myid1")
	machine, err = FindMachineById("myid2")
	c.Assert(err, check.IsNil)
	c.Assert(machine.Id, check.Equals, "myid2")
	_, err = FindMachineById("myid3")
	c.Assert(err, check.Equals, mgo.ErrNotFound)
}

func (s *S) TestFormatNodeAddress(c *check.C) {
	config.Set("iaas:node-protocol", "https")
	defer config.Unset("iaas:node-protocol")
	defer config.Unset("iaas:node-port")
	config.Unset("iaas:node-port")
	m, err := CreateMachineForIaaS("test-iaas", map[string]string{"id": "myid"})
	c.Assert(err, check.IsNil)
	c.Assert(m.Port, check.Equals, 0)
	addr := m.FormatNodeAddress()
	c.Assert(addr, check.Equals, "https://myid.somewhere.com:2375")
	config.Set("iaas:node-port", 1998)
	addr = m.FormatNodeAddress()
	c.Assert(addr, check.Equals, "https://myid.somewhere.com:1998")
	m.Port = 9123
	addr = m.FormatNodeAddress()
	c.Assert(addr, check.Equals, "https://myid.somewhere.com:9123")
	m.Protocol = "http"
	addr = m.FormatNodeAddress()
	c.Assert(addr, check.Equals, "http://myid.somewhere.com:9123")
}
