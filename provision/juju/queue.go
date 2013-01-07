// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/queue"
	"sort"
)

const addUnitToLoadBalancer = "add-unit-to-lb"

type app struct {
	name string
}

func (a *app) Log(m, s string) error {
	return nil
}

func (a *app) GetName() string {
	return a.name
}

func (a *app) GetFramework() string {
	return ""
}

func (a *app) ProvisionUnits() []provision.AppUnit {
	return nil
}

func handle(msg *queue.Message) {
	switch msg.Action {
	case addUnitToLoadBalancer:
		if len(msg.Args) < 2 {
			log.Printf("Failed to handle %q: it requires at least two arguments.", msg.Action)
			queue.Delete(msg)
			return
		}
		a := app{name: msg.Args[0]}
		unitNames := msg.Args[1:]
		sort.Strings(unitNames)
		status, err := (&JujuProvisioner{}).CollectStatus()
		if err != nil {
			log.Printf("Failed to handle %q: failed to run juju status:\n%s.", msg.Action, err)
			msg.Release()
			return
		}
		var units []provision.Unit
		for _, u := range status {
			n := sort.SearchStrings(unitNames, u.Name)
			if unitNames[n] == u.Name {
				units = append(units, u)
			}
		}
		if len(units) == 0 {
			log.Printf("Failed to handle %q: units not found.", msg.Action)
			queue.Delete(msg)
			return
		}
		manager := ELBManager{}
		for _, unit := range units {
			manager.Register(&a, unit)
		}
		queue.Delete(msg)
	default:
		msg.Release()
	}
}
