// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockermachine

import (
	"github.com/tsuru/tsuru/iaas"
	check "gopkg.in/check.v1"
)

func (s *S) TestCloseFake(c *check.C) {
	d, _ := NewFakeDockerMachine(DockerMachineConfig{})
	dm := d.(*FakeDockerMachine)
	c.Assert(dm.closed, check.Equals, false)
	dm.Close()
	c.Assert(dm.closed, check.Equals, true)
}

func (s *S) TestDeleteMachineFake(c *check.C) {
	d, _ := NewFakeDockerMachine(DockerMachineConfig{})
	dm := d.(*FakeDockerMachine)
	m := &iaas.Machine{Id: "my-machine"}
	err := dm.DeleteMachine(m)
	c.Assert(err, check.IsNil)
	c.Assert(dm.deletedMachine, check.DeepEquals, m)
}

func (s *S) TestCreateMachineFake(c *check.C) {
	d, _ := NewFakeDockerMachine(DockerMachineConfig{})
	opts := CreateMachineOpts{
		Name: "my-machine",
		Params: map[string]interface{}{
			"error": "failed",
		},
	}
	m, err := d.CreateMachine(opts)
	c.Assert(err, check.ErrorMatches, "failed")
	c.Assert(m.Base.Id, check.Equals, "my-machine")
}

func (s *S) TestCreateMachineErrorFake(c *check.C) {
	d, _ := NewFakeDockerMachine(DockerMachineConfig{})
	opts := CreateMachineOpts{Name: "my-machine"}
	m, err := d.CreateMachine(opts)
	c.Assert(err, check.IsNil)
	c.Assert(m.Base.Id, check.Equals, "my-machine")
	dm := d.(*FakeDockerMachine)
	c.Assert(dm.createdMachine, check.DeepEquals, m)
	c.Assert(dm.hostOpts, check.DeepEquals, &opts)
}
