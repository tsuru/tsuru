// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/testing"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
)

func (s *S) TestCreateServiceInstancMinParams(c *gocheck.C) {
	c.Assert(createServiceInstance.MinParams, gocheck.Equals, 2)
}

func (s *S) TestCreateServiceInstancName(c *gocheck.C) {
	c.Assert(createServiceInstance.Name, gocheck.Equals, "create-service-instance")
}

func (s *S) TestCreateServiceInstanceForward(c *gocheck.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	instance := ServiceInstance{Name: "mysql"}
	ctx := action.FWContext{
		Params: []interface{}{srv, instance},
	}
	r, err := createServiceInstance.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	a, ok := r.(ServiceInstance)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(a.Name, gocheck.Equals, instance.Name)
	c.Assert(atomic.LoadInt32(&requests), gocheck.Equals, int32(1))
}

func (s *S) TestCreateServiceInstanceForwardInvalidParams(c *gocheck.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	ctx := action.FWContext{Params: []interface{}{"", ""}}
	_, err = createServiceInstance.Forward(ctx)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "First parameter must be a Service.")
	ctx = action.FWContext{Params: []interface{}{srv, ""}}
	_, err = createServiceInstance.Forward(ctx)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Second parameter must be a ServiceInstance.")
}

func (s *S) TestCreateServiceInstanceBackward(c *gocheck.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	instance := ServiceInstance{Name: "mysql"}
	ctx := action.BWContext{Params: []interface{}{srv, instance}}
	createServiceInstance.Backward(ctx)
	c.Assert(atomic.LoadInt32(&requests), gocheck.Equals, int32(1))
}

func (s *S) TestCreateServiceInstanceBackwardParams(c *gocheck.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	ctx := action.BWContext{Params: []interface{}{srv, ""}}
	createServiceInstance.Backward(ctx)
	c.Assert(atomic.LoadInt32(&requests), gocheck.Equals, int32(0))
	ctx = action.BWContext{Params: []interface{}{"", ""}}
	createServiceInstance.Backward(ctx)
	c.Assert(atomic.LoadInt32(&requests), gocheck.Equals, int32(0))
}

func (s *S) TestInsertServiceInstancName(c *gocheck.C) {
	c.Assert(insertServiceInstance.Name, gocheck.Equals, "insert-service-instance")
}

func (s *S) TestInsertServiceInstancMinParams(c *gocheck.C) {
	c.Assert(insertServiceInstance.MinParams, gocheck.Equals, 2)
}

func (s *S) TestInsertServiceInstanceForward(c *gocheck.C) {
	srv := Service{Name: "mongodb"}
	instance := ServiceInstance{Name: "mysql"}
	ctx := action.FWContext{
		Params: []interface{}{srv, instance},
	}
	_, err := insertServiceInstance.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": instance.Name})
	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestInsertServiceInstanceForwardParams(c *gocheck.C) {
	ctx := action.FWContext{Params: []interface{}{"", ""}}
	_, err := insertServiceInstance.Forward(ctx)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Second parameter must be a ServiceInstance.")
}

func (s *S) TestInsertServiceInstanceBackward(c *gocheck.C) {
	srv := Service{Name: "mongodb"}
	instance := ServiceInstance{Name: "mysql"}
	err := s.conn.ServiceInstances().Insert(&instance)
	c.Assert(err, gocheck.IsNil)
	ctx := action.BWContext{
		Params: []interface{}{srv, instance},
	}
	insertServiceInstance.Backward(ctx)
	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestInsertServiceInstanceBackwardParams(c *gocheck.C) {
	instance := ServiceInstance{Name: "mysql"}
	err := s.conn.ServiceInstances().Insert(&instance)
	c.Assert(err, gocheck.IsNil)
	ctx := action.BWContext{
		Params: []interface{}{"", ""},
	}
	insertServiceInstance.Backward(ctx)
	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": instance.Name})
}

func (s *S) TestAddAppToServiceInstanceForward(c *gocheck.C) {
	si := ServiceInstance{Name: "mysql"}
	err := s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	a := testing.NewFakeApp("myapp", "static", 1)
	defer s.conn.Apps().Remove(bson.M{"name": a.GetName()})
	ctx := action.FWContext{
		Params: []interface{}{a, si},
	}
	_, err = addAppToServiceInstance.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	err = s.conn.ServiceInstances().Find(bson.M{"name": si.Name}).One(&si)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(si.Apps), gocheck.Equals, 1)
}

func (s *S) TestAddAppToServiceInstanceForwardInvalidServiceInstance(c *gocheck.C) {
	a := testing.NewFakeApp("myapp", "static", 1)
	defer s.conn.Apps().Remove(bson.M{"name": a.GetName()})
	ctx := action.FWContext{
		Params: []interface{}{a, "wrong parameter"},
	}
	_, err := addAppToServiceInstance.Forward(ctx)
	c.Assert(err, gocheck.Not(gocheck.IsNil))
	c.Assert(err, gocheck.ErrorMatches, "^Second parameter must be a ServiceInstance.$")
}

func (s *S) TestAddAppToServiceInstanceForwardInvalidApp(c *gocheck.C) {
	si := ServiceInstance{Name: "mysql"}
	err := s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	ctx := action.FWContext{
		Params: []interface{}{"wrong parameter", si},
	}
	_, err = addAppToServiceInstance.Forward(ctx)
	c.Assert(err, gocheck.Not(gocheck.IsNil))
	c.Assert(err, gocheck.ErrorMatches, "^First parameter must be a bind.App.$")
}

func (s *S) TestAddAppToServiceInstanceForwardTwice(c *gocheck.C) {
	si := ServiceInstance{Name: "mysql"}
	err := s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	a := testing.NewFakeApp("myapp", "static", 1)
	defer s.conn.Apps().Remove(bson.M{"name": a.GetName()})
	ctx := action.FWContext{
		Params: []interface{}{a, si},
	}
	_, err = addAppToServiceInstance.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	_, err = addAppToServiceInstance.Forward(ctx)
	c.Assert(err, gocheck.Not(gocheck.IsNil))
	c.Assert(err, gocheck.ErrorMatches, "^This app is already bound to this service instance.$")
}

func (s *S) TestAddAppToServiceInstanceBackwardRemovesAppFromServiceInstance(c *gocheck.C) {
	si := ServiceInstance{Name: "mysql"}
	err := s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	a := testing.NewFakeApp("myapp", "static", 1)
	defer s.conn.Apps().Remove(bson.M{"name": a.GetName()})
	err = si.AddApp(a.GetName())
	c.Assert(err, gocheck.IsNil)
	err = si.update()
	c.Assert(err, gocheck.IsNil)
	ctx := action.BWContext{
		Params: []interface{}{a, si},
	}
	addAppToServiceInstance.Backward(ctx)
	c.Assert(err, gocheck.IsNil)
	err = s.conn.ServiceInstances().Find(bson.M{"name": si.Name}).One(&si)
	c.Assert(err, gocheck.IsNil)
	c.Assert(len(si.Apps), gocheck.Equals, 0)
}

func (s *S) TestSetEnvironVariablesToAppForward(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	service := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := service.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(service.Name)
	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = si.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().RemoveId(si.Name)
	a := testing.NewFakeApp("myapp", "static", 1)
	ctx := action.FWContext{
		Params: []interface{}{a, si},
	}
	_, err = setEnvironVariablesToApp.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_USER": {
			Name:         "DATABASE_USER",
			Value:        "root",
			Public:       false,
			InstanceName: si.Name,
		},
		"DATABASE_PASSWORD": {
			Name:         "DATABASE_PASSWORD",
			Value:        "s3cr3t",
			Public:       false,
			InstanceName: si.Name,
		},
	}
	c.Assert(a.Envs(), gocheck.DeepEquals, expected)
}

func (s *S) TestSetEnvironVariablesToAppForwardReturnsEnvVars(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	service := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := service.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(service.Name)
	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = si.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().RemoveId(si.Name)
	a := testing.NewFakeApp("myapp", "static", 1)
	ctx := action.FWContext{
		Params: []interface{}{a, si},
	}
	result, err := setEnvironVariablesToApp.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	expected := []bind.EnvVar{
		{Name: "DATABASE_USER",
			Value:        "root",
			Public:       false,
			InstanceName: si.Name,
		},
		{Name: "DATABASE_PASSWORD",
			Value:        "s3cr3t",
			Public:       false,
			InstanceName: si.Name,
		},
	}
	got := result.([]bind.EnvVar)
	if got[0].Name == "DATABASE_PASSWORD" {
		got[0], got[1] = got[1], got[0]
	}
	c.Assert(got, gocheck.DeepEquals, expected)
}

func (s *S) TestSetEnvironVariablesToAppBackward(c *gocheck.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	service := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := service.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().RemoveId(service.Name)
	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = si.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().RemoveId(si.Name)
	a := testing.NewFakeApp("myapp", "static", 1)
	ctx := action.FWContext{
		Params: []interface{}{a, si},
	}
	r, err := setEnvironVariablesToApp.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	bwCtx := action.BWContext{
		Params:   []interface{}{a},
		FWResult: r,
	}
	setEnvironVariablesToApp.Backward(bwCtx)
	c.Assert(a.Envs(), gocheck.DeepEquals, map[string]bind.EnvVar{})
}
