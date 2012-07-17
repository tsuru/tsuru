package service_test

import (
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/errors"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
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
	err := instance.Bind(&a)
	c.Assert(err, IsNil)
	db.Session.ServiceInstances().Find(bson.M{"_id": instance.Name}).One(&instance)
	c.Assert(instance.Apps, DeepEquals, []string{a.Name})
}

func (s *S) TestBindAddsAllEnvironmentVariablesFromServiceInstanceToTheApp(c *C) {
	srvc := service.Service{Name: "mysql"}
	err := srvc.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Env:         map[string]string{"DATABASE_NAME": "mymysql", "DATABASE_HOST": "localhost"},
		State:       "running",
	}
	err = instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a := app.App{Name: "painkiller", Teams: []string{s.team.Name}}
	err = a.Create()
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = instance.Bind(&a)
	c.Assert(err, IsNil)
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(err, IsNil)
	expectedEnv := map[string]app.EnvVar{
		"DATABASE_NAME": app.EnvVar{
			Name:         "DATABASE_NAME",
			Value:        "mymysql",
			Public:       false,
			InstanceName: instance.Name,
		},
		"DATABASE_HOST": app.EnvVar{
			Name:         "DATABASE_HOST",
			Value:        "localhost",
			Public:       false,
			InstanceName: instance.Name,
		},
	}
	c.Assert(a.Env, DeepEquals, expectedEnv)
}

func (s *S) TestBindCallTheServiceAPIAndSetsEnvironmentVariableReturnedFromTheCall(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Env:         map[string]string{"DATABASE_NAME": "mymysql", "DATABASE_HOST": "localhost"},
		State:       "running",
	}
	err = instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a := app.App{
		Name:  "painkiller",
		Teams: []string{s.team.Name},
		Units: []unit.Unit{unit.Unit{Ip: "127.0.0.1"}},
	}
	err = a.Create()
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = instance.Bind(&a)
	c.Assert(err, IsNil)
	err = db.Session.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(err, IsNil)
	expectedEnv := map[string]app.EnvVar{
		"DATABASE_NAME": app.EnvVar{
			Name:         "DATABASE_NAME",
			Value:        "mymysql",
			Public:       false,
			InstanceName: instance.Name,
		},
		"DATABASE_HOST": app.EnvVar{
			Name:         "DATABASE_HOST",
			Value:        "localhost",
			Public:       false,
			InstanceName: instance.Name,
		},
		"DATABASE_USER": app.EnvVar{
			Name:         "DATABASE_USER",
			Value:        "root",
			Public:       false,
			InstanceName: instance.Name,
		},
		"DATABASE_PASSWORD": app.EnvVar{
			Name:         "DATABASE_PASSWORD",
			Value:        "s3cr3t",
			Public:       false,
			InstanceName: instance.Name,
		},
	}
	c.Assert(a.Env, DeepEquals, expectedEnv)
}

func (s *S) TestBindReturnConflictIfTheAppIsAlreadyBinded(c *C) {
	srvc := service.Service{Name: "mysql"}
	err := srvc.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		State:       "running",
		Apps:        []string{"painkiller"},
	}
	err = instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a := app.App{
		Name:  "painkiller",
		Teams: []string{s.team.Name},
		Units: []unit.Unit{unit.Unit{Ip: "127.0.0.1"}},
	}
	err = a.Create()
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = instance.Bind(&a)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusConflict)
	c.Assert(e, ErrorMatches, "^This app is already binded to this service instance.$")
}
