package collector

import (
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/log"
	"launchpad.net/goyaml"
	"launchpad.net/mgo/bson"
	"os/exec"
)

type Collector struct{}

type Unit struct {
	Machine int
	State   string `yaml:"agent-state"`
}

type Service struct {
	Units map[string]Unit
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
		for _, unit := range service.Units {
			appUnit := app.App{Name: serviceName}
			appUnit.Get()
			if unit.State == "started" {
				appUnit.State = "STARTED"
			} else {
				appUnit.State = "STOPPED"
			}
			appUnit.Ip = out.Machines[unit.Machine].(map[interface{}]interface{})["dns-name"].(string)
			appUnit.Machine = unit.Machine
			db.Session.Apps().Update(bson.M{"name": appUnit.Name}, appUnit)
		}
	}
}
