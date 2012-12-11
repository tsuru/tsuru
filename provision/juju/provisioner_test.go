// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/globocom/commandmocker"
	"github.com/globocom/tsuru/provision"
	. "launchpad.net/gocheck"
)

func (s *S) TestShouldBeRegistered(c *C) {
	p, err := provision.Get("juju")
	c.Assert(err, IsNil)
	c.Assert(p, FitsTypeOf, &JujuProvisioner{})
}

func (s *S) TestJujuProvision(c *C) {
	tmpdir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := NewFakeApp("trace", "python", 0)
	p := JujuProvisioner{}
	err = p.Provision(app)
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	c.Assert(commandmocker.Output(tmpdir), Equals, "deploy --repository /home/charms local:python trace")
}

func (s *S) TestJujuProvisionFailure(c *C) {
	tmpdir, err := commandmocker.Error("juju", "juju failed", 1)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := NewFakeApp("trace", "python", 0)
	p := JujuProvisioner{}
	pErr := p.Provision(app)
	c.Assert(pErr, NotNil)
	c.Assert(pErr.Reason, Equals, "juju failed")
	c.Assert(pErr.Err.Error(), Equals, "exit status 1")
}

func (s *S) TestJujuDestroy(c *C) {
	tmpdir, err := commandmocker.Add("juju", "$*")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := NewFakeApp("cribcaged", "python", 3)
	p := JujuProvisioner{}
	err = p.Destroy(app)
	c.Assert(err, IsNil)
	c.Assert(commandmocker.Ran(tmpdir), Equals, true)
	output := "destroy-service cribcagedterminate-machine 1"
	output += "terminate-machine 2terminate-machine 3"
	c.Assert(commandmocker.Output(tmpdir), Equals, output)
}

func (s *S) TestJujuDestroyFailure(c *C) {
	tmpdir, err := commandmocker.Error("juju", "juju failed to destroy the machine", 25)
	c.Assert(err, IsNil)
	defer commandmocker.Remove(tmpdir)
	app := NewFakeApp("idioglossia", "static", 1)
	p := JujuProvisioner{}
	pErr := p.Destroy(app)
	c.Assert(pErr, NotNil)
	c.Assert(pErr.Reason, Equals, "juju failed to destroy the machine")
	c.Assert(pErr.Err.Error(), Equals, "exit status 25")
}
