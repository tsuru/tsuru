package collector

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"launchpad.net/goyaml"
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

func (c *Collector) Update(db *sql.DB, out *output) {
	fmt.Println("updating status")

	var state, ip string
	updateApp, _ := db.Prepare("UPDATE apps SET state=?, ip=? WHERE name=?")
	for serviceName, service := range out.Services {
		for _, unit := range service.Units {
			tx, _ := db.Begin()
			stmt := tx.Stmt(updateApp)
			defer stmt.Close()
			if unit.State == "started" {
				state = "STARTED"
			} else {
				state = "STOPPED"
			}
			ip = out.Machines[unit.Machine].(map[interface{}]interface{})["dns-name"].(string)
			stmt.Exec(state, ip, serviceName)
			tx.Commit()
		}
	}
}
