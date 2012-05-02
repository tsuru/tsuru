package app

import (
	"fmt"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/db"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo"
	"launchpad.net/mgo/bson"
	"os"
	"path"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	session *mgo.Session
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	var err error
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru_app_test")
	c.Assert(err, IsNil)
}

func (s *S) TearDownSuite(c *C) {
	defer db.Session.Close()
	db.Session.DropDB()
}

func (s *S) TearDownTest(c *C) {
	err := db.Session.Apps().RemoveAll(nil)
	c.Assert(err, IsNil)
}

func (s *S) TestNewRepository(c *C) {
	a := App{Name: "foobar"}
	err := NewRepository(&a)
	c.Assert(err, IsNil)

	repoPath := GetRepositoryPath(&a)
	_, err = os.Open(repoPath) // test if repository dir exists
	c.Assert(err, IsNil)

	_, err = os.Open(path.Join(repoPath, "config"))
	c.Assert(err, IsNil)

	err = os.RemoveAll(repoPath)
	c.Assert(err, IsNil)
}

func (s *S) TestDeleteGitRepository(c *C) {
	a := &App{Name: "someApp"}
	repoPath := GetRepositoryPath(a)

	err := NewRepository(a)
	c.Assert(err, IsNil)

	_, err = os.Open(path.Join(repoPath, "config"))
	c.Assert(err, IsNil)

	DeleteRepository(a)
	_, err = os.Open(repoPath)
	c.Assert(err, NotNil)
}

func (s *S) TestGetRepositoryUrl(c *C) {
	a := App{Name: "foobar"}
	url := GetRepositoryUrl(&a)
	expected := fmt.Sprintf("git@tsuru.plataformas.glb.com:%s.git", a.Name)
	c.Assert(url, Equals, expected)
}

func (s *S) TestGetRepositoryName(c *C) {
	a := App{Name: "someApp"}
	obtained := GetRepositoryName(&a)
	expected := fmt.Sprintf("%s.git", a.Name)
	c.Assert(obtained, Equals, expected)
}

func (s *S) TestGetRepositoryPath(c *C) {
	a := App{Name: "someApp"}
	home := os.Getenv("HOME")
	obtained := GetRepositoryPath(&a)
	expected := path.Join(home, "../git", GetRepositoryName(&a))
	c.Assert(obtained, Equals, expected)
}

func (s *S) TestAll(c *C) {
	expected := make([]App, 0)
	app1 := App{Name: "app1", Teams: []auth.Team{}}
	app1.Create()
	expected = append(expected, app1)
	app2 := App{Name: "app2", Teams: []auth.Team{}}
	app2.Create()
	expected = append(expected, app2)
	app3 := App{Name: "app3", Teams: []auth.Team{}}
	app3.Create()
	expected = append(expected, app3)

	appList, err := AllApps()
	c.Assert(err, IsNil)
	c.Assert(expected, DeepEquals, appList)

	app1.Destroy()
	app2.Destroy()
	app3.Destroy()
}

func (s *S) TestGet(c *C) {
	newApp := App{Name: "myApp", Framework: "django", Teams: []auth.Team{}}
	err := newApp.Create()
	c.Assert(err, IsNil)

	myApp := App{Name: "myApp"}
	err = myApp.Get()
	c.Assert(err, IsNil)
	c.Assert(myApp, DeepEquals, newApp)

	err = myApp.Destroy()
	c.Assert(err, IsNil)
}

func (s *S) TestDestroy(c *C) {
	a := App{
		Name:      "aName",
		Framework: "django",
	}

	err := a.Create()
	c.Assert(err, IsNil)
	err = a.Destroy()
	c.Assert(err, IsNil)

	qtd, err := db.Session.Apps().Find(nil).Count()
	c.Assert(err, IsNil)
	c.Assert(qtd, Equals, 0)
}

func (s *S) TestCreate(c *C) {
	a := App{}
	a.Name = "appName"
	a.Framework = "django"

	err := a.Create()
	c.Assert(err, IsNil)

	repoPath := GetRepositoryPath(&a)
	_, err = os.Open(repoPath) // test if repository dir exists
	c.Assert(err, IsNil)

	c.Assert(a.State, Equals, "Pending")

	var retrievedApp App
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&retrievedApp)
	c.Assert(err, IsNil)
	c.Assert(retrievedApp.Name, Equals, a.Name)
	c.Assert(retrievedApp.Framework, Equals, a.Framework)
	c.Assert(retrievedApp.State, Equals, a.State)

	a.Destroy()

	_, err = os.Open(repoPath)
	c.Assert(err, NotNil) // ensures that repository dir has been deleted
}

func (s *S) TestCantCreateTwoAppsWithTheSameName(c *C) {
	a := App{Name: "appName", Framework: "django"}
	err := a.Create()
	c.Assert(err, IsNil)

	err = a.Create()
	c.Assert(err, NotNil)

	a.Destroy()
}
