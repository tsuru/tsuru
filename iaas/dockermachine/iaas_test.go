// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockermachine

import (
	"fmt"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/iaas"
	check "gopkg.in/check.v1"
)

func (s *S) TestBuildDriverOpts(c *check.C) {
	config.Set("iaas:dockermachine:driver:options", map[interface{}]interface{}{
		"options1": 1,
		"options2": "2",
		"options3": "3",
	})
	defer config.Unset("iaas:dockermachine:driver:options")
	dm := newDockerMachineIaaS("dockermachine")
	driverOpts := dm.(*dockerMachineIaaS).buildDriverOpts("amazonec2", map[string]string{
		"options2": "new2",
	})
	expectedOpts := map[string]interface{}{
		"options1":                      1,
		"options2":                      "new2",
		"options3":                      "3",
		"amazonec2-use-private-address": true,
	}
	c.Assert(driverOpts, check.DeepEquals, expectedOpts)
}

func (s *S) TestCreateMachineIaaS(c *check.C) {
	config.Set("iaas:dockermachine:ca-path", "/etc/ca-path")
	defer config.Unset("iaas:dockermachine:ca-path")
	i := newDockerMachineIaaS("dockermachine")
	dmIaas := i.(*dockerMachineIaaS)
	dmIaas.apiFactory = NewFakeDockerMachine
	m, err := dmIaas.CreateMachine(map[string]string{
		"insecure-registry":  "registry.com",
		"name":               "host-name",
		"driver":             "driver-name",
		"docker-install-url": "http://getdocker.com",
		"docker-flags":       "flag1,flag2",
	})
	expectedMachine := &iaas.Machine{
		Id: "host-name",
		CreationParams: map[string]string{
			"insecure-registry":  "registry.com",
			"driver":             "driver-name",
			"docker-install-url": "http://getdocker.com",
			"docker-flags":       "flag1,flag2",
		},
	}
	c.Assert(err, check.IsNil)
	c.Assert(m, check.DeepEquals, expectedMachine)
	c.Assert(FakeDM.closed, check.Equals, true)
	c.Assert(FakeDM.hostOpts.InsecureRegistry, check.Equals, "registry.com")
	c.Assert(FakeDM.hostOpts.DockerEngineInstallURL, check.Equals, "http://getdocker.com")
	c.Assert(FakeDM.hostOpts.ArbitraryFlags, check.DeepEquals, []string{"flag1", "flag2"})
}

func (s *S) TestCreateMachineIaaSConfigFromIaaSConfig(c *check.C) {
	config.Set("iaas:dockermachine:docker-install-url", "https://getdocker.com")
	config.Set("iaas:dockermachine:ca-path", "/etc/ca-path")
	config.Set("iaas:dockermachine:driver:name", "driver-name")
	config.Set("iaas:dockermachine:driver:user-data-file-param", "driver-userdata")
	config.Set("iaas:dockermachine:docker-flags", "flag1,flag2")
	defer config.Unset("iaas:dockermachine")
	i := newDockerMachineIaaS("dockermachine")
	dmIaas := i.(*dockerMachineIaaS)
	dmIaas.apiFactory = NewFakeDockerMachine
	m, err := dmIaas.CreateMachine(map[string]string{
		"name": "host-name",
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.Id, check.DeepEquals, "host-name")
	c.Assert(m.CreationParams["driver"], check.Equals, "driver-name")
	c.Assert(FakeDM.hostOpts.Params["driver-userdata"], check.Not(check.Equals), "")
	c.Assert(m.CreationParams["driver-userdata"], check.Equals, "")
	c.Assert(FakeDM.hostOpts.DockerEngineInstallURL, check.Equals, "https://getdocker.com")
	c.Assert(FakeDM.hostOpts.ArbitraryFlags, check.DeepEquals, []string{"flag1", "flag2"})
}

func (s *S) TestCreateMachineIaaSFailsWithNoDriver(c *check.C) {
	config.Unset("iaas:dockermachine:driver")
	config.Set("iaas:dockermachine:ca-path", "/etc/ca-path")
	defer config.Unset("iaas:dockermachine:ca-path")
	i := newDockerMachineIaaS("dockermachine")
	dmIaas := i.(*dockerMachineIaaS)
	dmIaas.apiFactory = NewFakeDockerMachine
	m, err := dmIaas.CreateMachine(map[string]string{
		"name": "host-name",
	})
	c.Assert(err, check.Equals, errDriverNotSet)
	c.Assert(m, check.IsNil)
}

func (s *S) TestCreateMachineGeneratesName(c *check.C) {
	i := newDockerMachineIaaS("dockermachine")
	dmIaas := i.(*dockerMachineIaaS)
	dmIaas.apiFactory = NewFakeDockerMachine
	m, err := dmIaas.CreateMachine(map[string]string{
		"pool":   "theonepool",
		"driver": "driver-name",
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.Id, check.Matches, fmt.Sprintf("theonepool-.*"))
}

func (s *S) TestCreateMachineDeletesMachineWithError(c *check.C) {
	config.Set("iaas:dockermachine:ca-path", "/etc/ca-path")
	defer config.Unset("iaas:dockermachine:ca-path")
	i := newDockerMachineIaaS("dockermachine")
	dmIaas := i.(*dockerMachineIaaS)
	dmIaas.apiFactory = NewFakeDockerMachine
	m, err := dmIaas.CreateMachine(map[string]string{
		"pool":   "theonepool",
		"driver": "driver-name",
		"name":   "my-machine",
		"error":  "failed to create",
	})
	c.Assert(err, check.NotNil)
	c.Assert(m, check.IsNil)
	c.Assert(FakeDM.deletedMachine.Id, check.Equals, "my-machine")
}

func (s *S) TestDeleteMachineIaaS(c *check.C) {
	i := newDockerMachineIaaS("dockermachine")
	dmIaas := i.(*dockerMachineIaaS)
	dmIaas.apiFactory = NewFakeDockerMachine
	m := &iaas.Machine{Id: "machine-id"}
	err := dmIaas.DeleteMachine(m)
	c.Assert(err, check.IsNil)
	c.Assert(FakeDM.deletedMachine, check.DeepEquals, m)
	c.Assert(FakeDM.closed, check.Equals, true)
}
