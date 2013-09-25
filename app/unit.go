// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"github.com/globocom/tsuru/provision"
	"regexp"
	"strconv"
)

// Unit is the smaller bit in tsuru. Each app is composed of one or more units.
//
// The unit is equivalent to a machine. How the machine is actually represented
// (baremetal, virtual machine, jails, containers, etc.) is up to the
// provisioner.
type Unit struct {
	Name       string
	Type       string
	Machine    int
	InstanceId string
	Ip         string
	State      string
	QuotaItem  string
	app        *App
}

func (u *Unit) GetName() string {
	return u.Name
}

func (u *Unit) GetMachine() int {
	return u.Machine
}

func (u *Unit) GetIp() string {
	return u.Ip
}

func (u *Unit) GetStatus() provision.Status {
	return provision.Status(u.State)
}

func (u *Unit) GetInstanceId() string {
	return u.InstanceId
}

// UnitSlice attaches the methods of sort.Interface to []Unit, sorting in increasing order.
type UnitSlice []Unit

func (u UnitSlice) Len() int {
	return len(u)
}

func (u UnitSlice) Less(i, j int) bool {
	weight := map[string]int{
		provision.StatusError.String():      0,
		provision.StatusDown.String():       1,
		provision.StatusInstalling.String(): 2,
		provision.StatusBuilding.String():   3,
		provision.StatusStarted.String():    4,
	}
	return weight[u[i].State] < weight[u[j].State]
}

func (u UnitSlice) Swap(i, j int) {
	u[i], u[j] = u[j], u[i]
}

func generateUnitQuotaItems(app *App, n int) []string {
	l := len(app.Units)
	initial := 0
	names := make([]string, n)
	if l > 0 {
		last := app.Units[l-1]
		re := regexp.MustCompile(app.Name + `-(\d+)`)
		parts := re.FindStringSubmatch(last.QuotaItem)
		if len(parts) > 1 {
			initial, _ = strconv.Atoi(parts[1])
			initial++
		}
	}
	for i := 0; i < n; i++ {
		names[i] = fmt.Sprintf("%s-%d", app.Name, initial+i)
	}
	return names
}
