// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package deploy

import (
	"bytes"
	"github.com/globocom/tsuru/testing"
	"launchpad.net/gocheck"
)

func (s *S) TestDeploy(c *gocheck.C) {
	provisioner := testing.NewFakeProvisioner()
	app := testing.NewFakeApp("cribcaged", "python", 1)
	w := &bytes.Buffer{}
	err := Git(provisioner, app, w)
	c.Assert(err, gocheck.IsNil)
	expected := []string{
		"git clone git://tsuruhost.com/cribcaged.git test/dir --depth 1",
		"restart",
	}
	c.Assert(app.Commands, gocheck.DeepEquals, expected)
	c.Assert(provisioner.InstalledDeps(app), gocheck.Equals, 1)
}

func (s *S) TestDeployLogsActions(c *gocheck.C) {
	provisioner := testing.NewFakeProvisioner()
	app := testing.NewFakeApp("cribcaged", "python", 1)
	w := &bytes.Buffer{}
	err := Git(provisioner, app, w)
	c.Assert(err, gocheck.IsNil)
	logs := w.String()
	expected := `
 ---> Tsuru receiving push

 ---> Replicating the application repository across units

 ---> Installing dependencies

 ---> Restarting application

 ---> Deploy done!

`
	c.Assert(logs, gocheck.Equals, expected)
}
