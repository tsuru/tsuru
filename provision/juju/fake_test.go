// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"fmt"
	"github.com/globocom/tsuru/provision"
)

type FakeUnit struct {
	name    string
	machine int
	status  provision.Status
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

func (u *FakeUnit) GetStatus() provision.Status {
	u.actions = append(u.actions, "getstatus")
	return u.status
}

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
	namefmt := "%s/%d"
	for i := 0; i < units; i++ {
		app.units[i] = &FakeUnit{
			name:    fmt.Sprintf(namefmt, name, i),
			machine: i + 1,
			status:  provision.StatusStarted,
		}
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

func (a *FakeApp) ProvisionUnits() []provision.AppUnit {
	a.actions = append(a.actions, "getunits")
	return a.units
}
