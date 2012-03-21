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

func (s *S) TestGet(c *C) {
	newApp := app.App{Name: "myApp", Framework: "django"}
	err := newApp.Create()
	c.Assert(err, IsNil)

	myApp := app.App{Name: "myApp"}
	err = myApp.Get()
	c.Assert(err, IsNil)
	c.Assert(myApp, Equals, newApp)

	err = myApp.Destroy()
	c.Assert(err, IsNil)
}

func (s *S) TestDestroy(c *C) {
	app := app.App{}
	app.Name = "appName"
	app.Framework = "django"

	err := app.Create()
	c.Assert(err, IsNil)

	err = app.Destroy()
	c.Assert(err, IsNil)

	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()
	rows, err := db.Query("SELECT count(*) FROM apps WHERE name = 'appName'")

	if err != nil {
		panic(err)
	}

	var qtd int

	for rows.Next() {
		rows.Scan(&qtd)
	}

	c.Assert(qtd, Equals, 0)
}

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
	}

	c.Assert(name, Equals, app.Name)
	c.Assert(framework, Equals, app.Framework)
	c.Assert(state, Equals, app.State)

	app.Destroy()
}
