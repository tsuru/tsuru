// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provisiontest

import (
	"bytes"
	"errors"
	"io/ioutil"
	"testing"

	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type S struct{}

var _ = check.Suite(&S{})

func (s *S) TestFakeAppAddUnit(c *check.C) {
	app := NewFakeApp("jean", "mj", 0)
	app.AddUnit(provision.Unit{Name: "jean-0"})
	c.Assert(app.units, check.HasLen, 1)
}

func (s *S) TestFakeAppRestart(c *check.C) {
	var buf bytes.Buffer
	app := NewFakeApp("sou", "otm", 0)
	err := app.Restart(&buf)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "Restarting app...")
}

func (s *S) TestFakeAppGetMemory(c *check.C) {
	app := NewFakeApp("sou", "otm", 0)
	app.Memory = 100
	c.Assert(app.GetMemory(), check.Equals, int64(100))
}

func (s *S) TestFakeAppGetSwap(c *check.C) {
	app := NewFakeApp("sou", "otm", 0)
	c.Assert(app.GetSwap(), check.Equals, int64(0))
}

func (s *S) TestFakeAppSerializeEnvVars(c *check.C) {
	app := NewFakeApp("sou", "otm", 0)
	err := app.SerializeEnvVars()
	c.Assert(err, check.IsNil)
	c.Assert(app.Commands, check.DeepEquals, []string{"serialize"})
}

func (s *S) TestEnvs(c *check.C) {
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
	c.Assert(envs, check.DeepEquals, app.env)
}

func (s *S) TestSetEnvs(c *check.C) {
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
	c.Assert(app.env, check.DeepEquals, expected)
}

func (s *S) TestGetUnitsReturnUnits(c *check.C) {
	a := NewFakeApp("foo", "static", 1)
	units := a.GetUnits()
	c.Assert(len(units), check.Equals, 1)
}

func (s *S) TestUnsetEnvs(c *check.C) {
	app := FakeApp{name: "time"}
	env := bind.EnvVar{
		Name:   "http_proxy",
		Value:  "http://theirproxy.com:3128/",
		Public: true,
	}
	app.SetEnv(env)
	app.UnsetEnvs([]string{"http_proxy"}, false, nil)
	c.Assert(app.env, check.DeepEquals, map[string]bind.EnvVar{})
}

func (s *S) TestFakeAppBindUnit(c *check.C) {
	var unit provision.Unit
	app := NewFakeApp("sou", "otm", 0)
	err := app.BindUnit(&unit)
	c.Assert(err, check.IsNil)
	c.Assert(app.HasBind(&unit), check.Equals, true)
}

func (s *S) TestFakeAppUnbindUnit(c *check.C) {
	var unit provision.Unit
	app := NewFakeApp("sou", "otm", 0)
	err := app.BindUnit(&unit)
	c.Assert(err, check.IsNil)
	err = app.UnbindUnit(&unit)
	c.Assert(err, check.IsNil)
	c.Assert(app.HasBind(&unit), check.Equals, false)
}

func (s *S) TestFakeAppUnbindUnitNotBound(c *check.C) {
	var unit provision.Unit
	app := NewFakeApp("sou", "otm", 0)
	err := app.UnbindUnit(&unit)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "not bound")
}

func (s *S) TestFakeAppGetInstances(c *check.C) {
	instance1 := bind.ServiceInstance{Name: "inst1"}
	instance2 := bind.ServiceInstance{Name: "inst2"}
	app := NewFakeApp("sou", "otm", 0)
	app.instances["mysql"] = []bind.ServiceInstance{instance1, instance2}
	instances := app.GetInstances("mysql")
	c.Assert(instances, check.DeepEquals, []bind.ServiceInstance{instance1, instance2})
	instances = app.GetInstances("mongodb")
	c.Assert(instances, check.HasLen, 0)
}

func (s *S) TestFakeAppAddInstance(c *check.C) {
	instance1 := bind.ServiceInstance{Name: "inst1"}
	instance2 := bind.ServiceInstance{Name: "inst2"}
	app := NewFakeApp("sou", "otm", 0)
	err := app.AddInstance("mysql", instance1, nil)
	c.Assert(err, check.IsNil)
	err = app.AddInstance("mongodb", instance2, nil)
	c.Assert(err, check.IsNil)
	instances := app.GetInstances("mysql")
	c.Assert(instances, check.DeepEquals, []bind.ServiceInstance{instance1})
	instances = app.GetInstances("mongodb")
	c.Assert(instances, check.DeepEquals, []bind.ServiceInstance{instance2})
	instances = app.GetInstances("redis")
	c.Assert(instances, check.HasLen, 0)
}

func (s *S) TestFakeAppRemoveInstance(c *check.C) {
	instance1 := bind.ServiceInstance{Name: "inst1"}
	instance2 := bind.ServiceInstance{Name: "inst2"}
	app := NewFakeApp("sou", "otm", 0)
	app.AddInstance("mysql", instance1, nil)
	app.AddInstance("mongodb", instance2, nil)
	err := app.RemoveInstance("mysql", instance1, nil)
	c.Assert(err, check.IsNil)
	instances := app.GetInstances("mysql")
	c.Assert(instances, check.HasLen, 0)
	instances = app.GetInstances("mongodb")
	c.Assert(instances, check.HasLen, 1)
}

func (s *S) TestFakeAppRemoveInstanceNotFound(c *check.C) {
	instance1 := bind.ServiceInstance{Name: "inst1"}
	instance2 := bind.ServiceInstance{Name: "inst2"}
	app := NewFakeApp("sou", "otm", 0)
	app.AddInstance("mysql", instance1, nil)
	err := app.RemoveInstance("mysql", instance2, nil)
	c.Assert(err.Error(), check.Equals, "instance not found")
}

func (s *S) TestFakeAppRemoveInstanceServiceNotFound(c *check.C) {
	instance := bind.ServiceInstance{Name: "inst1"}
	app := NewFakeApp("sou", "otm", 0)
	err := app.RemoveInstance("mysql", instance, nil)
	c.Assert(err.Error(), check.Equals, "instance not found")
}

func (s *S) TestFakeAppLogs(c *check.C) {
	app := NewFakeApp("sou", "otm", 0)
	app.Log("something happened", "[tsuru]", "[api]")
	c.Assert(app.Logs(), check.DeepEquals, []string{"[tsuru][api]something happened"})
}

func (s *S) TestFakeAppHasLog(c *check.C) {
	app := NewFakeApp("sou", "otm", 0)
	app.Log("something happened", "[tsuru]", "[api]")
	c.Assert(app.HasLog("[tsuru]", "[api]", "something happened"), check.Equals, true)
	c.Assert(app.HasLog("tsuru", "api", "something happened"), check.Equals, false)
}

func (s *S) TestProvisioned(c *check.C) {
	app := NewFakeApp("red-sector", "rush", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	c.Assert(p.Provisioned(app), check.Equals, true)
	otherapp := *app
	otherapp.name = "blue-sector"
	c.Assert(p.Provisioned(&otherapp), check.Equals, false)
}

func (s *S) TestRestarts(c *check.C) {
	app1 := NewFakeApp("fairy-tale", "shaman", 1)
	app2 := NewFakeApp("unfairy-tale", "shaman", 1)
	p := NewFakeProvisioner()
	p.apps = map[string]provisionedApp{
		app1.GetName(): {app: app1, restarts: 10},
		app2.GetName(): {app: app1, restarts: 0},
	}
	c.Assert(p.Restarts(app1), check.Equals, 10)
	c.Assert(p.Restarts(app2), check.Equals, 0)
	c.Assert(p.Restarts(NewFakeApp("pride", "shaman", 1)), check.Equals, 0)
}

func (s *S) TestStarts(c *check.C) {
	app1 := NewFakeApp("fairy-tale", "shaman", 1)
	app2 := NewFakeApp("unfairy-tale", "shaman", 1)
	p := NewFakeProvisioner()
	p.apps = map[string]provisionedApp{
		app1.GetName(): {app: app1, starts: 10},
		app2.GetName(): {app: app1, starts: 0},
	}
	c.Assert(p.Starts(app1), check.Equals, 10)
	c.Assert(p.Starts(app2), check.Equals, 0)
	c.Assert(p.Starts(NewFakeApp("pride", "shaman", 1)), check.Equals, 0)
}

func (s *S) TestStops(c *check.C) {
	app1 := NewFakeApp("fairy-tale", "shaman", 1)
	app2 := NewFakeApp("unfairy-tale", "shaman", 1)
	p := NewFakeProvisioner()
	p.apps = map[string]provisionedApp{
		app1.GetName(): {app: app1, stops: 10},
		app2.GetName(): {app: app1, stops: 0},
	}
	c.Assert(p.Stops(app1), check.Equals, 10)
	c.Assert(p.Stops(app2), check.Equals, 0)
	c.Assert(p.Stops(NewFakeApp("pride", "shaman", 1)), check.Equals, 0)
}

func (s *S) TestGetCmds(c *check.C) {
	app := NewFakeApp("enemy-within", "rush", 1)
	p := NewFakeProvisioner()
	p.cmds = []Cmd{
		{Cmd: "ls -lh", App: app},
		{Cmd: "ls -lah", App: app},
	}
	c.Assert(p.GetCmds("ls -lh", app), check.HasLen, 1)
	c.Assert(p.GetCmds("l", app), check.HasLen, 0)
	c.Assert(p.GetCmds("", app), check.HasLen, 2)
	otherapp := *app
	otherapp.name = "enemy-without"
	c.Assert(p.GetCmds("ls -lh", &otherapp), check.HasLen, 0)
	c.Assert(p.GetCmds("", &otherapp), check.HasLen, 0)
}

func (s *S) TestGetUnits(c *check.C) {
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
	c.Assert(units, check.DeepEquals, list)
}

func (s *S) TestVersion(c *check.C) {
	var buf bytes.Buffer
	app := NewFakeApp("free", "matos", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	_, err := p.GitDeploy(app, "master", &buf)
	c.Assert(err, check.IsNil)
	c.Assert(p.Version(app), check.Equals, "master")
	_, err = p.GitDeploy(app, "1.0", &buf)
	c.Assert(err, check.IsNil)
	c.Assert(p.Version(app), check.Equals, "1.0")
}

func (s *S) TestPrepareOutput(c *check.C) {
	output := []byte("the body eletric")
	p := NewFakeProvisioner()
	p.PrepareOutput(output)
	got := <-p.outputs
	c.Assert(string(got), check.Equals, string(output))
}

func (s *S) TestPrepareFailure(c *check.C) {
	err := errors.New("the body eletric")
	p := NewFakeProvisioner()
	p.PrepareFailure("Rush", err)
	got := <-p.failures
	c.Assert(got.method, check.Equals, "Rush")
	c.Assert(got.err.Error(), check.Equals, "the body eletric")
}

func (s *S) TestGitDeploy(c *check.C) {
	var buf bytes.Buffer
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	_, err := p.GitDeploy(app, "1.0", &buf)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "Git deploy called")
	c.Assert(p.apps[app.GetName()].version, check.Equals, "1.0")
}

func (s *S) TestGitDeployUnknownApp(c *check.C) {
	var buf bytes.Buffer
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	_, err := p.GitDeploy(app, "1.0", &buf)
	c.Assert(err, check.Equals, errNotProvisioned)
}

func (s *S) TestGitDeployWithPreparedFailure(c *check.C) {
	var buf bytes.Buffer
	err := errors.New("not really")
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("GitDeploy", err)
	_, e := p.GitDeploy(app, "1.0", &buf)
	c.Assert(e, check.NotNil)
	c.Assert(e, check.Equals, err)
}

func (s *S) TestArchiveDeploy(c *check.C) {
	var buf bytes.Buffer
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	_, err := p.ArchiveDeploy(app, "https://s3.amazonaws.com/smt/archive.tar.gz", &buf)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "Archive deploy called")
	c.Assert(p.apps[app.GetName()].lastArchive, check.Equals, "https://s3.amazonaws.com/smt/archive.tar.gz")
}

func (s *S) TestArchiveDeployUnknownApp(c *check.C) {
	var buf bytes.Buffer
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	_, err := p.ArchiveDeploy(app, "https://s3.amazonaws.com/smt/archive.tar.gz", &buf)
	c.Assert(err, check.Equals, errNotProvisioned)
}

func (s *S) TestArchiveDeployWithPreparedFailure(c *check.C) {
	var buf bytes.Buffer
	err := errors.New("not really")
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("ArchiveDeploy", err)
	_, e := p.ArchiveDeploy(app, "https://s3.amazonaws.com/smt/archive.tar.gz", &buf)
	c.Assert(e, check.NotNil)
	c.Assert(e, check.Equals, err)
}

func (s *S) TestUploadDeploy(c *check.C) {
	var buf, input bytes.Buffer
	file := ioutil.NopCloser(&input)
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	_, err := p.UploadDeploy(app, file, &buf)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "Upload deploy called")
	c.Assert(p.apps[app.GetName()].lastFile, check.Equals, file)
}

func (s *S) TestUploadDeployUnknownApp(c *check.C) {
	var buf bytes.Buffer
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	_, err := p.UploadDeploy(app, nil, &buf)
	c.Assert(err, check.Equals, errNotProvisioned)
}

func (s *S) TestUploadDeployWithPreparedFailure(c *check.C) {
	var buf bytes.Buffer
	err := errors.New("not really")
	app := NewFakeApp("soul", "arch", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("UploadDeploy", err)
	_, e := p.UploadDeploy(app, nil, &buf)
	c.Assert(e, check.NotNil)
	c.Assert(e, check.Equals, err)
}

func (s *S) TestProvision(c *check.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, check.IsNil)
	pApp := p.apps[app.GetName()]
	c.Assert(pApp.app, check.DeepEquals, app)
	c.Assert(pApp.units, check.HasLen, 0)
}

func (s *S) TestProvisionWithPreparedFailure(c *check.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("Provision", errors.New("Failed to provision."))
	err := p.Provision(app)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to provision.")
}

func (s *S) TestDoubleProvision(c *check.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, check.IsNil)
	err = p.Provision(app)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "App already provisioned.")
}

func (s *S) TestRestart(c *check.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	err := p.Restart(app, nil)
	c.Assert(err, check.IsNil)
	c.Assert(p.Restarts(app), check.Equals, 1)
}

func (s *S) TestStart(c *check.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	err := p.Start(app)
	c.Assert(err, check.IsNil)
	c.Assert(p.Starts(app), check.Equals, 1)
}

func (s *S) TestStop(c *check.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	err := p.Stop(app)
	c.Assert(err, check.IsNil)
	c.Assert(p.Stops(app), check.Equals, 1)
}

func (s *S) TestRestartNotProvisioned(c *check.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Restart(app, nil)
	c.Assert(err, check.Equals, errNotProvisioned)
}

func (s *S) TestRestartWithPreparedFailure(c *check.C) {
	app := NewFakeApp("fairy-tale", "shaman", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("Restart", errors.New("Failed to restart."))
	err := p.Restart(app, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to restart.")
}

func (s *S) TestDestroy(c *check.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	p.Provision(app)
	err := p.Destroy(app)
	c.Assert(err, check.IsNil)
	c.Assert(p.Provisioned(app), check.Equals, false)
}

func (s *S) TestDestroyWithPreparedFailure(c *check.C) {
	app := NewFakeApp("red-lenses", "rush", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("Destroy", errors.New("Failed to destroy."))
	err := p.Destroy(app)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to destroy.")
}

func (s *S) TestDestroyNotProvisionedApp(c *check.C) {
	app := NewFakeApp("red-lenses", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Destroy(app)
	c.Assert(err, check.Equals, errNotProvisioned)
}

func (s *S) TestAddUnits(c *check.C) {
	app := NewFakeApp("mystic-rhythms", "rush", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	units, err := p.AddUnits(app, 2, nil)
	c.Assert(err, check.IsNil)
	c.Assert(p.GetUnits(app), check.HasLen, 2)
	c.Assert(units, check.HasLen, 2)
}

func (s *S) TestAddUnitsCopiesTheUnitsSlice(c *check.C) {
	app := NewFakeApp("fiction", "python", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	defer p.Destroy(app)
	units, err := p.AddUnits(app, 3, nil)
	c.Assert(err, check.IsNil)
	units[0].Name = "something-else"
	c.Assert(units[0].Name, check.Not(check.Equals), p.GetUnits(app)[1].Name)
}

func (s *S) TestAddZeroUnits(c *check.C) {
	p := NewFakeProvisioner()
	units, err := p.AddUnits(nil, 0, nil)
	c.Assert(units, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot add 0 units.")
}

func (s *S) TestAddUnitsUnprovisionedApp(c *check.C) {
	app := NewFakeApp("mystic-rhythms", "rush", 0)
	p := NewFakeProvisioner()
	units, err := p.AddUnits(app, 1, nil)
	c.Assert(units, check.IsNil)
	c.Assert(err, check.Equals, errNotProvisioned)
}

func (s *S) TestAddUnitsFailure(c *check.C) {
	p := NewFakeProvisioner()
	p.PrepareFailure("AddUnits", errors.New("Cannot add more units."))
	units, err := p.AddUnits(nil, 10, nil)
	c.Assert(units, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot add more units.")
}

func (s *S) TestRemoveUnits(c *check.C) {
	app := NewFakeApp("hemispheres", "rush", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	_, err := p.AddUnits(app, 5, nil)
	c.Assert(err, check.IsNil)
	err = p.RemoveUnits(app, 3)
	c.Assert(err, check.IsNil)
	c.Assert(p.GetUnits(app), check.HasLen, 2)
	c.Assert(p.GetUnits(app)[0].Name, check.Equals, "hemispheres-3")
}

func (s *S) TestRemoveUnitsTooManyUnits(c *check.C) {
	app := NewFakeApp("hemispheres", "rush", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	_, err := p.AddUnits(app, 1, nil)
	c.Assert(err, check.IsNil)
	err = p.RemoveUnits(app, 3)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "too many units to remove")
}

func (s *S) TestRemoveUnitsUnprovisionedApp(c *check.C) {
	app := NewFakeApp("tears", "bruce", 0)
	p := NewFakeProvisioner()
	err := p.RemoveUnits(app, 1)
	c.Assert(err, check.Equals, errNotProvisioned)
}

func (s *S) TestRemoveUnitsFailure(c *check.C) {
	p := NewFakeProvisioner()
	p.PrepareFailure("RemoveUnits", errors.New("This program has performed an illegal operation."))
	err := p.RemoveUnits(nil, 0)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "This program has performed an illegal operation.")
}

func (s *S) TestRemoveUnit(c *check.C) {
	app := NewFakeApp("hemispheres", "rush", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	units, err := p.AddUnits(app, 2, nil)
	c.Assert(err, check.IsNil)
	err = p.RemoveUnit(units[0])
	c.Assert(err, check.IsNil)
	c.Assert(p.GetUnits(app), check.HasLen, 1)
	c.Assert(p.GetUnits(app)[0].Name, check.Equals, "hemispheres-1")
}

func (s *S) TestRemoveUnitNotFound(c *check.C) {
	app := NewFakeApp("hemispheres", "rush", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	units, err := p.AddUnits(app, 2, nil)
	c.Assert(err, check.IsNil)
	err = p.RemoveUnit(provision.Unit{Name: units[0].Name + "wat", AppName: "hemispheres"})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "unit not found")
}

func (s *S) TestRemoveUnitFromUnprivisionedApp(c *check.C) {
	p := NewFakeProvisioner()
	err := p.RemoveUnit(provision.Unit{AppName: "hemispheres"})
	c.Assert(err, check.Equals, errNotProvisioned)
}

func (s *S) TestRemoveUnitFailure(c *check.C) {
	p := NewFakeProvisioner()
	p.PrepareFailure("RemoveUnit", errors.New("This program has performed an illegal operation."))
	err := p.RemoveUnit(provision.Unit{AppName: "hemispheres"})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "This program has performed an illegal operation.")
}

func (s *S) TestExecuteCommand(c *check.C) {
	var buf bytes.Buffer
	output := []byte("myoutput!")
	app := NewFakeApp("grand-designs", "rush", 2)
	p := NewFakeProvisioner()
	p.PrepareOutput(output)
	p.PrepareOutput(output)
	err := p.ExecuteCommand(&buf, nil, app, "ls", "-l")
	c.Assert(err, check.IsNil)
	cmds := p.GetCmds("ls", app)
	c.Assert(cmds, check.HasLen, 1)
	expected := string(output) + string(output)
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestExecuteCommandFailureNoOutput(c *check.C) {
	app := NewFakeApp("manhattan-project", "rush", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("ExecuteCommand", errors.New("Failed to run command."))
	err := p.ExecuteCommand(nil, nil, app, "ls", "-l")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to run command.")
}

func (s *S) TestExecuteCommandWithOutputAndFailure(c *check.C) {
	var buf bytes.Buffer
	app := NewFakeApp("marathon", "rush", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("ExecuteCommand", errors.New("Failed to run command."))
	p.PrepareOutput([]byte("myoutput!"))
	err := p.ExecuteCommand(nil, &buf, app, "ls", "-l")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to run command.")
	c.Assert(buf.String(), check.Equals, "myoutput!")
}

func (s *S) TestExecuteComandTimeout(c *check.C) {
	app := NewFakeApp("territories", "rush", 1)
	p := NewFakeProvisioner()
	err := p.ExecuteCommand(nil, nil, app, "ls -l")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "FakeProvisioner timed out waiting for output.")
}

func (s *S) TestAddr(c *check.C) {
	app := NewFakeApp("quick", "who", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, check.IsNil)
	addr, err := p.Addr(app)
	c.Assert(err, check.IsNil)
	c.Assert(addr, check.Equals, "quick.fake-lb.tsuru.io")
}

func (s *S) TestAddrFailure(c *check.C) {
	p := NewFakeProvisioner()
	p.PrepareFailure("Addr", errors.New("Cannot get addr of this app."))
	addr, err := p.Addr(nil)
	c.Assert(addr, check.Equals, "")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot get addr of this app.")
}

func (s *S) TestSetCName(c *check.C) {
	app := NewFakeApp("jean", "mj", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	err := p.SetCName(app, "cname.com")
	c.Assert(err, check.IsNil)
	c.Assert(p.apps[app.GetName()].cnames, check.DeepEquals, []string{"cname.com"})
}

func (s *S) TestSetCNameNotProvisioned(c *check.C) {
	app := NewFakeApp("jean", "mj", 0)
	p := NewFakeProvisioner()
	err := p.SetCName(app, "cname.com")
	c.Assert(err, check.Equals, errNotProvisioned)
}

func (s *S) TestSetCNameFailure(c *check.C) {
	app := NewFakeApp("jean", "mj", 0)
	p := NewFakeProvisioner()
	p.PrepareFailure("SetCName", errors.New("wut"))
	err := p.SetCName(app, "cname.com")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "wut")
}

func (s *S) TestUnsetCName(c *check.C) {
	app := NewFakeApp("jean", "mj", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	err := p.SetCName(app, "cname.com")
	c.Assert(err, check.IsNil)
	c.Assert(p.apps[app.GetName()].cnames, check.DeepEquals, []string{"cname.com"})
	err = p.UnsetCName(app, "cname.com")
	c.Assert(err, check.IsNil)
	c.Assert(p.HasCName(app, "cname.com"), check.Equals, false)
}

func (s *S) TestUnsetCNameNotProvisioned(c *check.C) {
	app := NewFakeApp("jean", "mj", 0)
	p := NewFakeProvisioner()
	err := p.UnsetCName(app, "cname.com")
	c.Assert(err, check.Equals, errNotProvisioned)
}

func (s *S) TestUnsetCNameFailure(c *check.C) {
	app := NewFakeApp("jean", "mj", 0)
	p := NewFakeProvisioner()
	p.PrepareFailure("UnsetCName", errors.New("wut"))
	err := p.UnsetCName(app, "cname.com")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "wut")
}

func (s *S) TestHasCName(c *check.C) {
	app := NewFakeApp("jean", "mj", 0)
	p := NewFakeProvisioner()
	p.Provision(app)
	err := p.SetCName(app, "cname.com")
	c.Assert(err, check.IsNil)
	c.Assert(p.HasCName(app, "cname.com"), check.Equals, true)
	err = p.UnsetCName(app, "cname.com")
	c.Assert(err, check.IsNil)
	c.Assert(p.HasCName(app, "cname.com"), check.Equals, false)
}

func (s *S) TestExecuteCommandOnce(c *check.C) {
	var buf bytes.Buffer
	output := []byte("myoutput!")
	app := NewFakeApp("grand-designs", "rush", 1)
	p := NewFakeProvisioner()
	p.PrepareOutput(output)
	err := p.ExecuteCommandOnce(&buf, nil, app, "ls", "-l")
	c.Assert(err, check.IsNil)
	cmds := p.GetCmds("ls", app)
	c.Assert(cmds, check.HasLen, 1)
	c.Assert(buf.String(), check.Equals, string(output))
}

func (s *S) TestExtensiblePlatformAdd(c *check.C) {
	p := ExtensibleFakeProvisioner{FakeProvisioner: NewFakeProvisioner()}
	args := map[string]string{"dockerfile": "mydockerfile.txt"}
	err := p.PlatformAdd("python", args, nil)
	c.Assert(err, check.IsNil)
	platform := p.GetPlatform("python")
	c.Assert(platform.Name, check.Equals, "python")
	c.Assert(platform.Version, check.Equals, 1)
	c.Assert(platform.Args, check.DeepEquals, args)
}

func (s *S) TestExtensiblePlatformAddTwice(c *check.C) {
	p := ExtensibleFakeProvisioner{FakeProvisioner: NewFakeProvisioner()}
	args := map[string]string{"dockerfile": "mydockerfile.txt"}
	err := p.PlatformAdd("python", args, nil)
	c.Assert(err, check.IsNil)
	err = p.PlatformAdd("python", nil, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "duplicate platform")
}

func (s *S) TestExtensiblePlatformUpdate(c *check.C) {
	p := ExtensibleFakeProvisioner{FakeProvisioner: NewFakeProvisioner()}
	args := map[string]string{"dockerfile": "mydockerfile.txt"}
	err := p.PlatformAdd("python", args, nil)
	c.Assert(err, check.IsNil)
	args["something"] = "wat"
	err = p.PlatformUpdate("python", args, nil)
	c.Assert(err, check.IsNil)
	platform := p.GetPlatform("python")
	c.Assert(platform.Name, check.Equals, "python")
	c.Assert(platform.Version, check.Equals, 2)
	c.Assert(platform.Args, check.DeepEquals, args)
}

func (s *S) TestExtensiblePlatformUpdateNotFound(c *check.C) {
	p := ExtensibleFakeProvisioner{FakeProvisioner: NewFakeProvisioner()}
	err := p.PlatformUpdate("python", nil, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "platform not found")
}

func (s *S) TestExtensiblePlatformRemove(c *check.C) {
	p := ExtensibleFakeProvisioner{FakeProvisioner: NewFakeProvisioner()}
	args := map[string]string{"dockerfile": "mydockerfile.txt"}
	err := p.PlatformAdd("python", args, nil)
	c.Assert(err, check.IsNil)
	err = p.PlatformRemove("python")
	c.Assert(err, check.IsNil)
	platform := p.GetPlatform("python")
	c.Assert(platform, check.IsNil)
}

func (s *S) TestExtensiblePlatformRemoveNotFound(c *check.C) {
	p := ExtensibleFakeProvisioner{FakeProvisioner: NewFakeProvisioner()}
	err := p.PlatformRemove("python")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "platform not found")
}

func (s *S) TestFakeProvisionerAddUnit(c *check.C) {
	app := NewFakeApp("red-sector", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, check.IsNil)
	p.AddUnit(app, provision.Unit{Name: "red-sector/1"})
	c.Assert(p.Units(app), check.HasLen, 1)
	c.Assert(p.apps[app.GetName()].unitLen, check.Equals, 1)
}

func (s *S) TestFakeProvisionerUnits(c *check.C) {
	app := NewFakeApp("red-sector", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, check.IsNil)
	p.AddUnit(app, provision.Unit{Name: "red-sector/1"})
	c.Assert(p.Units(app), check.HasLen, 1)
}

func (s *S) TestFakeProvisionerUnitsAppNotFound(c *check.C) {
	app := NewFakeApp("red-sector", "rush", 1)
	p := NewFakeProvisioner()
	c.Assert(p.Units(app), check.HasLen, 0)
}

func (s *S) TestFakeProvisionerSetUnitStatus(c *check.C) {
	app := NewFakeApp("red-sector", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, check.IsNil)
	unit := provision.Unit{AppName: "red-sector", Name: "red-sector/1", Status: provision.StatusStarted}
	p.AddUnit(app, unit)
	err = p.SetUnitStatus(unit, provision.StatusError)
	c.Assert(err, check.IsNil)
	unit = p.Units(app)[0]
	c.Assert(unit.Status, check.Equals, provision.StatusError)
}

func (s *S) TestFakeProvisionerSetUnitStatusAppNotFound(c *check.C) {
	p := NewFakeProvisioner()
	err := p.SetUnitStatus(provision.Unit{AppName: "something"}, provision.StatusError)
	c.Assert(err, check.Equals, errNotProvisioned)
}

func (s *S) TestFakeProvisionerSetUnitStatusUnitNotFound(c *check.C) {
	app := NewFakeApp("red-sector", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, check.IsNil)
	unit := provision.Unit{AppName: "red-sector", Name: "red-sector/1", Status: provision.StatusStarted}
	err = p.SetUnitStatus(unit, provision.StatusError)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "unit not found")
}

func (s *S) TestFakeProvisionerRegisterUnit(c *check.C) {
	app := NewFakeApp("shine-on", "diamond", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, check.IsNil)
	unit := provision.Unit{AppName: "shine-on", Name: "unit/1"}
	p.AddUnit(app, unit)
	units := p.Units(app)
	ip := units[0].Ip
	err = p.RegisterUnit(unit, nil)
	c.Assert(err, check.IsNil)
	units = p.Units(app)
	c.Assert(units[0].Ip, check.Equals, ip+"-updated")
}

func (s *S) TestFakeProvisionerRegisterUnitNotFound(c *check.C) {
	app := NewFakeApp("shine-on", "diamond", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, check.IsNil)
	unit := provision.Unit{AppName: "shine-on", Name: "unit/1"}
	err = p.RegisterUnit(unit, nil)
	c.Assert(err, check.ErrorMatches, "unit not found")
}

func (s *S) TestFakeProvisionerRegisterUnitSavesData(c *check.C) {
	app := NewFakeApp("shine-on", "diamond", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, check.IsNil)
	unit := provision.Unit{AppName: "shine-on", Name: "unit/1"}
	p.AddUnit(app, unit)
	units := p.Units(app)
	ip := units[0].Ip
	data := map[string]interface{}{"my": "data"}
	err = p.RegisterUnit(unit, data)
	c.Assert(err, check.IsNil)
	units = p.Units(app)
	c.Assert(units[0].Ip, check.Equals, ip+"-updated")
	c.Assert(p.CustomData(app), check.DeepEquals, data)
}

func (s *S) TestFakeProvisionerShellNoSpecification(c *check.C) {
	app := NewFakeApp("shine-on", "diamond", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, check.IsNil)
	unit := provision.Unit{AppName: "shine-on", Name: "unit/1"}
	p.AddUnit(app, unit)
	unit = provision.Unit{AppName: "shine-on", Name: "unit/2"}
	p.AddUnit(app, unit)
	unit = provision.Unit{AppName: "shine-on", Name: "unit/3"}
	p.AddUnit(app, unit)
	opts := provision.ShellOptions{App: app}
	err = p.Shell(opts)
	c.Assert(err, check.IsNil)
	c.Assert(p.Shells("unit/1"), check.DeepEquals, []provision.ShellOptions{opts})
	c.Assert(p.Shells("unit/2"), check.HasLen, 0)
	c.Assert(p.Shells("unit/3"), check.HasLen, 0)
}

func (s *S) TestFakeProvisionerShellSpecifying(c *check.C) {
	app := NewFakeApp("shine-on", "diamond", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, check.IsNil)
	unit := provision.Unit{AppName: "shine-on", Name: "unit/1"}
	p.AddUnit(app, unit)
	unit = provision.Unit{AppName: "shine-on", Name: "unit/2"}
	p.AddUnit(app, unit)
	unit = provision.Unit{AppName: "shine-on", Name: "unit/3"}
	p.AddUnit(app, unit)
	opts := provision.ShellOptions{App: app, Unit: "unit/3"}
	err = p.Shell(opts)
	c.Assert(err, check.IsNil)
	c.Assert(p.Shells("unit/3"), check.DeepEquals, []provision.ShellOptions{opts})
	c.Assert(p.Shells("unit/1"), check.HasLen, 0)
	c.Assert(p.Shells("unit/2"), check.HasLen, 0)
}

func (s *S) TestFakeProvisionerShellUnitNotFound(c *check.C) {
	app := NewFakeApp("shine-on", "diamond", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, check.IsNil)
	unit := provision.Unit{AppName: "shine-on", Name: "unit/1"}
	p.AddUnit(app, unit)
	opts := provision.ShellOptions{App: app, Unit: "unit/12"}
	err = p.Shell(opts)
	c.Assert(err.Error(), check.Equals, "unit not found")
}

func (s *S) TestFakeProvisionerShellNoUnits(c *check.C) {
	app := NewFakeApp("shine-on", "diamond", 1)
	p := NewFakeProvisioner()
	err := p.Provision(app)
	c.Assert(err, check.IsNil)
	opts := provision.ShellOptions{App: app}
	err = p.Shell(opts)
	c.Assert(err.Error(), check.Equals, "app has no units")
}
