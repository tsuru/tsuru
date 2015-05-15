// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type BindSuite struct {
	conn   *db.Storage
	user   auth.User
	team   auth.Team
	tmpdir string
}

var _ = check.Suite(&BindSuite{})

func TestT(t *testing.T) {
	check.TestingT(t)
}

func (s *BindSuite) SetUpSuite(c *check.C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_service_bind_test")
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	s.user = auth.User{Email: "sad-but-true@metallica.com"}
	s.user.Create()
	s.team = auth.Team{Name: "metallica", Users: []string{s.user.Email}}
	s.conn.Teams().Insert(s.team)
	app.Provisioner = provisiontest.NewFakeProvisioner()
}

func (s *BindSuite) TearDownSuite(c *check.C) {
	dbtest.ClearAllCollections(s.conn.Apps().Database)
}

func createTestApp(conn *db.Storage, name, framework string, teams []string) (app.App, error) {
	a := app.App{
		Name:     name,
		Platform: framework,
		Teams:    teams,
	}
	err := conn.Apps().Insert(&a)
	return a, err
}

func (s *BindSuite) TestBindUnit(c *check.C) {
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	app.Provisioner.Provision(&a)
	defer app.Provisioner.Destroy(&a)
	app.Provisioner.AddUnits(&a, 1, "web", nil)
	err = instance.BindUnit(&a, a.GetUnits()[0])
	c.Assert(err, check.IsNil)
	c.Assert(called, check.Equals, true)
}

func (s *BindSuite) TestBindAppFailsWhenEndpointIsDown(c *check.C) {
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ""}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	app.Provisioner.Provision(&a)
	defer app.Provisioner.Destroy(&a)
	app.Provisioner.AddUnits(&a, 1, "web", nil)
	err = instance.BindApp(&a, nil)
	c.Assert(err, check.NotNil)
}

func (s *BindSuite) TestBindAddsAppToTheServiceInstance(c *check.C) {
	fakeProvisioner := app.Provisioner.(*provisiontest.FakeProvisioner)
	fakeProvisioner.PrepareOutput([]byte("exported"))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = instance.BindApp(&a, nil)
	c.Assert(err, check.IsNil)
	s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(instance.Apps, check.DeepEquals, []string{a.Name})
}

func (s *BindSuite) TestBindCallTheServiceAPIAndSetsEnvironmentVariableReturnedFromTheCall(c *check.C) {
	fakeProvisioner := app.Provisioner.(*provisiontest.FakeProvisioner)
	fakeProvisioner.PrepareOutput([]byte("exported"))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	app.Provisioner.Provision(&a)
	defer app.Provisioner.Destroy(&a)
	app.Provisioner.AddUnits(&a, 1, "web", nil)
	err = instance.BindApp(&a, nil)
	c.Assert(err, check.IsNil)
	newApp, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
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
	expectedTsuruServices := map[string][]bind.ServiceInstance{
		"mysql": {
			bind.ServiceInstance{
				Name: instance.Name,
				Envs: map[string]string{"DATABASE_USER": "root", "DATABASE_PASSWORD": "s3cr3t"},
			},
		},
	}
	servicesEnv := newApp.Env[app.TsuruServicesEnvVar]
	var tsuruServices map[string][]bind.ServiceInstance
	json.Unmarshal([]byte(servicesEnv.Value), &tsuruServices)
	c.Assert(tsuruServices, check.DeepEquals, expectedTsuruServices)
	delete(newApp.Env, app.TsuruServicesEnvVar)
	c.Assert(newApp.Env, check.DeepEquals, expectedEnv)
}

func (s *BindSuite) TestBindAppMultiUnits(c *check.C) {
	fakeProvisioner := app.Provisioner.(*provisiontest.FakeProvisioner)
	fakeProvisioner.PrepareOutput([]byte("exported"))
	fakeProvisioner.PrepareOutput([]byte("exported"))
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
		calls++
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	app.Provisioner.Provision(&a)
	defer app.Provisioner.Destroy(&a)
	app.Provisioner.AddUnits(&a, 1, "web", nil)
	err = instance.BindApp(&a, nil)
	c.Assert(err, check.IsNil)
	ok := make(chan bool)
	go func() {
		t := time.Tick(1)
		for _ = <-t; atomic.LoadInt32(&calls) < 1; _ = <-t {
		}
		ok <- true
	}()
	select {
	case <-ok:
	case <-time.After(2e9):
		c.Errorf("Did not bind all units afters 2s.")
	}
	c.Assert(calls, check.Equals, int32(2))
}

func (s *BindSuite) TestBindReturnConflictIfTheAppIsAlreadyBound(c *check.C) {
	srvc := service.Service{Name: "mysql"}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = instance.BindApp(&a, nil)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusConflict)
	c.Assert(e, check.ErrorMatches, "^This app is already bound to this service instance.$")
}

func (s *BindSuite) TestBindAppWithNoUnits(c *check.C) {
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = instance.BindApp(&a, nil)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(err, check.IsNil)
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
	expectedTsuruServices := map[string][]bind.ServiceInstance{
		"mysql": {
			bind.ServiceInstance{
				Name: instance.Name,
				Envs: map[string]string{"DATABASE_USER": "root", "DATABASE_PASSWORD": "s3cr3t"},
			},
		},
	}
	servicesEnv := a.Env[app.TsuruServicesEnvVar]
	var tsuruServices map[string][]bind.ServiceInstance
	json.Unmarshal([]byte(servicesEnv.Value), &tsuruServices)
	c.Assert(tsuruServices, check.DeepEquals, expectedTsuruServices)
	delete(a.Env, app.TsuruServicesEnvVar)
	c.Assert(a.Env, check.DeepEquals, expectedEnv)
}

func (s *BindSuite) TestUnbindUnit(c *check.C) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	app.Provisioner.Provision(&a)
	defer app.Provisioner.Destroy(&a)
	app.Provisioner.AddUnits(&a, 1, "web", nil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
		Units:       []string{a.GetUnits()[0].GetName()},
	}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	err = instance.UnbindUnit(&a, a.GetUnits()[0])
	c.Assert(err, check.IsNil)
	c.Assert(called, check.Equals, true)
	err = s.conn.ServiceInstances().Find(bson.M{"name": "my-mysql"}).One(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Units, check.HasLen, 0)
}

func (s *BindSuite) TestUnbindMultiUnits(c *check.C) {
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
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	app.Provisioner.Provision(&a)
	defer app.Provisioner.Destroy(&a)
	units, _ := app.Provisioner.AddUnits(&a, 2, "web", nil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
		Units:       []string{units[0].Name, units[1].Name},
	}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	err = instance.UnbindApp(&a, nil)
	c.Assert(err, check.IsNil)
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

func (s *BindSuite) TestUnbindRemovesAppFromServiceInstance(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = app.Provisioner.Provision(&a)
	c.Assert(err, check.IsNil)
	defer app.Provisioner.Destroy(&a)
	err = instance.UnbindApp(&a, nil)
	c.Assert(err, check.IsNil)
	s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(instance.Apps, check.DeepEquals, []string{})
}

func (s *BindSuite) TestUnbindRemovesEnvironmentVariableFromApp(c *check.C) {
	fakeProvisioner := app.Provisioner.(*provisiontest.FakeProvisioner)
	fakeProvisioner.PrepareOutput([]byte("exported"))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
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
			app.TsuruServicesEnvVar: {
				Name: app.TsuruServicesEnvVar,
				Value: `{"mysql": [{"instance_name": "my-mysql", "envs": {"DATABASE_USER": "root", "DATABASE_PASSWORD": "s3cre3t"}},
					               {"instance_name": "other-mysql", "envs": {"DATABASE_USER": "1", "DATABASE_PASSWORD": "2"}}]}`,
			},
		},
	}
	err = s.conn.Apps().Insert(&a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = app.Provisioner.Provision(&a)
	c.Assert(err, check.IsNil)
	defer app.Provisioner.Destroy(&a)
	err = instance.UnbindApp(&a, nil)
	c.Assert(err, check.IsNil)
	newApp, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	services := newApp.Env[app.TsuruServicesEnvVar].Value
	var tsuruServices map[string][]bind.ServiceInstance
	err = json.Unmarshal([]byte(services), &tsuruServices)
	c.Assert(err, check.IsNil)
	c.Assert(tsuruServices, check.DeepEquals, map[string][]bind.ServiceInstance{
		"mysql": {
			{
				Name: "other-mysql",
				Envs: map[string]string{"DATABASE_USER": "1", "DATABASE_PASSWORD": "2"},
			},
		},
	})
	delete(newApp.Env, app.TsuruServicesEnvVar)
	expected := map[string]bind.EnvVar{
		"MY_VAR": {
			Name:  "MY_VAR",
			Value: "123",
		},
	}
	c.Assert(newApp.Env, check.DeepEquals, expected)
}

func (s *BindSuite) TestUnbindCallsTheUnbindMethodFromAPI(c *check.C) {
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/bind" {
			atomic.StoreInt32(&called, 1)
		}
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	app.Provisioner.Provision(&a)
	defer app.Provisioner.Destroy(&a)
	units, _ := app.Provisioner.AddUnits(&a, 1, "web", nil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
		Units:       []string{units[0].Name},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	err = instance.UnbindApp(&a, nil)
	c.Assert(err, check.IsNil)
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

func (s *BindSuite) TestUnbindReturnsPreconditionFailedIfTheAppIsNotBoundToTheInstance(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	a, err := createTestApp(s.conn, "painkiller", "", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = instance.UnbindApp(&a, nil)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e, check.ErrorMatches, "^This app is not bound to this service instance.$")
}
