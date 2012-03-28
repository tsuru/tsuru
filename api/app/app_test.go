package app_test

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/timeredbull/tsuru/api/app"
	. "github.com/timeredbull/tsuru/database"
	. "launchpad.net/gocheck"
	"os"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	Db, _ = sql.Open("sqlite3", "./tsuru.db")
	_, err := Db.Exec("CREATE TABLE 'apps' ('id' INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, 'name' varchar(255), 'framework' varchar(255), 'state' varchar(255), ip varchar(100))")
	c.Check(err, IsNil)
}

func (s *S) TearDownSuite(c *C) {
	os.Remove("./tsuru.db")
	Db.Close()
}

func (s *S) TearDownTest(c *C) {
	Db.Exec("DELETE FROM apps")
}

func (s *S) TestAll(c *C) {
	expected := make([]app.App, 0)
	app1 := app.App{Name: "app1"}
	app1.Create()
	expected = append(expected, app1)
	app2 := app.App{Name: "app2"}
	app2.Create()
	expected = append(expected, app2)
	app3 := app.App{Name: "app3"}
	app3.Create()
	expected = append(expected, app3)

	appList, err := app.AllApps()
	c.Assert(err, IsNil)
	c.Assert(expected, DeepEquals, appList)

	app1.Destroy()
	app2.Destroy()
	app3.Destroy()
}

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

	rows, err := Db.Query("SELECT count(*) FROM apps WHERE name = 'appName'")
	c.Assert(err, IsNil)

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
	c.Assert(app.Id, Not(Equals), int64(0))

	rows, err := Db.Query("SELECT id, name, framework, state FROM apps WHERE name = 'appName'")
	c.Assert(err, IsNil)

	var state string
	var name string
	var framework string
	var id int

	for rows.Next() {
		rows.Scan(&id, &name, &framework, &state)
	}

	c.Assert(id, Equals, int(app.Id))
	c.Assert(name, Equals, app.Name)
	c.Assert(framework, Equals, app.Framework)
	c.Assert(state, Equals, app.State)

	app.Destroy()
}
