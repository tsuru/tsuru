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
	app := testing.NewFakeApp("cribcaged", "python", 1)
	w := &bytes.Buffer{}
	err := Git(app, w)
	c.Assert(err, gocheck.IsNil)
	expected := make([]string, 3)
	// also ensures execution order
	expected[0] = "git clone git://tsuruhost.com/cribcaged.git test/dir --depth 1" // the command expected to run on the units
	expected[1] = "install deps"
	expected[2] = "restart"
	c.Assert(app.Commands, gocheck.DeepEquals, expected)
}

func (s *S) TestDeployLogsActions(c *gocheck.C) {
	app := testing.NewFakeApp("cribcaged", "python", 1)
	w := &bytes.Buffer{}
	err := Git(app, w)
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
