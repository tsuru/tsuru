// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package deploy

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/repository"
	"github.com/globocom/tsuru/testing"
	"launchpad.net/gocheck"
)

func (s *S) TestDeploy(c *gocheck.C) {
	content := `{"git_url": "git://tsuruhost.com/cribcaged.git"}`
	h := &testing.TestHandler{Content: content}
	t := &testing.T{}
	gandalfServer := t.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	provisioner := testing.NewFakeProvisioner()
	provisioner.PrepareOutput([]byte("cloned"))
	provisioner.PrepareOutput([]byte("updated"))
	app := testing.NewFakeApp("cribcaged", "python", 1)
	provisioner.Provision(app)
	w := &bytes.Buffer{}
	err := Git(provisioner, app, "5734f0042844fdeb5bbc1b72b18f2dc1779cade7", w)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Commands, gocheck.DeepEquals, []string{"restart"})
	c.Assert(provisioner.InstalledDeps(app), gocheck.Equals, 1)
	cloneCommand := "git clone git://tsuruhost.com/cribcaged.git test/dir --depth 1"
	c.Assert(provisioner.GetCmds(cloneCommand, app), gocheck.HasLen, 1)
	path, _ := repository.GetPath()
	checkoutCommand := fmt.Sprintf("cd %s && git checkout 5734f0042844fdeb5bbc1b72b18f2dc1779cade7", path)
	c.Assert(provisioner.GetCmds(checkoutCommand, app), gocheck.HasLen, 1)
}

func (s *S) TestDeployLogsActions(c *gocheck.C) {
	h := &testing.TestHandler{}
	t := &testing.T{}
	gandalfServer := t.StartGandalfTestServer(h)
	defer gandalfServer.Close()
	provisioner := testing.NewFakeProvisioner()
	provisioner.PrepareOutput([]byte(""))
	provisioner.PrepareOutput([]byte("updated"))
	app := testing.NewFakeApp("cribcaged", "python", 1)
	provisioner.Provision(app)
	w := &bytes.Buffer{}
	err := Git(provisioner, app, "5734f0042844fdeb5bbc1b72b18f2dc1779cade7", w)
	c.Assert(err, gocheck.IsNil)
	logs := w.String()
	expected := `
 ---> Tsuru receiving push

 ---> Replicating the application repository across units

 ---> Installing dependencies

 ---> Restarting application
Restarting app...
 ---> Deploy done!

`
	c.Assert(logs, gocheck.Equals, expected)
}

func (s *S) TestCloneRepository(c *gocheck.C) {
	h := &testing.TestHandler{}
	t := &testing.T{}
	gandalfServer := t.StartGandalfTestServer(h)
	defer gandalfServer.Close()
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
	out, err := fetch(p, app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(out), gocheck.Equals, "pulled")
	path, _ := repository.GetPath()
	expectedCommand := fmt.Sprintf("cd %s && git fetch origin", path)
	c.Assert(p.GetCmds(expectedCommand, app), gocheck.HasLen, 1)
}

func (s *S) TestPullRepositoryUndefinedPath(c *gocheck.C) {
	old, _ := config.Get("git:unit-repo")
	config.Unset("git:unit-repo")
	defer config.Set("git:unit-repo", old)
	_, err := fetch(nil, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `Tsuru is misconfigured: key "git:unit-repo" not found`)
}

func (s *S) TestCheckout(c *gocheck.C) {
	p := testing.NewFakeProvisioner()
	p.PrepareOutput([]byte("updated"))
	app := testing.NewFakeApp("moon", "python", 1)
	out, err := checkout(p, app, "5734f0042844fdeb5bbc1b72b18f2dc1779cade7")
	c.Assert(err, gocheck.IsNil)
	c.Assert(out, gocheck.IsNil)
	path, _ := repository.GetPath()
	expectedCommand := fmt.Sprintf("cd %s && git checkout 5734f0042844fdeb5bbc1b72b18f2dc1779cade7", path)
	c.Assert(p.GetCmds(expectedCommand, app), gocheck.HasLen, 1)
}

func (s *S) TestCheckoutUndefinedPath(c *gocheck.C) {
	old, _ := config.Get("git:unit-repo")
	config.Unset("git:unit-repo")
	defer config.Set("git:unit-repo", old)
	_, err := checkout(nil, nil, "")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `Tsuru is misconfigured: key "git:unit-repo" not found`)
}

func (s *S) TestCheckoutFailure(c *gocheck.C) {
	p := testing.NewFakeProvisioner()
	p.PrepareOutput([]byte("failed to update"))
	p.PrepareFailure("ExecuteCommand", errors.New("exit status 128"))
	app := testing.NewFakeApp("moon", "python", 1)
	out, err := checkout(p, app, "5734f0042844fdeb5bbc1b72b18f2dc1779cade7")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "exit status 128")
	c.Assert(string(out), gocheck.Equals, "failed to update")
}
