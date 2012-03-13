package collector

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"path/filepath"
	"testing"
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

func (s *S) TestCollectorUpdate(c *C) {
	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()
	insertApp, _ := db.Prepare("INSERT INTO apps (id, name, state) VALUES (?, ?, ?)")

	tx, _ := db.Begin()
	stmt := tx.Stmt(insertApp)
	defer stmt.Close()
	stmt.Exec(1, "umaappqq", "STOPPED")
	tx.Commit()

	var collector Collector

	out := &output{
		Services:map[string]Service{
			"umaappqq":Service{
				Units:map[string]Unit{
					"umaappqq/0":Unit{
						State:"started"}}}}}

	collector.Update(db, out)

	rows, _ := db.Query("SELECT state FROM apps WHERE id = 1")

	var state string
	for rows.Next() {
		rows.Scan(&state)
	}

	c.Assert("RUNNING", DeepEquals, state)

}

func (s *S) TestCollectorParser(c *C) {
	var collector Collector

	file, _ := os.Open(filepath.Join("testdata", "output.yaml"))
	jujuOutput, _ := ioutil.ReadAll(file)
	file.Close()

	expected := &output{
		Services:map[string]Service{
			"umaappqq":Service{
				Units:map[string]Unit{
					"umaappqq/0":Unit{
						State:"started"}}}}}

	c.Assert(collector.Parse(jujuOutput), DeepEquals, expected)
}
