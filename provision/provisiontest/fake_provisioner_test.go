// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provisiontest

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router/routertest"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	provTypes "github.com/tsuru/tsuru/types/provision"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type S struct{}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "fake_provision_tests_s")
}

func (s *S) SetUpTest(c *check.C) {
	servicemock.SetMockService(&servicemock.MockService{})
	err := storagev2.ClearAllCollections(nil)
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.Reset()
}

func (s *S) TestFakeAppAddUnit(c *check.C) {
	app := NewFakeApp("jean", "mj", 0)
	app.AddUnit(provTypes.Unit{ID: "jean-0"})
	c.Assert(app.units, check.HasLen, 1)
}

func (s *S) TestFakeAppGetMemory(c *check.C) {
	app := NewFakeApp("sou", "otm", 0)
	app.Memory = 100
	c.Assert(app.GetMemory(), check.Equals, int64(100))
}

func (s *S) TestEnvs(c *check.C) {
	app := FakeApp{name: "time"}
	env := bindTypes.EnvVar{
		Name:   "http_proxy",
		Value:  "http://theirproxy.com:3128/",
		Public: true,
	}
	app.SetEnv(env)
	envs := map[string]bindTypes.EnvVar{
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
	envs := []bindTypes.EnvVar{
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
	app.SetEnvs(bind.SetEnvArgs{
		Envs:          envs,
		ShouldRestart: true,
	})
	expected := map[string]bindTypes.EnvVar{
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
	a := NewFakeApp("foo", "static", 2)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 2)
	c.Assert(a.units, check.HasLen, 2)
	c.Assert(units[0].GetID(), check.Equals, a.units[0].ID)
	c.Assert(units[1].GetID(), check.Equals, a.units[1].ID)
}

func (s *S) TestUnsetEnvs(c *check.C) {
	app := FakeApp{name: "time"}
	env := bindTypes.EnvVar{
		Name:   "http_proxy",
		Value:  "http://theirproxy.com:3128/",
		Public: true,
	}
	app.SetEnv(env)
	app.UnsetEnvs(bind.UnsetEnvArgs{
		VariableNames: []string{"http_proxy"},
		ShouldRestart: true,
	})
	c.Assert(app.env, check.DeepEquals, map[string]bindTypes.EnvVar{})
}

func (s *S) TestFakeAppGetCname(c *check.C) {
	app := NewFakeApp("sou", "otm", 0)
	app.cname = []string{"cname1", "cname2"}
	c.Assert(app.GetCname(), check.DeepEquals, []string{"cname1", "cname2"})
}

func (s *S) TestFakeAppAddInstance(c *check.C) {
	app := NewFakeApp("sou", "otm", 0)
	err := app.AddInstance(context.TODO(), bind.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{
				ServiceName:  "mysql",
				InstanceName: "inst1",
				EnvVar:       bindTypes.EnvVar{Name: "env1", Value: "val1"},
			},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	err = app.AddInstance(context.TODO(), bind.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{
				ServiceName:  "mongodb",
				InstanceName: "inst2",
				EnvVar:       bindTypes.EnvVar{Name: "env2", Value: "val2"},
			},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	envs := app.GetServiceEnvs()
	c.Assert(envs, check.DeepEquals, []bindTypes.ServiceEnvVar{
		{
			ServiceName:  "mysql",
			InstanceName: "inst1",
			EnvVar:       bindTypes.EnvVar{Name: "env1", Value: "val1"},
		},
		{
			ServiceName:  "mongodb",
			InstanceName: "inst2",
			EnvVar:       bindTypes.EnvVar{Name: "env2", Value: "val2"},
		},
	})
}

func (s *S) TestFakeAppRemoveInstance(c *check.C) {
	app := NewFakeApp("sou", "otm", 0)
	err := app.AddInstance(context.TODO(), bind.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{
				ServiceName:  "mysql",
				InstanceName: "inst1",
				EnvVar:       bindTypes.EnvVar{Name: "env1", Value: "val1"},
			},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	err = app.AddInstance(context.TODO(), bind.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{
				ServiceName:  "mongodb",
				InstanceName: "inst2",
				EnvVar:       bindTypes.EnvVar{Name: "env2", Value: "val2"},
			},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	err = app.RemoveInstance(context.TODO(), bind.RemoveInstanceArgs{
		ServiceName:   "mysql",
		InstanceName:  "inst1",
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	envs := app.GetServiceEnvs()
	c.Assert(envs, check.DeepEquals, []bindTypes.ServiceEnvVar{
		{
			ServiceName:  "mongodb",
			InstanceName: "inst2",
			EnvVar:       bindTypes.EnvVar{Name: "env2", Value: "val2"},
		},
	})
}

func (s *S) TestFakeAppRemoveInstanceNotFound(c *check.C) {
	app := NewFakeApp("sou", "otm", 0)
	err := app.AddInstance(context.TODO(), bind.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{
				ServiceName:  "mysql",
				InstanceName: "inst1",
				EnvVar:       bindTypes.EnvVar{Name: "env1", Value: "val1"},
			},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	err = app.RemoveInstance(context.TODO(), bind.RemoveInstanceArgs{
		ServiceName:   "mysql",
		InstanceName:  "inst2",
		ShouldRestart: true,
	})
	c.Assert(err.Error(), check.Equals, "instance not found")
}

func (s *S) TestFakeAppRemoveInstanceServiceNotFound(c *check.C) {
	app := NewFakeApp("sou", "otm", 0)
	err := app.RemoveInstance(context.TODO(), bind.RemoveInstanceArgs{
		ServiceName:   "mysql",
		InstanceName:  "inst2",
		ShouldRestart: true,
	})
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
	err := p.Provision(context.TODO(), app)
	c.Assert(err, check.IsNil)
	c.Assert(p.Provisioned(app), check.Equals, true)
	otherapp := NewFakeApp("blue-sector", "rush", 1)
	c.Assert(p.Provisioned(otherapp), check.Equals, false)
}

func (s *S) TestRestarts(c *check.C) {
	app1 := NewFakeApp("fairy-tale", "shaman", 1)
	app2 := NewFakeApp("unfairly-tale", "shaman", 1)
	p := NewFakeProvisioner()
	p.apps = map[string]provisionedApp{
		app1.GetName(): {app: app1, restarts: map[string]int{"": 10, "web": 2}},
		app2.GetName(): {app: app1, restarts: map[string]int{"": 0}},
	}
	c.Assert(p.Restarts(app1, ""), check.Equals, 10)
	c.Assert(p.Restarts(app1, "web"), check.Equals, 2)
	c.Assert(p.Restarts(app2, ""), check.Equals, 0)
	c.Assert(p.Restarts(NewFakeApp("pride", "shaman", 1), ""), check.Equals, 0)
}

func (s *S) TestStarts(c *check.C) {
	app1 := NewFakeApp("fairy-tale", "shaman", 1)
	app2 := NewFakeApp("unfairly-tale", "shaman", 1)
	p := NewFakeProvisioner()
	p.apps = map[string]provisionedApp{
		app1.GetName(): {app: app1, starts: map[string]int{"web": 10, "worker": 1}},
		app2.GetName(): {app: app1, starts: map[string]int{"": 0}},
	}
	c.Assert(p.Starts(app1, "web"), check.Equals, 10)
	c.Assert(p.Starts(app1, "worker"), check.Equals, 1)
	c.Assert(p.Starts(app2, ""), check.Equals, 0)
	c.Assert(p.Starts(NewFakeApp("pride", "shaman", 1), ""), check.Equals, 0)
}

func (s *S) TestStops(c *check.C) {
	app1 := NewFakeApp("fairy-tale", "shaman", 1)
	app2 := NewFakeApp("unfairly-tale", "shaman", 1)
	p := NewFakeProvisioner()
	p.apps = map[string]provisionedApp{
		app1.GetName(): {app: app1, stops: map[string]int{"web": 10, "worker": 1}},
		app2.GetName(): {app: app1, stops: map[string]int{"": 0}},
	}
	c.Assert(p.Stops(app1, "web"), check.Equals, 10)
	c.Assert(p.Stops(app1, "worker"), check.Equals, 1)
	c.Assert(p.Stops(app2, ""), check.Equals, 0)
	c.Assert(p.Stops(NewFakeApp("pride", "shaman", 1), ""), check.Equals, 0)
}

func (s *S) TestGetUnits(c *check.C) {
	list := []provTypes.Unit{
		{ID: "chain-lighting-0", AppName: "chain-lighting", ProcessName: "web", Type: "django", IP: "10.10.10.10", Status: provTypes.UnitStatusStarted},
		{ID: "chain-lighting-1", AppName: "chain-lighting", ProcessName: "web", Type: "django", IP: "10.10.10.15", Status: provTypes.UnitStatusStarted},
	}
	app := NewFakeApp("chain-lighting", "rush", 1)
	p := NewFakeProvisioner()
	p.apps = map[string]provisionedApp{
		app.GetName(): {app: app, units: list},
	}
	units := p.GetUnits(app)
	c.Assert(units, check.DeepEquals, list)
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

func (s *S) TestProvision(c *check.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Provision(context.TODO(), app)
	c.Assert(err, check.IsNil)
	pApp := p.apps[app.GetName()]
	c.Assert(pApp.app, check.DeepEquals, app)
	c.Assert(pApp.units, check.HasLen, 0)
	c.Assert(routertest.FakeRouter.HasBackend(app.GetName()), check.Equals, true)
}

func (s *S) TestProvisionWithPreparedFailure(c *check.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("Provision", errors.New("Failed to provision."))
	err := p.Provision(context.TODO(), app)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to provision.")
}

func (s *S) TestDoubleProvision(c *check.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Provision(context.TODO(), app)
	c.Assert(err, check.IsNil)
	err = p.Provision(context.TODO(), app)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "App already provisioned.")
}

func (s *S) TestRestart(c *check.C) {
	a := NewFakeApp("kid-gloves", "rush", 1)
	nApp := app.App{
		Name: a.name,
	}

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertOne(context.TODO(), nApp)
	defer appsCollection.DeleteOne(context.TODO(), mongoBSON.M{"name": nApp.Name})
	c.Assert(err, check.IsNil)
	p := NewFakeProvisioner()
	p.Provision(context.TODO(), a)
	err = p.Restart(context.TODO(), a, "web", nil, nil)
	c.Assert(err, check.IsNil)
	c.Assert(p.Restarts(a, "web"), check.Equals, 1)
}

func (s *S) TestStart(c *check.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	p.Provision(context.TODO(), app)
	err := p.Start(context.TODO(), app, "", nil, nil)
	c.Assert(err, check.IsNil)
	err = p.Start(context.TODO(), app, "web", nil, nil)
	c.Assert(err, check.IsNil)
	c.Assert(p.Starts(app, ""), check.Equals, 1)
	c.Assert(p.Starts(app, "web"), check.Equals, 1)
}

func (s *S) TestStop(c *check.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	p.Provision(context.TODO(), app)
	err := p.Stop(context.TODO(), app, "", nil, nil)
	c.Assert(err, check.IsNil)
	c.Assert(p.Stops(app, ""), check.Equals, 1)
}

func (s *S) TestRestartNotProvisioned(c *check.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Restart(context.TODO(), app, "web", nil, nil)
	c.Assert(err, check.Equals, errNotProvisioned)
}

func (s *S) TestRestartWithPreparedFailure(c *check.C) {
	app := NewFakeApp("fairy-tale", "shaman", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("Restart", errors.New("Failed to restart."))
	err := p.Restart(context.TODO(), app, "web", nil, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to restart.")
}

func (s *S) TestDestroy(c *check.C) {
	app := NewFakeApp("kid-gloves", "rush", 1)
	p := NewFakeProvisioner()
	p.Provision(context.TODO(), app)
	err := p.Destroy(context.TODO(), app)
	c.Assert(err, check.IsNil)
	c.Assert(p.Provisioned(app), check.Equals, false)
}

func (s *S) TestDestroyWithPreparedFailure(c *check.C) {
	app := NewFakeApp("red-lenses", "rush", 1)
	p := NewFakeProvisioner()
	p.PrepareFailure("Destroy", errors.New("Failed to destroy."))
	err := p.Destroy(context.TODO(), app)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to destroy.")
}

func (s *S) TestDestroyNotProvisionedApp(c *check.C) {
	app := NewFakeApp("red-lenses", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Destroy(context.TODO(), app)
	c.Assert(err, check.Equals, errNotProvisioned)
}

func (s *S) TestAddUnits(c *check.C) {
	app := NewFakeApp("mystic-rhythms", "rush", 0)
	p := NewFakeProvisioner()
	err := p.Provision(context.TODO(), app)
	c.Assert(err, check.IsNil)
	err = p.AddUnits(context.TODO(), app, 2, "web", nil, nil)
	c.Assert(err, check.IsNil)
	err = p.AddUnits(context.TODO(), app, 2, "worker", nil, nil)
	c.Assert(err, check.IsNil)
	allUnits := p.GetUnits(app)
	c.Assert(allUnits, check.HasLen, 4)
	c.Assert(allUnits[0].ProcessName, check.Equals, "web")
	c.Assert(allUnits[1].ProcessName, check.Equals, "web")
	c.Assert(allUnits[2].ProcessName, check.Equals, "worker")
	c.Assert(allUnits[3].ProcessName, check.Equals, "worker")
}

func (s *S) TestAddUnitsCopiesTheUnitsSlice(c *check.C) {
	app := NewFakeApp("fiction", "python", 0)
	p := NewFakeProvisioner()
	p.Provision(context.TODO(), app)
	defer p.Destroy(context.TODO(), app)
	err := p.AddUnits(context.TODO(), app, 3, "web", nil, nil)
	c.Assert(err, check.IsNil)
	units, err := p.Units(context.TODO(), app)
	c.Assert(err, check.IsNil)
	units[0].ID = "something-else"
	c.Assert(units[0].ID, check.Not(check.Equals), p.GetUnits(app)[1].ID)
}

func (s *S) TestAddZeroUnits(c *check.C) {
	p := NewFakeProvisioner()
	err := p.AddUnits(context.TODO(), nil, 0, "web", nil, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot add 0 units.")
}

func (s *S) TestAddUnitsUnprovisionedApp(c *check.C) {
	app := NewFakeApp("mystic-rhythms", "rush", 0)
	p := NewFakeProvisioner()
	err := p.AddUnits(context.TODO(), app, 1, "web", nil, nil)
	c.Assert(err, check.Equals, errNotProvisioned)
}

func (s *S) TestAddUnitsFailure(c *check.C) {
	p := NewFakeProvisioner()
	p.PrepareFailure("AddUnits", errors.New("Cannot add more units."))
	err := p.AddUnits(context.TODO(), nil, 10, "web", nil, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot add more units.")
}

func (s *S) TestAddUnitsToNode(c *check.C) {
	app := NewFakeApp("mystic-rhythms", "rush", 0)
	p := NewFakeProvisioner()
	p.Reset()
	err := p.Provision(context.TODO(), app)
	c.Assert(err, check.IsNil)
	_, err = p.AddUnitsToNode(app, 2, "web", nil, "nother", nil)
	c.Assert(err, check.IsNil)
	allUnits := p.GetUnits(app)
	c.Assert(allUnits, check.HasLen, 2)
	c.Assert(allUnits[0].Address.Host, check.Equals, "nother:1")
	c.Assert(allUnits[1].Address.Host, check.Equals, "nother:2")
}

func (s *S) TestRemoveUnits(c *check.C) {
	app := NewFakeApp("hemispheres", "rush", 0)
	p := NewFakeProvisioner()
	p.Provision(context.TODO(), app)
	err := p.AddUnits(context.TODO(), app, 5, "web", nil, nil)
	c.Assert(err, check.IsNil)
	oldUnits := p.GetUnits(app)
	buf := bytes.NewBuffer(nil)
	err = p.RemoveUnits(context.TODO(), app, 3, "web", nil, buf)
	c.Assert(err, check.IsNil)
	units := p.GetUnits(app)
	c.Assert(units, check.HasLen, 2)
	c.Assert(units[0].ID, check.Equals, "hemispheres-3")
	c.Assert(buf.String(), check.Equals, "removing 3 units")
	c.Assert(units[0].Address.String(), check.Equals, oldUnits[3].Address.String())
}

func (s *S) TestRemoveUnitsDifferentProcesses(c *check.C) {
	app := NewFakeApp("hemispheres", "rush", 0)
	p := NewFakeProvisioner()
	err := p.Provision(context.TODO(), app)
	c.Assert(err, check.IsNil)
	err = p.AddUnits(context.TODO(), app, 5, "p1", nil, nil)
	c.Assert(err, check.IsNil)
	err = p.AddUnits(context.TODO(), app, 2, "p2", nil, nil)
	c.Assert(err, check.IsNil)
	err = p.AddUnits(context.TODO(), app, 2, "p3", nil, nil)
	c.Assert(err, check.IsNil)
	err = p.RemoveUnits(context.TODO(), app, 2, "p2", nil, nil)
	c.Assert(err, check.IsNil)
	c.Assert(p.GetUnits(app), check.HasLen, 7)
	for i, u := range p.GetUnits(app) {
		if i < 5 {
			c.Assert(u.ProcessName, check.Equals, "p1")
		} else {
			c.Assert(u.ProcessName, check.Equals, "p3")
		}
	}
}

func (s *S) TestRemoveUnitsTooManyUnits(c *check.C) {
	app := NewFakeApp("hemispheres", "rush", 0)
	p := NewFakeProvisioner()
	err := p.Provision(context.TODO(), app)
	c.Assert(err, check.IsNil)
	err = p.AddUnits(context.TODO(), app, 1, "web", nil, nil)
	c.Assert(err, check.IsNil)
	err = p.RemoveUnits(context.TODO(), app, 3, "web", nil, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "too many units to remove")
}

func (s *S) TestRemoveUnitsTooManyUnitsOfProcess(c *check.C) {
	app := NewFakeApp("hemispheres", "rush", 0)
	p := NewFakeProvisioner()
	err := p.Provision(context.TODO(), app)
	c.Assert(err, check.IsNil)
	err = p.AddUnits(context.TODO(), app, 1, "web", nil, nil)
	c.Assert(err, check.IsNil)
	err = p.AddUnits(context.TODO(), app, 4, "worker", nil, nil)
	c.Assert(err, check.IsNil)
	err = p.RemoveUnits(context.TODO(), app, 3, "web", nil, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "too many units to remove")
}

func (s *S) TestRemoveUnitsUnprovisionedApp(c *check.C) {
	app := NewFakeApp("tears", "bruce", 0)
	p := NewFakeProvisioner()
	err := p.RemoveUnits(context.TODO(), app, 1, "web", nil, nil)
	c.Assert(err, check.Equals, errNotProvisioned)
}

func (s *S) TestRemoveUnitsFailure(c *check.C) {
	p := NewFakeProvisioner()
	p.PrepareFailure("RemoveUnits", errors.New("This program has performed an illegal operation."))
	err := p.RemoveUnits(context.TODO(), nil, 0, "web", nil, nil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "This program has performed an illegal operation.")
}

func (s *S) TestExecuteCommand(c *check.C) {
	var buf bytes.Buffer
	output := []byte("myoutput!")
	app := NewFakeApp("grand-designs", "rush", 0)
	p := NewFakeProvisioner()
	err := p.Provision(context.TODO(), app)
	c.Assert(err, check.IsNil)
	err = p.AddUnits(context.TODO(), app, 2, "web", nil, nil)
	c.Assert(err, check.IsNil)
	units := p.GetUnits(app)
	p.PrepareOutput(output)
	p.PrepareOutput(output)
	err = p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		Stdout: &buf,
		App:    app,
		Cmds:   []string{"ls", "-l"},
		Units:  []string{units[0].ID, units[1].ID},
	})
	c.Assert(err, check.IsNil)
	expectedExecs := []provision.ExecOptions{{
		Stdout: &buf,
		App:    app,
		Cmds:   []string{"ls", "-l"},
		Units:  []string{units[0].ID, units[1].ID},
	}}
	execsUnit0 := p.Execs(units[0].ID)
	c.Assert(execsUnit0, check.DeepEquals, expectedExecs)
	execsUnit1 := p.Execs(units[1].ID)
	c.Assert(execsUnit1, check.DeepEquals, expectedExecs)
	expected := string(output) + string(output)
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestExecuteCommandFailureNoOutput(c *check.C) {
	app := NewFakeApp("manhattan-project", "rush", 0)
	p := NewFakeProvisioner()
	err := p.Provision(context.TODO(), app)
	c.Assert(err, check.IsNil)
	err = p.AddUnits(context.TODO(), app, 1, "web", nil, nil)
	c.Assert(err, check.IsNil)
	units := p.GetUnits(app)
	p.PrepareFailure("ExecuteCommand", errors.New("Failed to run command."))
	err = p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:   app,
		Cmds:  []string{"ls", "-l"},
		Units: []string{units[0].ID},
	})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to run command.")
}

func (s *S) TestExecuteCommandWithOutputAndFailure(c *check.C) {
	var buf bytes.Buffer
	app := NewFakeApp("marathon", "rush", 0)
	p := NewFakeProvisioner()
	err := p.Provision(context.TODO(), app)
	c.Assert(err, check.IsNil)
	err = p.AddUnits(context.TODO(), app, 1, "web", nil, nil)
	c.Assert(err, check.IsNil)
	units := p.GetUnits(app)
	p.PrepareFailure("ExecuteCommand", errors.New("Failed to run command."))
	p.PrepareOutput([]byte("myoutput!"))
	err = p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:    app,
		Stderr: &buf,
		Cmds:   []string{"ls", "-l"},
		Units:  []string{units[0].ID},
	})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to run command.")
	c.Assert(buf.String(), check.Equals, "myoutput!")
}

func (s *S) TestExecuteComandTimeout(c *check.C) {
	app := NewFakeApp("territories", "rush", 0)
	p := NewFakeProvisioner()
	err := p.Provision(context.TODO(), app)
	c.Assert(err, check.IsNil)
	err = p.AddUnits(context.TODO(), app, 1, "web", nil, nil)
	c.Assert(err, check.IsNil)
	units := p.GetUnits(app)
	err = p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:   app,
		Cmds:  []string{"ls", "-l"},
		Units: []string{units[0].ID},
	})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "FakeProvisioner timed out waiting for output.")
}

func (s *S) TestExecuteCommandNoUnits(c *check.C) {
	var buf bytes.Buffer
	output := []byte("myoutput!")
	app := NewFakeApp("grand-designs", "rush", 1)
	p := NewFakeProvisioner()
	p.PrepareOutput(output)
	err := p.ExecuteCommand(context.TODO(), provision.ExecOptions{
		App:    app,
		Stdout: &buf,
		Cmds:   []string{"ls", "-l"},
	})
	c.Assert(err, check.IsNil)
	expectedExecs := []provision.ExecOptions{{
		Stdout: &buf,
		App:    app,
		Cmds:   []string{"ls", "-l"},
	}}
	execsIsolated := p.Execs("isolated")
	c.Assert(execsIsolated, check.DeepEquals, expectedExecs)
	expected := string(output)
	c.Assert(buf.String(), check.Equals, expected)
}

func (s *S) TestAddr(c *check.C) {
	app := NewFakeApp("quick", "who", 1)
	p := NewFakeProvisioner()
	err := p.Provision(context.TODO(), app)
	c.Assert(err, check.IsNil)
	addr, err := p.Addr(app)
	c.Assert(err, check.IsNil)
	c.Assert(addr, check.Equals, "quick.fakerouter.com")
}

func (s *S) TestAddrFailure(c *check.C) {
	p := NewFakeProvisioner()
	p.PrepareFailure("Addr", errors.New("Cannot get addr of this app."))
	addr, err := p.Addr(nil)
	c.Assert(addr, check.Equals, "")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Cannot get addr of this app.")
}

func (s *S) TestFakeProvisionerAddUnit(c *check.C) {
	app := NewFakeApp("red-sector", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Provision(context.TODO(), app)
	c.Assert(err, check.IsNil)
	p.AddUnit(app, provTypes.Unit{ID: "red-sector/1"})
	units, err := p.Units(context.TODO(), app)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(p.apps[app.GetName()].unitLen, check.Equals, 1)
}

func (s *S) TestFakeProvisionerUnits(c *check.C) {
	app := NewFakeApp("red-sector", "rush", 1)
	p := NewFakeProvisioner()
	err := p.Provision(context.TODO(), app)
	c.Assert(err, check.IsNil)
	p.AddUnit(app, provTypes.Unit{ID: "red-sector/1"})
	units, err := p.Units(context.TODO(), app)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
}

func (s *S) TestFakeProvisionerUnitsAppNotFound(c *check.C) {
	app := NewFakeApp("red-sector", "rush", 1)
	p := NewFakeProvisioner()
	units, err := p.Units(context.TODO(), app)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 0)
}

func (s *S) TestGetAppFromUnitID(c *check.C) {
	list := []provTypes.Unit{
		{ID: "chain-lighting-0", AppName: "chain-lighting", ProcessName: "web", Type: "django", IP: "10.10.10.10", Status: provTypes.UnitStatusStarted},
	}
	app := NewFakeApp("chain-lighting", "rush", 1)
	p := NewFakeProvisioner()
	p.apps = map[string]provisionedApp{
		app.GetName(): {app: app, units: list},
	}
	a, err := p.GetAppFromUnitID("chain-lighting-0")
	c.Assert(err, check.IsNil)
	c.Assert(app, check.DeepEquals, a)
}

func (s *S) TestGetAppFromUnitIDNotFound(c *check.C) {
	p := NewFakeProvisioner()
	_, err := p.GetAppFromUnitID("chain-lighting-0")
	c.Assert(err, check.NotNil)
}

func (s *S) TestUpdateApp(c *check.C) {
	app := NewFakeApp("myapp", "arch", 1)
	p := NewFakeProvisioner()
	err := p.Provision(context.TODO(), app)
	c.Assert(err, check.IsNil)
	newApp := NewFakeApp("myapp", "python", 1)
	err = p.UpdateApp(context.TODO(), app, newApp, nil)
	c.Assert(err, check.IsNil)
	c.Assert(p.Provisioned(newApp), check.Equals, true)
	c.Assert(p.apps["myapp"].app, check.DeepEquals, newApp)
}
