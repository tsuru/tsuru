// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service_test

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/service"
	ttesting "github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type S struct {
	conn   *db.Storage
	user   auth.User
	team   auth.Team
	tmpdir string
}

var _ = gocheck.Suite(&S{})

func TestT(t *testing.T) {
	gocheck.TestingT(t)
}

func (s *S) SetUpSuite(c *gocheck.C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_service_bind_test")
	config.Set("auth:salt", "test_salt")
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
	s.user = auth.User{Email: "sad-but-true@metallica.com"}
	s.user.Create()
	s.team = auth.Team{Name: "metallica", Users: []string{s.user.Email}}
	s.conn.Teams().Insert(s.team)
	app.Provisioner = ttesting.NewFakeProvisioner()
}

func (s *S) TearDownSuite(c *gocheck.C) {
	s.conn.Apps().Database.DropDatabase()
}

func createTestApp(conn *db.Storage, name, framework string, teams []string, units []app.Unit) (app.App, error) {
	a := app.App{
		Name:     name,
		Platform: framework,
		Teams:    teams,
		Units:    units,
	}
	err := conn.Apps().Insert(&a)
	return a, err
}

func (s *S) TestBindUnit(c *gocheck.C) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "10.10.10.10"}})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	envs, err := instance.BindUnit(&a, a.GetUnits()[0])
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
	expectedEnvs := map[string]string{
		"DATABASE_USER":     "root",
		"DATABASE_PASSWORD": "s3cr3t",
	}
	c.Assert(envs, gocheck.DeepEquals, expectedEnvs)
}

func (s *S) TestBindAddsWhenEndpointIsDown(c *gocheck.C) {
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ""}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "10.10.10.10"}})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = instance.BindApp(&a)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestBindAddsAppToTheServiceInstance(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "10.10.10.10"}})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = instance.BindApp(&a)
	c.Assert(err, gocheck.IsNil)
	s.conn.ServiceInstances().Find(bson.M{"_id": instance.Name}).One(&instance)
	c.Assert(instance.Apps, gocheck.DeepEquals, []string{a.Name})
}

func (s *S) TestBindCallTheServiceAPIAndSetsEnvironmentVariableReturnedFromTheCall(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "127.0.0.1"}})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = instance.BindApp(&a)
	c.Assert(err, gocheck.IsNil)
	newApp := app.App{Name: a.Name}
	err = newApp.Get()
	c.Assert(err, gocheck.IsNil)
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
	c.Assert(a.Env, gocheck.DeepEquals, expectedEnv)
}

func (s *S) TestBindAppMultiUnits(c *gocheck.C) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
		calls++
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "127.0.0.1"}, {Ip: "128.0.0.1"}})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = instance.BindApp(&a)
	c.Assert(err, gocheck.IsNil)
	ok := make(chan bool)
	go func() {
		t := time.Tick(1)
		for _ = <-t; atomic.LoadInt32(&calls) < 2; _ = <-t {
		}
		ok <- true
	}()
	select {
	case <-ok:
	case <-time.After(2e9):
		c.Errorf("Did not bind all units afters 2s.")
	}
	c.Assert(calls, gocheck.Equals, int32(2))
}

func (s *S) TestBindReturnConflictIfTheAppIsAlreadyBound(c *gocheck.C) {
	srvc := service.Service{Name: "mysql"}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	err = instance.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "127.0.0.1"}})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = instance.BindApp(&a)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusConflict)
	c.Assert(e, gocheck.ErrorMatches, "^This app is already bound to this service instance.$")
}

func (s *S) TestBindReturnsPreconditionFailedIfTheAppDoesNotHaveAnUnitAndServiceHasEndpoint(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = instance.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name}, []app.Unit{})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = instance.BindApp(&a)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusPreconditionFailed)
	c.Assert(e.Message, gocheck.Equals, "This app does not have an IP yet.")
}

func (s *S) TestUnbindUnit(c *gocheck.C) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "10.10.10.10"}})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = instance.UnbindUnit(a.GetUnits()[0])
	c.Assert(err, gocheck.IsNil)
	c.Assert(called, gocheck.Equals, true)
}

func (s *S) TestUnbindMultiUnits(c *gocheck.C) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := atomic.LoadInt32(&calls)
		i++
		atomic.StoreInt32(&calls, i)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "10.10.10.10"}, {Ip: "9.9.9.9"}})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = instance.UnbindApp(&a)
	c.Assert(err, gocheck.IsNil)
	s.conn.ServiceInstances().Find(bson.M{"_id": instance.Name}).One(&instance)
	ok := make(chan bool, 1)
	go func() {
		t := time.Tick(1)
		for _ = <-t; atomic.LoadInt32(&calls) < 2; _ = <-t {
		}
		ok <- true
	}()
	select {
	case <-ok:
		c.SucceedNow()
	case <-time.After(1 * time.Second):
		c.Error("endpoint not called")
	}
}

func (s *S) TestUnbindRemovesAppFromServiceInstance(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "10.10.10.10"}})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = instance.UnbindApp(&a)
	c.Assert(err, gocheck.IsNil)
	s.conn.ServiceInstances().Find(bson.M{"_id": instance.Name}).One(&instance)
	c.Assert(instance.Apps, gocheck.DeepEquals, []string{})
}

func (s *S) TestUnbindRemovesEnvironmentVariableFromApp(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	err = instance.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
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
	err = s.conn.Apps().Insert(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = instance.UnbindApp(&a)
	c.Assert(err, gocheck.IsNil)
	newApp := app.App{Name: a.Name}
	err = newApp.Get()
	c.Assert(err, gocheck.IsNil)
	expected := map[string]bind.EnvVar{
		"MY_VAR": {
			Name:  "MY_VAR",
			Value: "123",
		},
	}
	c.Assert(a.Env, gocheck.DeepEquals, expected)
}

func (s *S) TestUnbindCallsTheUnbindMethodFromAPI(c *gocheck.C) {
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/hostname/127.0.0.1" {
			atomic.StoreInt32(&called, 1)
		}
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	err = instance.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "127.0.0.1"}})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = instance.UnbindApp(&a)
	c.Assert(err, gocheck.IsNil)
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

func (s *S) TestUnbindReturnsPreconditionFailedIfTheAppIsNotBoundToTheInstance(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name}, []app.Unit{{Ip: "10.10.10.10"}})
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = instance.UnbindApp(&a)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e, gocheck.ErrorMatches, "^This app is not bound to this service instance.$")
}
