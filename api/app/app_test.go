package app_test

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/timeredbull/tsuru/api/app"
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

func (s *S) TestCreate(c *C) {
	app := app.App{}
	app.Name = "appName"
	app.Framework = "django"

	err := app.Create()
	c.Assert(err, IsNil)

	c.Assert(app.State, Equals, "Pending")

	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()
	rows, err := db.Query("SELECT name, framework, state FROM apps WHERE name = 'appName'")

	if err != nil {
		panic(err)
	}

	var state string
	var name string
	var framework string

	for rows.Next() {
		rows.Scan(&name, &framework, &state)
		rows.Scan(&framework)
		rows.Scan(&state)
	}

	c.Assert(name, Equals, app.Name)
	c.Assert(framework, Equals, app.Framework)
	c.Assert(state, Equals, app.State)

}
