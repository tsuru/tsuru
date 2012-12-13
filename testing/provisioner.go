// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"github.com/globocom/tsuru/provision"
	"io"
	"strconv"
	"time"
)

func init() {
	provision.Register("fake", &FakeProvisioner{})
}

// Fake implementation for provision.Unit.
type FakeUnit struct {
	name    string
	machine int
	actions []string
}

func (u *FakeUnit) GetName() string {
	u.actions = append(u.actions, "getname")
	return u.name
}

func (u *FakeUnit) GetMachine() int {
	u.actions = append(u.actions, "getmachine")
	return u.machine
}

// Fake implementation for provision.App.
type FakeApp struct {
	name      string
	framework string
	units     []provision.AppUnit
	logs      []string
	actions   []string
}

func NewFakeApp(name, framework string, units int) *FakeApp {
	app := FakeApp{
		name:      name,
		framework: framework,
		units:     make([]provision.AppUnit, units),
	}
	for i := 0; i < units; i++ {
		app.units[i] = &FakeUnit{name: name, machine: i + 1}
	}
	return &app
}

func (a *FakeApp) Log(message, source string) error {
	a.logs = append(a.logs, source+message)
	a.actions = append(a.actions, "log "+source+" - "+message)
	return nil
}

func (a *FakeApp) GetName() string {
	a.actions = append(a.actions, "getname")
	return a.name
}

func (a *FakeApp) GetFramework() string {
	a.actions = append(a.actions, "getframework")
	return a.framework
}

func (a *FakeApp) GetProvisionUnits() []provision.AppUnit {
	a.actions = append(a.actions, "getunits")
	return a.units
}

type Cmd struct {
	Cmd  string
	Args []string
	App  provision.App
}

// Fake implementation for provision.Provisioner.
type FakeProvisioner struct {
	apps     []provision.App
	Cmds     []Cmd
	outputs  chan []byte
	failures chan error
}

func NewFakeProvisioner() *FakeProvisioner {
	p := FakeProvisioner{}
	p.outputs = make(chan []byte, 8)
	p.failures = make(chan error, 8)
	return &p
}

func (p *FakeProvisioner) FindApp(app provision.App) int {
	for i, a := range p.apps {
		if a.GetName() == app.GetName() {
			return i
		}
	}
	return -1
}

func (p *FakeProvisioner) PrepareOutput(b []byte) {
	p.outputs <- b
}

func (p *FakeProvisioner) PrepareCommandFailure(err error) {
	p.failures <- err
}

func (p *FakeProvisioner) Reset() {
	close(p.outputs)
	close(p.failures)
	p.outputs = make(chan []byte, 8)
	p.failures = make(chan error, 8)
}

func (p *FakeProvisioner) Provision(app provision.App) *provision.Error {
	index := p.FindApp(app)
	if index > -1 {
		return &provision.Error{Reason: "App already provisioned."}
	}
	p.apps = append(p.apps, app)
	return nil
}

func (p *FakeProvisioner) Destroy(app provision.App) *provision.Error {
	index := p.FindApp(app)
	if index == -1 {
		return &provision.Error{Reason: "App is not provisioned."}
	}
	copy(p.apps[index:], p.apps[index+1:])
	p.apps = p.apps[:len(p.apps)-1]
	return nil
}

func (p *FakeProvisioner) ExecuteCommand(w io.Writer, app provision.App, cmd string, args ...string) error {
	command := Cmd{
		Cmd:  cmd,
		Args: args,
		App:  app,
	}
	p.Cmds = append(p.Cmds, command)
	var output []byte
	select {
	case output = <-p.outputs:
		w.Write(output)
	case <-time.After(1e9):
	}
	return nil
}

func (p *FakeProvisioner) CollectStatus() ([]provision.Unit, *provision.Error) {
	units := make([]provision.Unit, len(p.apps))
	for i, app := range p.apps {
		unit := provision.Unit{
			Name:    "somename",
			AppName: app.GetName(),
			Type:    app.GetFramework(),
			Status:  "started",
			Ip:      "10.10.10." + strconv.Itoa(i+1),
			Machine: i + 1,
		}
		units = append(units, unit)
	}
	return units, nil
}
