// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package deploy

import (
	"bytes"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/repository"
	"github.com/globocom/tsuru/testing"
	"launchpad.net/gocheck"
)

func (s *S) TestDeploy(c *gocheck.C) {
	provisioner := testing.NewFakeProvisioner()
	provisioner.PrepareOutput([]byte("cloned"))
	app := testing.NewFakeApp("cribcaged", "python", 1)
	provisioner.Provision(app)
	w := &bytes.Buffer{}
	err := Git(provisioner, app, w)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Commands, gocheck.DeepEquals, []string{"restart"})
	c.Assert(provisioner.InstalledDeps(app), gocheck.Equals, 1)
	cloneCommand := "git clone git://tsuruhost.com/cribcaged.git test/dir --depth 1"
	c.Assert(provisioner.GetCmds(cloneCommand, app), gocheck.HasLen, 1)
}

func (s *S) TestDeployLogsActions(c *gocheck.C) {
	provisioner := testing.NewFakeProvisioner()
	provisioner.PrepareOutput([]byte(""))
	app := testing.NewFakeApp("cribcaged", "python", 1)
	provisioner.Provision(app)
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

func (s *S) TestCloneRepository(c *gocheck.C) {
	p := testing.NewFakeProvisioner()
	p.PrepareOutput([]byte("something"))
	app := testing.NewFakeApp("your", "python", 1)
	out, err := clone(p, app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(out), gocheck.Equals, "something")
	url := repository.ReadOnlyURL(app.GetName())
	path, _ := repository.GetPath()
	expectedCommand := fmt.Sprintf("git clone %s %s --depth 1", url, path)
	c.Assert(p.GetCmds(expectedCommand, app), gocheck.HasLen, 1)
}

func (s *S) TestCloneRepositoryUndefinedPath(c *gocheck.C) {
	old, _ := config.Get("git:unit-repo")
	config.Unset("git:unit-repo")
	defer config.Set("git:unit-repo", old)
	_, err := clone(nil, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `Tsuru is misconfigured: key "git:unit-repo" not found`)
}

func (s *S) TestPullRepository(c *gocheck.C) {
	p := testing.NewFakeProvisioner()
	p.PrepareOutput([]byte("pulled"))
	app := testing.NewFakeApp("your", "python", 1)
	out, err := pull(p, app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(out), gocheck.Equals, "pulled")
	path, _ := repository.GetPath()
	expectedCommand := fmt.Sprintf("cd %s && git pull origin master", path)
	c.Assert(p.GetCmds(expectedCommand, app), gocheck.HasLen, 1)
}

func (s *S) TestPullRepositoryUndefinedPath(c *gocheck.C) {
	old, _ := config.Get("git:unit-repo")
	config.Unset("git:unit-repo")
	defer config.Set("git:unit-repo", old)
	_, err := pull(nil, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `Tsuru is misconfigured: key "git:unit-repo" not found`)
}
