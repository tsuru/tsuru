package app_test

import (
	"github.com/timeredbull/tsuru/api/app"
	. "github.com/timeredbull/tsuru/database"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{
	session *mgo.Session
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	var err error
	s.session, err = mgo.Dial("localhost:27017")
	c.Assert(err, IsNil)
	Mdb = s.session.DB("tsuru_test")
	c.Assert(err, IsNil)
}

func (s *S) TearDownSuite(c *C) {
	err := Mdb.DropDatabase()
	c.Assert(err, IsNil)
	s.session.Close()
}

func (s *S) TearDownTest(c *C) {
	err := Mdb.C("apps").DropCollection()
	c.Assert(err, IsNil)
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

	collection := Mdb.C("apps")
	qtd, err := collection.Find(nil).Count()
	c.Assert(err, IsNil)

	c.Assert(qtd, Equals, 0)
}

func (s *S) TestCreate(c *C) {
	a := app.App{}
	a.Name = "appName"
	a.Framework = "django"

	err := a.Create()
	c.Assert(err, IsNil)

	c.Assert(a.State, Equals, "Pending")
	c.Assert(a.Id, Not(Equals), "")

	collection := Mdb.C("apps")
	var retrievedApp app.App

	query := make(map[string]interface{})
	query["name"] = a.Name

	err = collection.Find(query).One(&retrievedApp)

	c.Assert(err, IsNil)
	c.Assert(retrievedApp.Id, Equals, a.Id)
	c.Assert(retrievedApp.Name, Equals, a.Name)
	c.Assert(retrievedApp.Framework, Equals, a.Framework)
	c.Assert(retrievedApp.State, Equals, a.State)

	a.Destroy()
}
