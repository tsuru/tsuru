package collector

import (
	"launchpad.net/goyaml"
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"fmt"
	"os/exec"
)

type collector struct{}

type Unit struct {
	State string
}

type Service struct {
	Units map[string]Unit
}

type output struct {
	Services map[string]Service
}

func (c *collector) Collect() ([]byte, error) {
	return exec.Command("juju status").Output()
}

func (c *collector) Parse(data []byte) *output {
	raw := new(output)
	_ = goyaml.Unmarshal(data, raw)
	return raw
}

func (c *collector) Update(out *output) {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()

	updateApp, _ := db.Prepare("UPDATE apps SET state=?")

	for _, service := range out.Services {
		for _, unit := range service.Units {
			fmt.Println(unit.State)
			tx, _ := db.Begin()
			stmt := tx.Stmt(updateApp)
			defer stmt.Close()
			if unit.State == "started" {
				stmt.Exec("RUNNING")
			} else {
				stmt.Exec("STOPPED")
			}
			tx.Commit()
		}
	}

}
