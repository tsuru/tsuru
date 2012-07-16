package service_test

import (
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"testing"
)

type S struct {
	user   auth.User
	team   auth.Team
	tmpdir string
}

var _ = Suite(&S{})

func TestT(t *testing.T) {
	TestingT(t)
}

func (s *S) SetUpSuite(c *C) {
	var err error
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru_service_bind_test")
	c.Assert(err, IsNil)
	s.user = auth.User{Email: "sad-but-true@metallica.com"}
	s.user.Create()
	s.team = auth.Team{Name: "metallica", Users: []auth.User{s.user}}
	db.Session.Teams().Insert(s.team)
}

func (s *S) TearDownSuite(c *C) {
	defer commandmocker.Remove(s.tmpdir)
	defer db.Session.Close()
	db.Session.Apps().Database.DropDatabase()
}

func (s *S) TestBindAddsAppToTheServiceInstance(c *C) {
	srvc := service.Service{Name: "mysql"}
	srvc.Create()
	defer srvc.Delete()
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}, State: "running"}
	instance.Create()
	defer instance.Delete()
	a := app.App{Name: "painkiller", Teams: []string{s.team.Name}}
	a.Create()
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err := instance.Bind(&a, &s.user)
	c.Assert(err, IsNil)
	db.Session.ServiceInstances().Find(bson.M{"_id": instance.Name}).One(&instance)
	c.Assert(instance.Apps, DeepEquals, []string{a.Name})
}
