// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"bytes"
	"errors"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/provision"
	"launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) TestFakeAppAddUnit(c *gocheck.C) {
	app := NewFakeApp("jean", "mj", 0)
	app.AddUnit(&FakeUnit{Name: "jean/0"})
	c.Assert(app.units, gocheck.HasLen, 1)
}

func (s *S) TestFakeAppRemoveUnit(c *gocheck.C) {
	app := NewFakeApp("jean", "mk", 0)
	app.AddUnit(&FakeUnit{Name: "jean/0"})
	err := app.RemoveUnit("jean/0")
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.units, gocheck.HasLen, 0)
	err = app.RemoveUnit("jean/0")
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestFakeAppReady(c *gocheck.C) {
	app := NewFakeApp("sou", "otm", 0)
	c.Assert(app.IsReady(), gocheck.Equals, false)
	err := app.Ready()
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.IsReady(), gocheck.Equals, true)
}

func (s *S) TestFakeAppRestart(c *gocheck.C) {
	var buf bytes.Buffer
	app := NewFakeApp("sou", "otm", 0)
	err := app.Restart(&buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "Restarting app...")
}

func (s *S) TestFakeAppSerializeEnvVars(c *gocheck.C) {
	app := NewFakeApp("sou", "otm", 0)
	err := app.SerializeEnvVars()
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Commands, gocheck.DeepEquals, []string{"serialize"})
}

func (s *S) TestEnvs(c *gocheck.C) {
	app := FakeApp{name: "time"}
	env := bind.EnvVar{
		Name:   "http_proxy",
		Value:  "http://theirproxy.com:3128/",
		Public: true,
	}
	app.SetEnv(env)
	envs := map[string]bind.EnvVar{
		"http_proxy": {
			Name:   "http_proxy",
			Value:  "http://theirproxy.com:3128/",
			Public: true,
		},
	}
	c.Assert(envs, gocheck.DeepEquals, app.env)
}

func (s *S) TestFakeAppLogs(c *gocheck.C) {
	app := NewFakeApp("sou", "otm", 0)
	app.Log("something happened", "[tsuru]")
	c.Assert(app.Logs(), gocheck.DeepEquals, []string{"[tsuru]something happened"})
}

func (s *S) TestFakeAppHasLog(c *gocheck.C) {
	app := NewFakeApp("sou", "otm", 0)
	app.Log("something happened", "[tsuru]")
	c.Assert(app.HasLog("[tsuru]", "something happened"), gocheck.Equals, true)
	c.Assert(app.HasLog("tsuru", "something happened"), gocheck.Equals, false)
}

func (s *S) TestProvisioned(c *gocheck.C) {
	app := NewFakeApp("red-sector", "rush", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	c.Assert(p.Provisioned(app), gocheck.Equals, true)
	otherapp := *app
	otherapp.name = "blue-sector"
	c.Assert(p.Provisioned(&otherapp), gocheck.Equals, false)
}

func (s *S) TestRestarts(c *gocheck.C) {
	app1 := NewFakeApp("fairy-tale", "shaman", 1)
	app2 := NewFakeApp("unfairy-tale", "shaman", 1)
	p := NewFakeProvisioner()
	p.apps = map[string]provisionedApp{
		app1.GetName(): {app: app1, restarts: 10},
		app2.GetName(): {app: app1, restarts: 0},
	}
	c.Assert(p.Restarts(app1), gocheck.Equals, 10)
	c.Assert(p.Restarts(app2), gocheck.Equals, 0)
	c.Assert(p.Restarts(NewFakeApp("pride", "shaman", 1)), gocheck.Equals, 0)
}

func (s *S) TestGetCmds(c *gocheck.C) {
	app := NewFakeApp("enemy-within", "rush", 1)
	p := NewFakeProvisioner()
	p.cmds = []Cmd{
		{Cmd: "ls -lh", App: app},
		{Cmd: "ls -lah", App: app},
	}
	c.Assert(p.GetCmds("ls -lh", app), gocheck.HasLen, 1)
	c.Assert(p.GetCmds("l", app), gocheck.HasLen, 0)
	c.Assert(p.GetCmds("", app), gocheck.HasLen, 2)
	otherapp := *app
	otherapp.name = "enemy-without"
	c.Assert(p.GetCmds("ls -lh", &otherapp), gocheck.HasLen, 0)
	c.Assert(p.GetCmds("", &otherapp), gocheck.HasLen, 0)
}

func (s *S) TestGetUnits(c *gocheck.C) {
	list := []provision.Unit{
		{"chain-lighting/0", "chain-lighting", "django", "i-0801", 1, "10.10.10.10", provision.StatusStarted},
		{"chain-lighting/1", "chain-lighting", "django", "i-0802", 2, "10.10.10.15", provision.StatusStarted},
	}
	app := NewFakeApp("chain-lighting", "rush", 1)
	p := NewFakeProvisioner()
	p.apps = map[string]provisionedApp{
		app.GetName(): {app: app, units: list},
	}
	units := p.GetUnits(app)
	c.Assert(units, gocheck.DeepEquals, list)
}

func (s *S) TestVersion(c *gocheck.C) {
	var buf bytes.Buffer
	app := NewFakeApp("free", "matos", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	err := p.Deploy(app, "master", &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.Version(app), gocheck.Equals, "master")
	err = p.Deploy(app, "1.0", &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.Version(app), gocheck.Equals, "1.0")
}

func (s *S) TestPrepareOutput(c *gocheck.C) {
	output := []byte("the body eletric")
	p := NewFakeProvisioner()
	p.PrepareOutput(output)
	got := <-p.outputs
	c.Assert(string(got), gocheck.Equals, string(output))
}

func (s *S) TestPrepareFailure(c *gocheck.C) {
	err := errors.New("the body eletric")
	p := NewFakeProvisioner()
	p.PrepareFailure("Rush", err)
	got := <-p.failures
	c.Assert(got.method, gocheck.Equals, "Rush")
	c.Assert(got.err.Error(), gocheck.Equals, "the body eletric")
}

func (s *S) TestDeploy(c *gocheck.C) {
	var buf bytes.Buffer
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	err := p.Deploy(app, "1.0", &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "Deploy called")
	c.Assert(p.apps[app.GetName()].version, gocheck.Equals, "1.0")
}

func (s *S) TestDeployUnknownApp(c *gocheck.C) {
	var buf bytes.Buffer
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	err := p.Deploy(app, "1.0", &buf)
	c.Assert(err, gocheck.Equals, errNotProvisioned)
}

func (s *S) TestDeployWithPreparedFailure(c *gocheck.C) {
	var buf bytes.Buffer
	err := errors.New("not really")
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("Deploy", err)
	e := p.Deploy(app, "1.0", &buf)
	c.Assert(e, gocheck.NotNil)
	c.Assert(e, gocheck.Equals, err)
}

func (s *S) TestProvision(c *gocheck.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, gocheck.IsNil)
	pApp := p.apps[app.GetName()]
	c.Assert(pApp.app, gocheck.DeepEquals, app)
	c.Assert(pApp.units, gocheck.HasLen, 1)
	expected := provision.Unit{
		Name:       "kid-gloves/0",
		AppName:    "kid-gloves",
		Type:       "rush",
		Status:     provision.StatusStarted,
		InstanceId: "i-080",
		Ip:         "10.10.10.1",
		Machine:    1,
	}
	unit := pApp.units[0]
	c.Assert(unit, gocheck.DeepEquals, expected)
}

func (s *S) TestProvisionWithPreparedFailure(c *gocheck.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("Provision", errors.New("Failed to provision."))
	err := p.Provision(app)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Failed to provision.")
}

func (s *S) TestDoubleProvision(c *gocheck.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, gocheck.IsNil)
	err = p.Provision(app)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "App already provisioned.")
}

func (s *S) TestRestart(c *gocheck.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	err := p.Restart(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.Restarts(app), gocheck.Equals, 1)
}

func (s *S) TestRestartNotProvisioned(c *gocheck.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Restart(app)
	c.Assert(err, gocheck.Equals, errNotProvisioned)
}

func (s *S) TestRestartWithPreparedFailure(c *gocheck.C) {
	app := NewFakeApp("fairy-tale", "shaman", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("Restart", errors.New("Failed to restart."))
	err := p.Restart(app)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Failed to restart.")
}

func (s *S) TestDestroy(c *gocheck.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	err := p.Destroy(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.Provisioned(app), gocheck.Equals, false)
}

func (s *S) TestDestroyWithPreparedFailure(c *gocheck.C) {
	app := NewFakeApp("red-lenses", "rush", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("Destroy", errors.New("Failed to destroy."))
	err := p.Destroy(app)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Failed to destroy.")
}

func (s *S) TestDestroyNotProvisionedApp(c *gocheck.C) {
	app := NewFakeApp("red-lenses", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Destroy(app)
	c.Assert(err, gocheck.Equals, errNotProvisioned)
}

func (s *S) TestAddUnits(c *gocheck.C) {
	app := NewFakeApp("mystic-rhythms", "rush", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	units, err := p.AddUnits(app, 2)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.GetUnits(app), gocheck.HasLen, 3)
	c.Assert(units, gocheck.HasLen, 2)
}

func (s *S) TestAddUnitsCopiesTheUnitsSlice(c *gocheck.C) {
	app := NewFakeApp("fiction", "python", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	defer p.Destroy(app)
	units, err := p.AddUnits(app, 3)
	c.Assert(err, gocheck.IsNil)
	units[0].Name = "something-else"
	c.Assert(units[0].Name, gocheck.Not(gocheck.Equals), p.GetUnits(app)[1].Name)
}

func (s *S) TestAddZeroUnits(c *gocheck.C) {
	p := NewFakeProvisioner()
	units, err := p.AddUnits(nil, 0)
	c.Assert(units, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Cannot add 0 units.")
}

func (s *S) TestAddUnitsUnprovisionedApp(c *gocheck.C) {
	app := NewFakeApp("mystic-rhythms", "rush", 0)
	p := NewFakeProvisioner()
	units, err := p.AddUnits(app, 1)
	c.Assert(units, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, errNotProvisioned)
}

func (s *S) TestAddUnitsFailure(c *gocheck.C) {
	p := NewFakeProvisioner()
	p.PrepareFailure("AddUnits", errors.New("Cannot add more units."))
	units, err := p.AddUnits(nil, 10)
	c.Assert(units, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Cannot add more units.")
}

func (s *S) TestRemoveUnit(c *gocheck.C) {
	app := NewFakeApp("hemispheres", "rush", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	_, err := p.AddUnits(app, 2)
	c.Assert(err, gocheck.IsNil)
	err = p.RemoveUnit(app, "hemispheres/1")
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.GetUnits(app), gocheck.HasLen, 2)
	c.Assert(p.GetUnits(app)[0].Name, gocheck.Equals, "hemispheres/0")
}

func (s *S) TestRemoveUnitFromUnprivisionedApp(c *gocheck.C) {
	app := NewFakeApp("hemispheres", "rush", 0)
	p := NewFakeProvisioner()
	err := p.RemoveUnit(app, "hemispheres/1")
	c.Assert(err, gocheck.Equals, errNotProvisioned)
}

func (s *S) TestRemoveUnknownUnit(c *gocheck.C) {
	app := NewFakeApp("hemispheres", "rush", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	_, err := p.AddUnits(app, 2)
	c.Assert(err, gocheck.IsNil)
	err = p.RemoveUnit(app, "hemispheres/3")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Unit not found.")
}

func (s *S) TestRemoveUnitFailure(c *gocheck.C) {
	p := NewFakeProvisioner()
	p.PrepareFailure("RemoveUnit", errors.New("This program has performed an illegal operation."))
	err := p.RemoveUnit(nil, "hemispheres/5")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "This program has performed an illegal operation.")
}

func (s *S) TestExecuteCommand(c *gocheck.C) {
	var buf bytes.Buffer
	output := []byte("myoutput!")
	app := NewFakeApp("grand-designs", "rush", 2)
	p := NewFakeProvisioner()
	p.PrepareOutput(output)
	p.PrepareOutput(output)
	err := p.ExecuteCommand(&buf, nil, app, "ls", "-l")
	c.Assert(err, gocheck.IsNil)
	cmds := p.GetCmds("ls", app)
	c.Assert(cmds, gocheck.HasLen, 1)
	expected := string(output) + string(output)
	c.Assert(buf.String(), gocheck.Equals, expected)
}

func (s *S) TestExecuteCommandFailureNoOutput(c *gocheck.C) {
	app := NewFakeApp("manhattan-project", "rush", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("ExecuteCommand", errors.New("Failed to run command."))
	err := p.ExecuteCommand(nil, nil, app, "ls", "-l")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Failed to run command.")
}

func (s *S) TestExecuteCommandWithOutputAndFailure(c *gocheck.C) {
	var buf bytes.Buffer
	app := NewFakeApp("marathon", "rush", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("ExecuteCommand", errors.New("Failed to run command."))
	p.PrepareOutput([]byte("myoutput!"))
	err := p.ExecuteCommand(nil, &buf, app, "ls", "-l")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Failed to run command.")
	c.Assert(buf.String(), gocheck.Equals, "myoutput!")
}

func (s *S) TestExecuteComandTimeout(c *gocheck.C) {
	app := NewFakeApp("territories", "rush", 1)
	p := NewFakeProvisioner()
	err := p.ExecuteCommand(nil, nil, app, "ls -l")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "FakeProvisioner timed out waiting for output.")
}

func (s *S) TestCollectStatus(c *gocheck.C) {
	p := NewFakeProvisioner()
	p.apps = map[string]provisionedApp{
		"red-lenses":         {app: NewFakeApp("red-lenses", "rush", 1)},
		"between-the-wheels": {app: NewFakeApp("between-the-wheels", "rush", 1)},
		"the-big-money":      {app: NewFakeApp("the-big-money", "rush", 1)},
		"grand-designs":      {app: NewFakeApp("grand-designs", "rush", 1)},
	}
	expected := []provision.Unit{
		{"red-lenses/0", "red-lenses", "rush", "i-0801", 1, "10.10.10.1", "started"},
		{"between-the-wheels/0", "between-the-wheels", "rush", "i-0802", 2, "10.10.10.2", "started"},
		{"the-big-money/0", "the-big-money", "rush", "i-0803", 3, "10.10.10.3", "started"},
		{"grand-designs/0", "grand-designs", "rush", "i-0804", 4, "10.10.10.4", "started"},
	}
	units, err := p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	c.Assert(units, gocheck.DeepEquals, expected)
}

func (s *S) TestCollectStatusPreparedFailure(c *gocheck.C) {
	p := NewFakeProvisioner()
	p.PrepareFailure("CollectStatus", errors.New("Failed to collect status."))
	units, err := p.CollectStatus()
	c.Assert(units, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Failed to collect status.")
}

func (s *S) TestCollectStatusNoApps(c *gocheck.C) {
	p := NewFakeProvisioner()
	units, err := p.CollectStatus()
	c.Assert(err, gocheck.IsNil)
	c.Assert(units, gocheck.HasLen, 0)
}
func (s *S) TestAddr(c *gocheck.C) {
	app := NewFakeApp("quick", "who", 1)
	p := NewFakeProvisioner()
	addr, err := p.Addr(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, "quick.fake-lb.tsuru.io")
}

func (s *S) TestAddrFailure(c *gocheck.C) {
	p := NewFakeProvisioner()
	p.PrepareFailure("Addr", errors.New("Cannot get addr of this app."))
	addr, err := p.Addr(nil)
	c.Assert(addr, gocheck.Equals, "")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Cannot get addr of this app.")
}

func (s *S) TestInstallDeps(c *gocheck.C) {
	var buf bytes.Buffer
	app := NewFakeApp("alcool", "raul", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	err := p.InstallDeps(app, &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.InstalledDeps(app), gocheck.Equals, 1)
	err = p.InstallDeps(app, &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.InstalledDeps(app), gocheck.Equals, 2)
}

func (s *S) TestInstallDepsNotProvisioned(c *gocheck.C) {
	var buf bytes.Buffer
	app := NewFakeApp("alcool", "raul", 1)
	p := NewFakeProvisioner()
	err := p.InstallDeps(app, &buf)
	c.Assert(err, gocheck.Equals, errNotProvisioned)
}

func (s *S) TestInstallDepsFailure(c *gocheck.C) {
	p := NewFakeProvisioner()
	p.PrepareFailure("InstallDeps", errors.New("Failed to install"))
	err := p.InstallDeps(nil, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Failed to install")
}

func (s *S) TestSetCName(c *gocheck.C) {
	app := NewFakeApp("jean", "mj", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	err := p.SetCName(app, "cname.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.apps[app.GetName()].cname, gocheck.Equals, "cname.com")
}

func (s *S) TestSetCNameNotProvisioned(c *gocheck.C) {
	app := NewFakeApp("jean", "mj", 0)
	p := NewFakeProvisioner()
	err := p.SetCName(app, "cname.com")
	c.Assert(err, gocheck.Equals, errNotProvisioned)
}

func (s *S) TestSetCNameFailure(c *gocheck.C) {
	app := NewFakeApp("jean", "mj", 0)
	p := NewFakeProvisioner()
	p.PrepareFailure("SetCName", errors.New("wut"))
	err := p.SetCName(app, "cname.com")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "wut")
}

func (s *S) TestUnsetCName(c *gocheck.C) {
	app := NewFakeApp("jean", "mj", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	err := p.SetCName(app, "cname.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.apps[app.GetName()].cname, gocheck.Equals, "cname.com")
	err = p.UnsetCName(app, "cname.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.HasCName(app, "cname.com"), gocheck.Equals, false)
}

func (s *S) TestUnsetCNameNotProvisioned(c *gocheck.C) {
	app := NewFakeApp("jean", "mj", 0)
	p := NewFakeProvisioner()
	err := p.UnsetCName(app, "cname.com")
	c.Assert(err, gocheck.Equals, errNotProvisioned)
}

func (s *S) TestUnsetCNameFailure(c *gocheck.C) {
	app := NewFakeApp("jean", "mj", 0)
	p := NewFakeProvisioner()
	p.PrepareFailure("UnsetCName", errors.New("wut"))
	err := p.UnsetCName(app, "cname.com")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "wut")
}

func (s *S) TestHasCName(c *gocheck.C) {
	app := NewFakeApp("jean", "mj", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	err := p.SetCName(app, "cname.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.HasCName(app, "cname.com"), gocheck.Equals, true)
	err = p.UnsetCName(app, "cname.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.HasCName(app, "cname.com"), gocheck.Equals, false)
}

func (s *S) TestCommandableProvisioner(c *gocheck.C) {
	var p CommandableProvisioner
	commands := p.Commands()
	c.Assert(commands, gocheck.HasLen, 1)
	commands2 := p.Commands()
	c.Assert(commands[0], gocheck.Equals, commands2[0])
}

func (s *S) TestExecuteCommandOnce(c *gocheck.C) {
	var buf bytes.Buffer
	output := []byte("myoutput!")
	app := NewFakeApp("grand-designs", "rush", 1)
	p := NewFakeProvisioner()
	p.PrepareOutput(output)
	err := p.ExecuteCommandOnce(&buf, nil, app, "ls", "-l")
	c.Assert(err, gocheck.IsNil)
	cmds := p.GetCmds("ls", app)
	c.Assert(cmds, gocheck.HasLen, 1)
	c.Assert(buf.String(), gocheck.Equals, string(output))
}

func (s *S) TestDeployPipeline(c *gocheck.C) {
	p := FakeProvisioner{}
	c.Assert(p.DeployPipeline(), gocheck.IsNil)
	p.CustomPipeline = true
	c.Assert(p.DeployPipeline(), gocheck.NotNil)
}

func (s *S) TestExecutedPipeline(c *gocheck.C) {
	p := FakeProvisioner{CustomPipeline: true}
	c.Assert(p.ExecutedPipeline(), gocheck.Equals, false)
	pipeline := p.DeployPipeline()
	err := pipeline.Execute()
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.ExecutedPipeline(), gocheck.Equals, true)
}
