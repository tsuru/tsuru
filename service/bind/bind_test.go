// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service_test

import (
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/service"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
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
	c.Assert(err, IsNil)
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru_service_bind_test")
	c.Assert(err, IsNil)
	s.user = auth.User{Email: "sad-but-true@metallica.com"}
	s.user.Create()
	s.team = auth.Team{Name: "metallica", Users: []string{s.user.Email}}
	db.Session.Teams().Insert(s.team)
}

func (s *S) TearDownSuite(c *C) {
	defer db.Session.Close()
	db.Session.Apps().Database.DropDatabase()
}

func createTestApp(name, framework string, teams []string, units []app.Unit) (app.App, error) {
	a := app.App{
		Name:      name,
		Framework: framework,
		Teams:     teams,
		Units:     units,
	}
	err := db.Session.Apps().Insert(&a)
	return a, err
}

func (s *S) TestBindUnit(c *C) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp("painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "10.10.10.10"}})
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	envs, err := instance.BindUnit(a.GetUnits()[0])
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
	expectedEnvs := map[string]string{
		"DATABASE_USER":     "root",
		"DATABASE_PASSWORD": "s3cr3t",
	}
	c.Assert(envs, DeepEquals, expectedEnvs)
}

func (s *S) TestBindAddsAppToTheServiceInstance(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp("painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "10.10.10.10"}})
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = instance.BindApp(&a)
	c.Assert(err, IsNil)
	db.Session.ServiceInstances().Find(bson.M{"_id": instance.Name}).One(&instance)
	c.Assert(instance.Apps, DeepEquals, []string{a.Name})
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
	}
	err = instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp("painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "127.0.0.1"}})
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = instance.BindApp(&a)
	c.Assert(err, IsNil)
	newApp := app.App{Name: a.Name}
	err = newApp.Get()
	c.Assert(err, IsNil)
	expectedEnv := map[string]bind.EnvVar{
		"DATABASE_USER": {
			Name:         "DATABASE_USER",
			Value:        "root",
			Public:       false,
			InstanceName: instance.Name,
		},
		"DATABASE_PASSWORD": {
			Name:         "DATABASE_PASSWORD",
			Value:        "s3cr3t",
			Public:       false,
			InstanceName: instance.Name,
		},
	}
	c.Assert(a.Env, DeepEquals, expectedEnv)
}

func (s *S) TestBindMultiUnits(c *C) {
	var calls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
		calls++
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
	}
	err = instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp("painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "127.0.0.1"}, {Ip: "128.0.0.1"}})
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = instance.BindApp(&a)
	c.Assert(err, IsNil)
	ok := make(chan bool)
	go func() {
		for {
			if calls == 2 {
				ok <- true
			}
		}
	}()
	select {
	case <-ok:
	case <-time.After(2e9):
		c.Errorf("Did not bind all units afters 2s.")
	}
	c.Assert(calls, Equals, 2)
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
		Apps:        []string{"painkiller"},
	}
	err = instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp("painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "127.0.0.1"}})
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = instance.BindApp(&a)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusConflict)
	c.Assert(e, ErrorMatches, "^This app is already binded to this service instance.$")
}

func (s *S) TestBindReturnsPreconditionFailedIfTheAppDoesNotHaveAnUnitAndServiceHasEndpoint(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp("painkiller", "", []string{s.team.Name}, []app.Unit{})
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = instance.BindApp(&a)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusPreconditionFailed)
	c.Assert(e.Message, Equals, "This app does not have an IP yet.")
}

func (s *S) TestUnbindUnit(c *C) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
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
		Apps:        []string{"painkiller"},
	}
	instance.Create()
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp("painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "10.10.10.10"}})
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = instance.UnbindUnit(a.GetUnits()[0])
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
}

func (s *S) TestUnbindMultiUnits(c *C) {
	var calls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
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
		Apps:        []string{"painkiller"},
	}
	instance.Create()
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp("painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "10.10.10.10"}, {Ip: "9.9.9.9"}})
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = instance.UnbindApp(&a)
	c.Assert(err, IsNil)
	db.Session.ServiceInstances().Find(bson.M{"_id": instance.Name}).One(&instance)
	ok := make(chan bool, 1)
	go func() {
		for {
			if c.Check(calls, Equals, 2) {
				ok <- true
				return
			}
		}
	}()
	select {
	case <-ok:
		c.SucceedNow()
	case <-time.After(1 * time.Second):
		c.Error("endpoint not called")
	}
}

func (s *S) TestUnbindRemovesAppFromServiceInstance(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
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
		Apps:        []string{"painkiller"},
	}
	instance.Create()
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp("painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "10.10.10.10"}})
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = instance.UnbindApp(&a)
	c.Assert(err, IsNil)
	db.Session.ServiceInstances().Find(bson.M{"_id": instance.Name}).One(&instance)
	c.Assert(instance.Apps, DeepEquals, []string{})
}

func (s *S) TestUnbindRemovesEnvironmentVariableFromApp(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
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
		Apps:        []string{"painkiller"},
	}
	err = instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a := app.App{
		Name:  "painkiller",
		Teams: []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST": {
				Name:         "DATABASE_HOST",
				Value:        "arrea",
				Public:       false,
				InstanceName: instance.Name,
			},
			"MY_VAR": {
				Name:  "MY_VAR",
				Value: "123",
			},
		},
		Units: []app.Unit{
			{
				Ip: "10.10.10.10",
			},
		},
	}
	err = db.Session.Apps().Insert(&a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = instance.UnbindApp(&a)
	c.Assert(err, IsNil)
	newApp := app.App{Name: a.Name}
	err = newApp.Get()
	c.Assert(err, IsNil)
	expected := map[string]bind.EnvVar{
		"MY_VAR": {
			Name:  "MY_VAR",
			Value: "123",
		},
	}
	c.Assert(a.Env, DeepEquals, expected)
}

func (s *S) TestUnbindCallsTheUnbindMethodFromAPI(c *C) {
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/hostname/127.0.0.1" {
			atomic.StoreInt32(&called, 1)
		}
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
		Apps:        []string{"painkiller"},
	}
	err = instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp("painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "127.0.0.1"}})
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = instance.UnbindApp(&a)
	c.Assert(err, IsNil)
	ch := make(chan bool)
	go func() {
		t := time.Tick(1)
		for _ = <-t; atomic.LoadInt32(&called) == 0; _ = <-t {
		}
		ch <- true
	}()
	select {
	case <-ch:
		c.SucceedNow()
	case <-time.After(1e9):
		c.Errorf("Failed to call API after 1 second.")
	}
}

func (s *S) TestUnbindReturnsPreconditionFailedIfTheAppIsNotBindedToTheInstance(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp("painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "10.10.10.10"}})
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = instance.UnbindApp(&a)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e, ErrorMatches, "^This app is not binded to this service instance.$")
}
