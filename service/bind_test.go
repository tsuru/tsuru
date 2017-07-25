// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/tsurutest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type BindSuite struct {
	conn *db.Storage
	user auth.User
	team auth.Team
}

var _ = check.Suite(&BindSuite{})

func TestT(t *testing.T) {
	check.TestingT(t)
}

func (s *BindSuite) SetUpSuite(c *check.C) {
	var err error
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "tsuru_service_bind_test")
	config.Set("routers:fake:type", "fake")
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	app.AuthScheme = auth.Scheme(native.NativeScheme{})
}

func (s *BindSuite) SetUpTest(c *check.C) {
	provisiontest.ProvisionerInstance.Reset()
	routertest.FakeRouter.Reset()
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.user = auth.User{Email: "sad-but-true@metallica.com"}
	err := s.user.Create()
	c.Assert(err, check.IsNil)
	s.team = auth.Team{Name: "metallica"}
	err = s.conn.Teams().Insert(s.team)
	c.Assert(err, check.IsNil)
	opts := provision.AddPoolOptions{Name: "pool1", Default: true, Provisioner: "fake"}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
}

func (s *BindSuite) TearDownSuite(c *check.C) {
	s.conn.Apps().Database.DropDatabase()
	s.conn.Close()
}

func (s *BindSuite) TestBindUnit(c *check.C) {
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	a := &app.App{Name: "painkiller", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(a, &s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "", nil)
	c.Assert(err, check.IsNil)
	units, err := a.GetUnits()
	c.Assert(err, check.IsNil)
	err = instance.BindUnit(a, units[0])
	c.Assert(err, check.IsNil)
	c.Assert(called, check.Equals, true)
}

func (s *BindSuite) TestBindAppFailsWhenEndpointIsDown(c *check.C) {
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": "wrong"}, Password: "s3cr3t"}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	a := &app.App{Name: "painkiller", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(a, &s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "", nil)
	c.Assert(err, check.IsNil)
	err = instance.BindApp(a, true, nil)
	c.Assert(err, check.NotNil)
}

func (s *BindSuite) TestBindAddsAppToTheServiceInstance(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	a := &app.App{Name: "painkiller", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(a, &s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "", nil)
	c.Assert(err, check.IsNil)
	err = instance.BindApp(a, true, nil)
	c.Assert(err, check.IsNil)
	s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(instance.Apps, check.DeepEquals, []string{a.GetName()})
}

func (s *BindSuite) TestBindAppMultiUnits(c *check.C) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
		atomic.AddInt32(&calls, 1)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "painkiller", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(a, &s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(2, "", nil)
	c.Assert(err, check.IsNil)
	err = instance.BindApp(a, true, nil)
	c.Assert(err, check.IsNil)
	err = tsurutest.WaitCondition(2*time.Second, func() bool {
		return atomic.LoadInt32(&calls) == 3
	})
	c.Assert(err, check.IsNil)
}

func (s *BindSuite) TestBindReturnConflictIfTheAppIsAlreadyBound(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	srvc := service.Service{Name: "mysql", Password: "s3cr3t", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "painkiller", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(a, &s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "", nil)
	c.Assert(err, check.IsNil)
	err = instance.BindApp(a, true, nil)
	c.Assert(err, check.Equals, service.ErrAppAlreadyBound)
}

func (s *BindSuite) TestBindAppWithNoUnits(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "painkiller", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(a, &s.user)
	c.Assert(err, check.IsNil)
	err = instance.BindApp(a, true, nil)
	c.Assert(err, check.IsNil)
	envs := a.Envs()
	c.Assert(envs["DATABASE_USER"], check.DeepEquals, bind.EnvVar{
		Name:         "DATABASE_USER",
		Value:        "root",
		Public:       false,
		InstanceName: "my-mysql",
	})
	c.Assert(envs["DATABASE_PASSWORD"], check.DeepEquals, bind.EnvVar{
		Name:         "DATABASE_PASSWORD",
		Value:        "s3cr3t",
		Public:       false,
		InstanceName: "my-mysql",
	})
}

func (s *BindSuite) TestUnbindUnit(c *check.C) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "painkiller", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(a, &s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "", nil)
	c.Assert(err, check.IsNil)
	units, err := a.GetUnits()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{a.GetName()},
		Units:       []string{units[0].GetID()},
	}
	instance.Create()
	units, err = a.GetUnits()
	c.Assert(err, check.IsNil)
	err = instance.UnbindUnit(a, units[0])
	c.Assert(err, check.IsNil)
	c.Assert(called, check.Equals, true)
	err = s.conn.ServiceInstances().Find(bson.M{"name": "my-mysql"}).One(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Units, check.HasLen, 0)
}

func (s *BindSuite) TestUnbindMultiUnits(c *check.C) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "painkiller", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(a, &s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(2, "", nil)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(
		bind.InstanceApp{
			ServiceName:   "mysql",
			Instance:      bind.ServiceInstance{Name: "my-mysql"},
			ShouldRestart: true,
		}, ioutil.Discard)
	c.Assert(err, check.IsNil)
	units, err := a.GetUnits()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{a.GetName()},
		Units:       []string{units[0].GetID(), units[1].GetID()},
	}
	instance.Create()
	err = instance.UnbindApp(a, true, nil)
	c.Assert(err, check.IsNil)
	err = tsurutest.WaitCondition(1e9, func() bool {
		return atomic.LoadInt32(&calls) > 1
	})
	c.Assert(err, check.IsNil)
	envs := a.Envs()
	c.Assert(envs, check.IsNil)
}

func (s *BindSuite) TestUnbindRemovesAppFromServiceInstance(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	instance.Create()
	a := &app.App{Name: "painkiller", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(a, &s.user)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(
		bind.InstanceApp{
			ServiceName:   "mysql",
			Instance:      bind.ServiceInstance{Name: "my-mysql"},
			ShouldRestart: true,
		}, ioutil.Discard)
	c.Assert(err, check.IsNil)
	err = instance.UnbindApp(a, true, nil)
	c.Assert(err, check.IsNil)
	s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(instance.Apps, check.DeepEquals, []string{})
}

func (s *BindSuite) TestUnbindCallsTheUnbindMethodFromAPI(c *check.C) {
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/bind" {
			atomic.StoreInt32(&called, 1)
		}
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "painkiller", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(a, &s.user)
	c.Assert(err, check.IsNil)
	err = a.AddUnits(1, "", nil)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(
		bind.InstanceApp{
			ServiceName:   "mysql",
			Instance:      bind.ServiceInstance{Name: "my-mysql"},
			ShouldRestart: true,
		}, ioutil.Discard)
	c.Assert(err, check.IsNil)
	units, err := a.GetUnits()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{a.GetName()},
		Units:       []string{units[0].GetID()},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	err = instance.UnbindApp(a, true, nil)
	c.Assert(err, check.IsNil)
	err = tsurutest.WaitCondition(1e9, func() bool {
		return atomic.LoadInt32(&called) > 0
	})
	c.Assert(err, check.IsNil)
}

func (s *BindSuite) TestUnbindReturnsPreconditionFailedIfTheAppIsNotBoundToTheInstance(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	a := &app.App{Name: "painkiller", Platform: "python", TeamOwner: s.team.Name}
	err = app.CreateApp(a, &s.user)
	c.Assert(err, check.IsNil)
	err = instance.UnbindApp(a, true, nil)
	c.Assert(err, check.Equals, service.ErrAppNotBound)
}
