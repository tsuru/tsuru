// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/tsurutest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

const tsuruServicesEnvVar = "TSURU_SERVICES"

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
}

func (s *BindSuite) SetUpTest(c *check.C) {
	routertest.FakeRouter.Reset()
	dbtest.ClearAllCollections(s.conn.Apps().Database)
	s.user = auth.User{Email: "sad-but-true@metallica.com"}
	s.user.Create()
	s.team = auth.Team{Name: "metallica"}
	s.conn.Teams().Insert(s.team)
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
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	app := provisiontest.NewFakeApp("painkiller", "python", 1)
	units, err := app.GetUnits()
	c.Assert(err, check.IsNil)
	err = instance.BindUnit(app, units[0])
	c.Assert(err, check.IsNil)
	c.Assert(called, check.Equals, true)
}

func (s *BindSuite) TestBindAppFailsWhenEndpointIsDown(c *check.C) {
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ""}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	app := provisiontest.NewFakeApp("painkiller", "python", 1)
	c.Assert(err, check.IsNil)
	err = instance.BindApp(app, true, nil)
	c.Assert(err, check.NotNil)
}

func (s *BindSuite) TestBindAddsAppToTheServiceInstance(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	app := provisiontest.NewFakeApp("painkiller", "python", 1)
	err = instance.BindApp(app, true, nil)
	c.Assert(err, check.IsNil)
	s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(instance.Apps, check.DeepEquals, []string{app.GetName()})
}

func (s *BindSuite) TestBindAppMultiUnits(c *check.C) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
		atomic.AddInt32(&calls, 1)
	}))
	defer ts.Close()
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	app := provisiontest.NewFakeApp("painkiller", "python", 2)
	err = instance.BindApp(app, true, nil)
	c.Assert(err, check.IsNil)
	err = tsurutest.WaitCondition(2e9, func() bool {
		return atomic.LoadInt32(&calls) == 3
	})
	c.Assert(err, check.IsNil)
}

func (s *BindSuite) TestBindReturnConflictIfTheAppIsAlreadyBound(c *check.C) {
	srvc := Service{Name: "mysql"}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	app := provisiontest.NewFakeApp("painkiller", "python", 1)
	err = instance.BindApp(app, true, nil)
	c.Assert(err, check.Equals, ErrAppAlreadyBound)
}

func (s *BindSuite) TestBindAppWithNoUnits(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	app := provisiontest.NewFakeApp("painkiller", "python", 0)
	err = instance.BindApp(app, true, nil)
	c.Assert(err, check.IsNil)
	expectedInstances := []bind.ServiceInstance{
		{
			Name: "my-mysql",
			Envs: map[string]string{
				"DATABASE_USER":     "root",
				"DATABASE_PASSWORD": "s3cr3t",
			},
		},
	}
	c.Assert(app.GetInstances("mysql"), check.DeepEquals, expectedInstances)
}

func (s *BindSuite) TestUnbindUnit(c *check.C) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	app := provisiontest.NewFakeApp("painkiller", "python", 1)
	units, err := app.GetUnits()
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{app.GetName()},
		Units:       []string{units[0].GetID()},
	}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	units, err = app.GetUnits()
	c.Assert(err, check.IsNil)
	err = instance.UnbindUnit(app, units[0])
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
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	app := provisiontest.NewFakeApp("painkiller", "python", 2)
	app.AddInstance(
		bind.InstanceApp{
			ServiceName:   "mysql",
			Instance:      bind.ServiceInstance{Name: "my-mysql"},
			ShouldRestart: true,
		}, ioutil.Discard)
	units, err := app.GetUnits()
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{app.GetName()},
		Units:       []string{units[0].GetID(), units[1].GetID()},
	}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	err = instance.UnbindApp(app, true, nil)
	c.Assert(err, check.IsNil)
	err = tsurutest.WaitCondition(1e9, func() bool {
		return atomic.LoadInt32(&calls) > 1
	})
	c.Assert(err, check.IsNil)
	c.Assert(app.GetInstances("mysql"), check.HasLen, 0)
}

func (s *BindSuite) TestUnbindRemovesAppFromServiceInstance(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
	}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	app := provisiontest.NewFakeApp("painkiller", "python", 0)
	app.AddInstance(
		bind.InstanceApp{
			ServiceName:   "mysql",
			Instance:      bind.ServiceInstance{Name: "my-mysql"},
			ShouldRestart: true,
		}, ioutil.Discard)
	err = instance.UnbindApp(app, true, nil)
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
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	app := provisiontest.NewFakeApp("painkiller", "python", 1)
	app.AddInstance(
		bind.InstanceApp{
			ServiceName:   "mysql",
			Instance:      bind.ServiceInstance{Name: "my-mysql"},
			ShouldRestart: true,
		}, ioutil.Discard)
	units, err := app.GetUnits()
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{app.GetName()},
		Units:       []string{units[0].GetID()},
	}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	err = instance.UnbindApp(app, true, nil)
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
	srvc := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"name": "my-mysql"})
	app := provisiontest.NewFakeApp("painkiller", "python", 0)
	err = instance.UnbindApp(app, true, nil)
	c.Assert(err, check.Equals, ErrAppNotBound)
}
