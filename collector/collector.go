// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/api/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"labix.org/v2/mgo/bson"
	"launchpad.net/goyaml"
	"os/exec"
)

type Service struct {
	Units map[string]app.Unit
}

type output struct {
	Services map[string]Service
	Machines map[int]interface{}
}

func collect() ([]byte, error) {
	log.Print("collecting status from juju")
	return exec.Command("juju", "status").Output()
}

func parse(data []byte) *output {
	log.Print("parsing juju yaml")
	raw := new(output)
	_ = goyaml.Unmarshal(data, raw)
	return raw
}

func update(out *output) {
	log.Print("updating status from juju")
	for serviceName, service := range out.Services {
		for _, yUnit := range service.Units {
			u := app.Unit{}
			a := app.App{Name: serviceName}
			a.Get()
			uMachine := out.Machines[yUnit.Machine].(map[interface{}]interface{})
			if uMachine["instance-id"] != nil {
				u.InstanceId = uMachine["instance-id"].(string)
			}
			if uMachine["dns-name"] != nil {
				u.Ip = uMachine["dns-name"].(string)
			}
			u.Machine = yUnit.Machine
			if uMachine["instance-state"] != nil {
				u.InstanceState = uMachine["instance-state"].(string)
			}
			if uMachine["agent-state"] != nil {
				u.MachineAgentState = uMachine["agent-state"].(string)
			}
			u.AgentState = yUnit.AgentState
			a.State = u.State()
			a.AddUnit(&u)
			db.Session.Apps().Update(bson.M{"name": a.Name}, a)
		}
	}
}
