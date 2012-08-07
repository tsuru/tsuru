package main

import (
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/log"
	"labix.org/v2/mgo/bson"
	"launchpad.net/goyaml"
	"os/exec"
)

type Collector struct{}

type Service struct {
	Units map[string]unit.Unit
}

type output struct {
	Services map[string]Service
	Machines map[int]interface{}
}

func (c *Collector) Collect() ([]byte, error) {
	log.Print("collecting status from juju")
	return exec.Command("juju", "status").Output()
}

func (c *Collector) Parse(data []byte) *output {
	log.Print("parsing juju yaml")
	raw := new(output)
	_ = goyaml.Unmarshal(data, raw)
	return raw
}

func (c *Collector) Update(out *output) {
	log.Print("updating status from juju")
	for serviceName, service := range out.Services {
		for _, yUnit := range service.Units {
			u := unit.Unit{}
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
			a.State = appState(&u)
			a.AddOrUpdateUnit(&u)
			db.Session.Apps().Update(bson.M{"name": a.Name}, a)
		}
	}
}

func appState(u *unit.Unit) string {
	if u.InstanceState == "error" || u.AgentState == "install-error" {
		return "error"
	}
	if u.MachineAgentState == "pending" || u.InstanceState == "pending" || u.MachineAgentState == "" || u.InstanceState == "" {
		return "creating"
	}
	if u.MachineAgentState == "running" && u.InstanceState == "running" && u.AgentState == "pending" {
		return "installing"
	}
	if u.MachineAgentState == "running" && u.AgentState == "started" && u.InstanceState == "running" {
		return "started"
	}
	return "pending"
}
