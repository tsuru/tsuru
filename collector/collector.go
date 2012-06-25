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
	log.Print("collecting status")
	return exec.Command("juju", "status").Output()
}

func (c *Collector) Parse(data []byte) *output {
	log.Print("parsing yaml")
	raw := new(output)
	_ = goyaml.Unmarshal(data, raw)
	return raw
}

func (c *Collector) Update(out *output) {
	log.Print("updating status")
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
				u.AgentState = uMachine["agent-state"].(string)
			}
			a.State = appState(&u)
			a.AddOrUpdateUnit(&u)
			db.Session.Apps().Update(bson.M{"name": a.Name}, a)
		}
	}
}

func appState(u *unit.Unit) string {
	if u.AgentState == "running" && u.InstanceState == "running" {
		return "STARTED"
	}
	if u.AgentState == "not-started" && u.InstanceState == "error" {
		return "ERROR"
	}
	if u.InstanceState == "pending" || u.InstanceState == "" {
		return "PENDING"
	}
	return "STOPPED"
}
