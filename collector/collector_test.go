package collector

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"path/filepath"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{
	db *sql.DB
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	s.db, _ = sql.Open("sqlite3", "./tsuru.db")
	_, err := s.db.Exec("CREATE TABLE apps (id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, name varchar(255), framework varchar(255), state varchar(255), ip varchar(100))")
	c.Check(err, IsNil)
}

func (s *S) TearDownSuite(c *C) {
	os.Remove("./tsuru.db")
	s.db.Close()
}

func (s *S) TestCollectorUpdate(c *C) {
	insertApp, _ := s.db.Prepare("INSERT INTO apps (id, name, state) VALUES (?, ?, ?)")

	tx, _ := s.db.Begin()
	stmt := tx.Stmt(insertApp)
	defer stmt.Close()
	stmt.Exec(1, "umaappqq", "STOPPED")
	tx.Commit()

	var collector Collector

	out := &output{
		Services: map[string]Service{
			"umaappqq": Service{
				Units: map[string]Unit{
					"umaappqq/0": Unit{
						State: "started"}}}}}

	collector.Update(s.db, out)

	rows, _ := s.db.Query("SELECT state FROM apps WHERE id = 1")

	var state string
	for rows.Next() {
		rows.Scan(&state)
	}

	c.Assert("STARTED", DeepEquals, state)
}

func (s *S) TestCollectorParser(c *C) {
	var collector Collector

	file, _ := os.Open(filepath.Join("testdata", "output.yaml"))
	jujuOutput, _ := ioutil.ReadAll(file)
	file.Close()

	expected := &output{
		Services: map[string]Service{
			"umaappqq": Service{
				Units: map[string]Unit{
					"umaappqq/0": Unit{
						State: "started",
						Machine: 1,
					},
				},
			},
		},
		Machines: map[int]interface{}{
			0: map[interface{}]interface{}{
				"dns-name": "192.168.0.10",
				"instance-id": "i-00000zz6",
				"instance-state": "running",
				"state": "running",
			},
			1: map[interface{}]interface{}{
				"dns-name": "192.168.0.11",
				"instance-id": "i-00000zz7",
				"instance-state": "running",
				"state": "running",
			},
		},
	}

	c.Assert(collector.Parse(jujuOutput), DeepEquals, expected)
}
