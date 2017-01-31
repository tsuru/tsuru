// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iaas

import (
	"testing"

	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (s *S) SetUpTest(c *check.C) {
	config.Unset("iaas")
	config.Set("database:name", "iaas_tests_s")
	iaasProviders = make(map[string]iaasFactory)
	iaasInstances = make(map[string]IaaS)
	RegisterIaasProvider("test-iaas", newTestIaaS)
	coll, err := collection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	coll.RemoveAll(nil)
	tplColl := template_collection()
	defer tplColl.Close()
	tplColl.RemoveAll(nil)
}

func (s *S) TearDownSuite(c *check.C) {
	coll, err := collection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	coll.Database.DropDatabase()
}

type TestIaaS struct {
	cmds []string
}

func (i *TestIaaS) DeleteMachine(m *Machine) error {
	i.cmds = append(i.cmds, "delete")
	m.Status = "destroyed"
	return nil
}

func (i *TestIaaS) CreateMachine(params map[string]string) (*Machine, error) {
	i.cmds = append(i.cmds, "create")
	params["should"] = "be in"
	addr := params["address"]
	if addr == "" {
		addr = params["id"]
	}
	m := Machine{
		Id:      params["id"],
		Status:  "running",
		Address: addr + ".somewhere.com",
	}
	return &m, nil
}

type TestDescriberIaaS struct {
	TestIaaS
}

func (i TestDescriberIaaS) DeleteMachine(m *Machine) error {
	return i.TestIaaS.DeleteMachine(m)
}

func (i TestDescriberIaaS) CreateMachine(params map[string]string) (*Machine, error) {
	return i.TestIaaS.CreateMachine(params)
}

func (i TestDescriberIaaS) Describe() string {
	return "ahoy desc!"
}

type TestCustomizableIaaS struct {
	NamedIaaS
	TestIaaS
}

func (i TestCustomizableIaaS) DeleteMachine(m *Machine) error {
	return i.TestIaaS.DeleteMachine(m)
}

func (i TestCustomizableIaaS) CreateMachine(params map[string]string) (*Machine, error) {
	return i.TestIaaS.CreateMachine(params)
}

type TestHealthCheckerIaaS struct {
	TestIaaS
	err error
}

func (i *TestHealthCheckerIaaS) HealthCheck() error {
	return i.err
}

func newTestHealthcheckIaaS(name string) IaaS {
	return &TestHealthCheckerIaaS{}
}

func newTestCustomizableIaaS(name string) IaaS {
	return TestCustomizableIaaS{NamedIaaS: NamedIaaS{IaaSName: name}}
}

func newTestDescriberIaaS(name string) IaaS {
	return TestDescriberIaaS{}
}

func newTestIaaS(name string) IaaS {
	return &TestIaaS{}
}
