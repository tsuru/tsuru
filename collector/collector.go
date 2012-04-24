package collector

import (
	"fmt"
	"github.com/timeredbull/tsuru/api/app"
	. "github.com/timeredbull/tsuru/database"
	"launchpad.net/goyaml"
	"launchpad.net/mgo/bson"
	"os/exec"
)

type Collector struct{}

type Unit struct {
	Machine int
	State   string
}

type Service struct {
	Units map[string]Unit
}

type output struct {
	Services map[string]Service
	Machines map[int]interface{}
}

func (c *Collector) Collect() ([]byte, error) {
	fmt.Println("collecting status")
	return exec.Command("juju", "status").Output()
}

func (c *Collector) Parse(data []byte) *output {
	fmt.Println("parsing yaml")
	raw := new(output)
	_ = goyaml.Unmarshal(data, raw)
	return raw
}

func (c *Collector) Update(out *output) {
	fmt.Println("updating status")

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
			c := Db.C("apps")
			c.Update(bson.M{"_id": appUnit.Id}, appUnit)
		}
	}
}
