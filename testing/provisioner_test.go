// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"bytes"
	"errors"
	"io/ioutil"
	"testing"

	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/provision"
	"launchpad.net/gocheck"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) TestFakeAppAddUnit(c *gocheck.C) {
	app := NewFakeApp("jean", "mj", 0)
	app.AddUnit(provision.Unit{Name: "jean-0"})
	c.Assert(app.units, gocheck.HasLen, 1)
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

func (s *S) TestFakeAppGetMemory(c *gocheck.C) {
	app := NewFakeApp("sou", "otm", 0)
	app.Memory = 100
	c.Assert(app.GetMemory(), gocheck.Equals, int64(100))
}

func (s *S) TestFakeAppGetSwap(c *gocheck.C) {
	app := NewFakeApp("sou", "otm", 0)
	c.Assert(app.GetSwap(), gocheck.Equals, int64(0))
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

func (s *S) TestSetEnvs(c *gocheck.C) {
	app := FakeApp{name: "time"}
	envs := []bind.EnvVar{
		{
			Name:   "http_proxy",
			Value:  "http://theirproxy.com:3128/",
			Public: true,
		},
		{
			Name:   "https_proxy",
			Value:  "http://theirproxy.com:3128/",
			Public: true,
		},
	}
	app.SetEnvs(envs, false, nil)
	expected := map[string]bind.EnvVar{
		"http_proxy": {
			Name:   "http_proxy",
			Value:  "http://theirproxy.com:3128/",
			Public: true,
		},
		"https_proxy": {
			Name:   "https_proxy",
			Value:  "http://theirproxy.com:3128/",
			Public: true,
		},
	}
	c.Assert(app.env, gocheck.DeepEquals, expected)
}

func (s *S) TestGetUnitsReturnUnits(c *gocheck.C) {
	a := NewFakeApp("foo", "static", 1)
	units := a.GetUnits()
	c.Assert(len(units), gocheck.Equals, 1)
}

func (s *S) TestUnsetEnvs(c *gocheck.C) {
	app := FakeApp{name: "time"}
	env := bind.EnvVar{
		Name:   "http_proxy",
		Value:  "http://theirproxy.com:3128/",
		Public: true,
	}
	app.SetEnv(env)
	app.UnsetEnvs([]string{"http_proxy"}, false, nil)
	c.Assert(app.env, gocheck.DeepEquals, map[string]bind.EnvVar{})
}

func (s *S) TestFakeAppBindUnit(c *gocheck.C) {
	var unit provision.Unit
	app := NewFakeApp("sou", "otm", 0)
	err := app.BindUnit(&unit)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.HasBind(&unit), gocheck.Equals, true)
}

func (s *S) TestFakeAppUnbindUnit(c *gocheck.C) {
	var unit provision.Unit
	app := NewFakeApp("sou", "otm", 0)
	err := app.BindUnit(&unit)
	c.Assert(err, gocheck.IsNil)
	err = app.UnbindUnit(&unit)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.HasBind(&unit), gocheck.Equals, false)
}

func (s *S) TestFakeAppUnbindUnitNotBound(c *gocheck.C) {
	var unit provision.Unit
	app := NewFakeApp("sou", "otm", 0)
	err := app.UnbindUnit(&unit)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "not bound")
}

func (s *S) TestFakeAppGetInstances(c *gocheck.C) {
	instance1 := bind.ServiceInstance{Name: "inst1"}
	instance2 := bind.ServiceInstance{Name: "inst2"}
	app := NewFakeApp("sou", "otm", 0)
	app.instances["mysql"] = []bind.ServiceInstance{instance1, instance2}
	instances := app.GetInstances("mysql")
	c.Assert(instances, gocheck.DeepEquals, []bind.ServiceInstance{instance1, instance2})
	instances = app.GetInstances("mongodb")
	c.Assert(instances, gocheck.HasLen, 0)
}

func (s *S) TestFakeAppAddInstance(c *gocheck.C) {
	instance1 := bind.ServiceInstance{Name: "inst1"}
	instance2 := bind.ServiceInstance{Name: "inst2"}
	app := NewFakeApp("sou", "otm", 0)
	err := app.AddInstance("mysql", instance1, nil)
	c.Assert(err, gocheck.IsNil)
	err = app.AddInstance("mongodb", instance2, nil)
	c.Assert(err, gocheck.IsNil)
	instances := app.GetInstances("mysql")
	c.Assert(instances, gocheck.DeepEquals, []bind.ServiceInstance{instance1})
	instances = app.GetInstances("mongodb")
	c.Assert(instances, gocheck.DeepEquals, []bind.ServiceInstance{instance2})
	instances = app.GetInstances("redis")
	c.Assert(instances, gocheck.HasLen, 0)
}

func (s *S) TestFakeAppRemoveInstance(c *gocheck.C) {
	instance1 := bind.ServiceInstance{Name: "inst1"}
	instance2 := bind.ServiceInstance{Name: "inst2"}
	app := NewFakeApp("sou", "otm", 0)
	app.AddInstance("mysql", instance1, nil)
	app.AddInstance("mongodb", instance2, nil)
	err := app.RemoveInstance("mysql", instance1, nil)
	c.Assert(err, gocheck.IsNil)
	instances := app.GetInstances("mysql")
	c.Assert(instances, gocheck.HasLen, 0)
	instances = app.GetInstances("mongodb")
	c.Assert(instances, gocheck.HasLen, 1)
}

func (s *S) TestFakeAppRemoveInstanceNotFound(c *gocheck.C) {
	instance1 := bind.ServiceInstance{Name: "inst1"}
	instance2 := bind.ServiceInstance{Name: "inst2"}
	app := NewFakeApp("sou", "otm", 0)
	app.AddInstance("mysql", instance1, nil)
	err := app.RemoveInstance("mysql", instance2, nil)
	c.Assert(err.Error(), gocheck.Equals, "instance not found")
}

func (s *S) TestFakeAppRemoveInstanceServiceNotFound(c *gocheck.C) {
	instance := bind.ServiceInstance{Name: "inst1"}
	app := NewFakeApp("sou", "otm", 0)
	err := app.RemoveInstance("mysql", instance, nil)
	c.Assert(err.Error(), gocheck.Equals, "instance not found")
}

func (s *S) TestFakeAppLogs(c *gocheck.C) {
	app := NewFakeApp("sou", "otm", 0)
	app.Log("something happened", "[tsuru]", "[api]")
	c.Assert(app.Logs(), gocheck.DeepEquals, []string{"[tsuru][api]something happened"})
}

func (s *S) TestFakeAppHasLog(c *gocheck.C) {
	app := NewFakeApp("sou", "otm", 0)
	app.Log("something happened", "[tsuru]", "[api]")
	c.Assert(app.HasLog("[tsuru]", "[api]", "something happened"), gocheck.Equals, true)
	c.Assert(app.HasLog("tsuru", "api", "something happened"), gocheck.Equals, false)
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

func (s *S) TestStarts(c *gocheck.C) {
	app1 := NewFakeApp("fairy-tale", "shaman", 1)
	app2 := NewFakeApp("unfairy-tale", "shaman", 1)
	p := NewFakeProvisioner()
	p.apps = map[string]provisionedApp{
		app1.GetName(): {app: app1, starts: 10},
		app2.GetName(): {app: app1, starts: 0},
	}
	c.Assert(p.Starts(app1), gocheck.Equals, 10)
	c.Assert(p.Starts(app2), gocheck.Equals, 0)
	c.Assert(p.Starts(NewFakeApp("pride", "shaman", 1)), gocheck.Equals, 0)
}

func (s *S) TestStops(c *gocheck.C) {
	app1 := NewFakeApp("fairy-tale", "shaman", 1)
	app2 := NewFakeApp("unfairy-tale", "shaman", 1)
	p := NewFakeProvisioner()
	p.apps = map[string]provisionedApp{
		app1.GetName(): {app: app1, stops: 10},
		app2.GetName(): {app: app1, stops: 0},
	}
	c.Assert(p.Stops(app1), gocheck.Equals, 10)
	c.Assert(p.Stops(app2), gocheck.Equals, 0)
	c.Assert(p.Stops(NewFakeApp("pride", "shaman", 1)), gocheck.Equals, 0)
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
		{"chain-lighting-0", "chain-lighting", "django", "10.10.10.10", provision.StatusStarted},
		{"chain-lighting-1", "chain-lighting", "django", "10.10.10.15", provision.StatusStarted},
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
	_, err := p.GitDeploy(app, "master", &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.Version(app), gocheck.Equals, "master")
	_, err = p.GitDeploy(app, "1.0", &buf)
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

func (s *S) TestGitDeploy(c *gocheck.C) {
	var buf bytes.Buffer
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	_, err := p.GitDeploy(app, "1.0", &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "Git deploy called")
	c.Assert(p.apps[app.GetName()].version, gocheck.Equals, "1.0")
}

func (s *S) TestGitDeployUnknownApp(c *gocheck.C) {
	var buf bytes.Buffer
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	_, err := p.GitDeploy(app, "1.0", &buf)
	c.Assert(err, gocheck.Equals, errNotProvisioned)
}

func (s *S) TestGitDeployWithPreparedFailure(c *gocheck.C) {
	var buf bytes.Buffer
	err := errors.New("not really")
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("GitDeploy", err)
	_, e := p.GitDeploy(app, "1.0", &buf)
	c.Assert(e, gocheck.NotNil)
	c.Assert(e, gocheck.Equals, err)
}

func (s *S) TestArchiveDeploy(c *gocheck.C) {
	var buf bytes.Buffer
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	_, err := p.ArchiveDeploy(app, "https://s3.amazonaws.com/smt/archive.tar.gz", &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "Archive deploy called")
	c.Assert(p.apps[app.GetName()].lastArchive, gocheck.Equals, "https://s3.amazonaws.com/smt/archive.tar.gz")
}

func (s *S) TestArchiveDeployUnknownApp(c *gocheck.C) {
	var buf bytes.Buffer
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	_, err := p.ArchiveDeploy(app, "https://s3.amazonaws.com/smt/archive.tar.gz", &buf)
	c.Assert(err, gocheck.Equals, errNotProvisioned)
}

func (s *S) TestArchiveDeployWithPreparedFailure(c *gocheck.C) {
	var buf bytes.Buffer
	err := errors.New("not really")
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("ArchiveDeploy", err)
	_, e := p.ArchiveDeploy(app, "https://s3.amazonaws.com/smt/archive.tar.gz", &buf)
	c.Assert(e, gocheck.NotNil)
	c.Assert(e, gocheck.Equals, err)
}

func (s *S) TestUploadDeploy(c *gocheck.C) {
	var buf, input bytes.Buffer
	file := ioutil.NopCloser(&input)
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	_, err := p.UploadDeploy(app, file, &buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "Upload deploy called")
	c.Assert(p.apps[app.GetName()].lastFile, gocheck.Equals, file)
}

func (s *S) TestUploadDeployUnknownApp(c *gocheck.C) {
	var buf bytes.Buffer
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	_, err := p.UploadDeploy(app, nil, &buf)
	c.Assert(err, gocheck.Equals, errNotProvisioned)
}

func (s *S) TestUploadDeployWithPreparedFailure(c *gocheck.C) {
	var buf bytes.Buffer
	err := errors.New("not really")
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("UploadDeploy", err)
	_, e := p.UploadDeploy(app, nil, &buf)
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
	c.Assert(pApp.units, gocheck.HasLen, 0)
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
	err := p.Restart(app, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.Restarts(app), gocheck.Equals, 1)
}

func (s *S) TestStart(c *gocheck.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	err := p.Start(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.Starts(app), gocheck.Equals, 1)
}

func (s *S) TestStop(c *gocheck.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	err := p.Stop(app)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.Stops(app), gocheck.Equals, 1)
}

func (s *S) TestRestartNotProvisioned(c *gocheck.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Restart(app, nil)
	c.Assert(err, gocheck.Equals, errNotProvisioned)
}

func (s *S) TestRestartWithPreparedFailure(c *gocheck.C) {
	app := NewFakeApp("fairy-tale", "shaman", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("Restart", errors.New("Failed to restart."))
	err := p.Restart(app, nil)
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
	units, err := p.AddUnits(app, 2, nil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.GetUnits(app), gocheck.HasLen, 2)
	c.Assert(units, gocheck.HasLen, 2)
}

func (s *S) TestAddUnitsCopiesTheUnitsSlice(c *gocheck.C) {
	app := NewFakeApp("fiction", "python", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	defer p.Destroy(app)
	units, err := p.AddUnits(app, 3, nil)
	c.Assert(err, gocheck.IsNil)
	units[0].Name = "something-else"
	c.Assert(units[0].Name, gocheck.Not(gocheck.Equals), p.GetUnits(app)[1].Name)
}

func (s *S) TestAddZeroUnits(c *gocheck.C) {
	p := NewFakeProvisioner()
	units, err := p.AddUnits(nil, 0, nil)
	c.Assert(units, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Cannot add 0 units.")
}

func (s *S) TestAddUnitsUnprovisionedApp(c *gocheck.C) {
	app := NewFakeApp("mystic-rhythms", "rush", 0)
	p := NewFakeProvisioner()
	units, err := p.AddUnits(app, 1, nil)
	c.Assert(units, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, errNotProvisioned)
}

func (s *S) TestAddUnitsFailure(c *gocheck.C) {
	p := NewFakeProvisioner()
	p.PrepareFailure("AddUnits", errors.New("Cannot add more units."))
	units, err := p.AddUnits(nil, 10, nil)
	c.Assert(units, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Cannot add more units.")
}

func (s *S) TestRemoveUnits(c *gocheck.C) {
	app := NewFakeApp("hemispheres", "rush", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	_, err := p.AddUnits(app, 5, nil)
	c.Assert(err, gocheck.IsNil)
	err = p.RemoveUnits(app, 3)
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.GetUnits(app), gocheck.HasLen, 2)
	c.Assert(p.GetUnits(app)[0].Name, gocheck.Equals, "hemispheres-3")
}

func (s *S) TestRemoveUnitsTooManyUnits(c *gocheck.C) {
	app := NewFakeApp("hemispheres", "rush", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	_, err := p.AddUnits(app, 1, nil)
	c.Assert(err, gocheck.IsNil)
	err = p.RemoveUnits(app, 3)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "too many units to remove")
}

func (s *S) TestRemoveUnitsUnprovisionedApp(c *gocheck.C) {
	app := NewFakeApp("tears", "bruce", 0)
	p := NewFakeProvisioner()
	err := p.RemoveUnits(app, 1)
	c.Assert(err, gocheck.Equals, errNotProvisioned)
}

func (s *S) TestRemoveUnitsFailure(c *gocheck.C) {
	p := NewFakeProvisioner()
	p.PrepareFailure("RemoveUnits", errors.New("This program has performed an illegal operation."))
	err := p.RemoveUnits(nil, 0)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "This program has performed an illegal operation.")
}

func (s *S) TestRemoveUnit(c *gocheck.C) {
	app := NewFakeApp("hemispheres", "rush", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	units, err := p.AddUnits(app, 2, nil)
	c.Assert(err, gocheck.IsNil)
	err = p.RemoveUnit(units[0])
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.GetUnits(app), gocheck.HasLen, 1)
	c.Assert(p.GetUnits(app)[0].Name, gocheck.Equals, "hemispheres-1")
}

func (s *S) TestRemoveUnitNotFound(c *gocheck.C) {
	app := NewFakeApp("hemispheres", "rush", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	units, err := p.AddUnits(app, 2, nil)
	c.Assert(err, gocheck.IsNil)
	err = p.RemoveUnit(provision.Unit{Name: units[0].Name + "wat", AppName: "hemispheres"})
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "unit not found")
}

func (s *S) TestRemoveUnitFromUnprivisionedApp(c *gocheck.C) {
	p := NewFakeProvisioner()
	err := p.RemoveUnit(provision.Unit{AppName: "hemispheres"})
	c.Assert(err, gocheck.Equals, errNotProvisioned)
}

func (s *S) TestRemoveUnitFailure(c *gocheck.C) {
	p := NewFakeProvisioner()
	p.PrepareFailure("RemoveUnit", errors.New("This program has performed an illegal operation."))
	err := p.RemoveUnit(provision.Unit{AppName: "hemispheres"})
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

func (s *S) TestAddr(c *gocheck.C) {
	app := NewFakeApp("quick", "who", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, gocheck.IsNil)
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

func (s *S) TestSetCName(c *gocheck.C) {
	app := NewFakeApp("jean", "mj", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	err := p.SetCName(app, "cname.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(p.apps[app.GetName()].cnames, gocheck.DeepEquals, []string{"cname.com"})
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
	c.Assert(p.apps[app.GetName()].cnames, gocheck.DeepEquals, []string{"cname.com"})
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

func (s *S) TestExtensiblePlatformAdd(c *gocheck.C) {
	p := ExtensibleFakeProvisioner{FakeProvisioner: NewFakeProvisioner()}
	args := map[string]string{"dockerfile": "mydockerfile.txt"}
	err := p.PlatformAdd("python", args, nil)
	c.Assert(err, gocheck.IsNil)
	platform := p.GetPlatform("python")
	c.Assert(platform.Name, gocheck.Equals, "python")
	c.Assert(platform.Version, gocheck.Equals, 1)
	c.Assert(platform.Args, gocheck.DeepEquals, args)
}

func (s *S) TestExtensiblePlatformAddTwice(c *gocheck.C) {
	p := ExtensibleFakeProvisioner{FakeProvisioner: NewFakeProvisioner()}
	args := map[string]string{"dockerfile": "mydockerfile.txt"}
	err := p.PlatformAdd("python", args, nil)
	c.Assert(err, gocheck.IsNil)
	err = p.PlatformAdd("python", nil, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "duplicate platform")
}

func (s *S) TestExtensiblePlatformUpdate(c *gocheck.C) {
	p := ExtensibleFakeProvisioner{FakeProvisioner: NewFakeProvisioner()}
	args := map[string]string{"dockerfile": "mydockerfile.txt"}
	err := p.PlatformAdd("python", args, nil)
	c.Assert(err, gocheck.IsNil)
	args["something"] = "wat"
	err = p.PlatformUpdate("python", args, nil)
	c.Assert(err, gocheck.IsNil)
	platform := p.GetPlatform("python")
	c.Assert(platform.Name, gocheck.Equals, "python")
	c.Assert(platform.Version, gocheck.Equals, 2)
	c.Assert(platform.Args, gocheck.DeepEquals, args)
}

func (s *S) TestExtensiblePlatformUpdateNotFound(c *gocheck.C) {
	p := ExtensibleFakeProvisioner{FakeProvisioner: NewFakeProvisioner()}
	err := p.PlatformUpdate("python", nil, nil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "platform not found")
}

func (s *S) TestExtensiblePlatformRemove(c *gocheck.C) {
	p := ExtensibleFakeProvisioner{FakeProvisioner: NewFakeProvisioner()}
	args := map[string]string{"dockerfile": "mydockerfile.txt"}
	err := p.PlatformAdd("python", args, nil)
	c.Assert(err, gocheck.IsNil)
	err = p.PlatformRemove("python")
	c.Assert(err, gocheck.IsNil)
	platform := p.GetPlatform("python")
	c.Assert(platform, gocheck.IsNil)
}

func (s *S) TestExtensiblePlatformRemoveNotFound(c *gocheck.C) {
	p := ExtensibleFakeProvisioner{FakeProvisioner: NewFakeProvisioner()}
	err := p.PlatformRemove("python")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "platform not found")
}

func (s *S) TestFakeProvisionerAddUnit(c *gocheck.C) {
	app := NewFakeApp("red-sector", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, gocheck.IsNil)
	p.AddUnit(app, provision.Unit{Name: "red-sector/1"})
	c.Assert(p.Units(app), gocheck.HasLen, 1)
	c.Assert(p.apps[app.GetName()].unitLen, gocheck.Equals, 1)
}

func (s *S) TestFakeProvisionerUnits(c *gocheck.C) {
	app := NewFakeApp("red-sector", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, gocheck.IsNil)
	p.AddUnit(app, provision.Unit{Name: "red-sector/1"})
	c.Assert(p.Units(app), gocheck.HasLen, 1)
}

func (s *S) TestFakeProvisionerUnitsAppNotFound(c *gocheck.C) {
	app := NewFakeApp("red-sector", "rush", 1)
	p := NewFakeProvisioner()
	c.Assert(p.Units(app), gocheck.HasLen, 0)
}

func (s *S) TestFakeProvisionerSetUnitStatus(c *gocheck.C) {
	app := NewFakeApp("red-sector", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, gocheck.IsNil)
	unit := provision.Unit{AppName: "red-sector", Name: "red-sector/1", Status: provision.StatusStarted}
	p.AddUnit(app, unit)
	err = p.SetUnitStatus(unit, provision.StatusError)
	c.Assert(err, gocheck.IsNil)
	unit = p.Units(app)[0]
	c.Assert(unit.Status, gocheck.Equals, provision.StatusError)
}

func (s *S) TestFakeProvisionerSetUnitStatusAppNotFound(c *gocheck.C) {
	p := NewFakeProvisioner()
	err := p.SetUnitStatus(provision.Unit{AppName: "something"}, provision.StatusError)
	c.Assert(err, gocheck.Equals, errNotProvisioned)
}

func (s *S) TestFakeProvisionerSetUnitStatusUnitNotFound(c *gocheck.C) {
	app := NewFakeApp("red-sector", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, gocheck.IsNil)
	unit := provision.Unit{AppName: "red-sector", Name: "red-sector/1", Status: provision.StatusStarted}
	err = p.SetUnitStatus(unit, provision.StatusError)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "unit not found")
}

func (s *S) TestFakeProvisionerRegisterUnit(c *gocheck.C) {
	app := NewFakeApp("shine-on", "diamond", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, gocheck.IsNil)
	unit := provision.Unit{AppName: "shine-on", Name: "unit/1"}
	p.AddUnit(app, unit)
	units := p.Units(app)
	ip := units[0].Ip
	err = p.RegisterUnit(unit)
	c.Assert(err, gocheck.IsNil)
	units = p.Units(app)
	c.Assert(units[0].Ip, gocheck.Equals, ip+"-updated")
}

func (s *S) TestFakeProvisionerRegisterUnitNotFound(c *gocheck.C) {
	app := NewFakeApp("shine-on", "diamond", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, gocheck.IsNil)
	unit := provision.Unit{AppName: "shine-on", Name: "unit/1"}
	err = p.RegisterUnit(unit)
	c.Assert(err, gocheck.ErrorMatches, "unit not found")
}
