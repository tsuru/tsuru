// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestCreateServiceInstancMinParams(c *check.C) {
	c.Assert(createServiceInstance.MinParams, check.Equals, 2)
}

func (s *S) TestCreateServiceInstancName(c *check.C) {
	c.Assert(createServiceInstance.Name, check.Equals, "create-service-instance")
}

func (s *S) TestCreateServiceInstanceForward(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	instance := ServiceInstance{Name: "mysql"}
	ctx := action.FWContext{
		Params: []interface{}{srv, instance, "my@user"},
	}
	r, err := createServiceInstance.Forward(ctx)
	c.Assert(err, check.IsNil)
	a, ok := r.(ServiceInstance)
	c.Assert(ok, check.Equals, true)
	c.Assert(a.Name, check.Equals, instance.Name)
	c.Assert(atomic.LoadInt32(&requests), check.Equals, int32(1))
}

func (s *S) TestCreateServiceInstanceForwardInvalidParams(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	ctx := action.FWContext{Params: []interface{}{"", "", ""}}
	_, err = createServiceInstance.Forward(ctx)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "First parameter must be a Service.")
	ctx = action.FWContext{Params: []interface{}{srv, "", ""}}
	_, err = createServiceInstance.Forward(ctx)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Second parameter must be a ServiceInstance.")
	instance := ServiceInstance{Name: "mysql"}
	ctx = action.FWContext{Params: []interface{}{srv, instance, 1}}
	_, err = createServiceInstance.Forward(ctx)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Third parameter must be a string.")
}

func (s *S) TestCreateServiceInstanceBackward(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	instance := ServiceInstance{Name: "mysql"}
	ctx := action.BWContext{Params: []interface{}{srv, instance}}
	createServiceInstance.Backward(ctx)
	c.Assert(atomic.LoadInt32(&requests), check.Equals, int32(1))
}

func (s *S) TestCreateServiceInstanceBackwardParams(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	ctx := action.BWContext{Params: []interface{}{srv, ""}}
	createServiceInstance.Backward(ctx)
	c.Assert(atomic.LoadInt32(&requests), check.Equals, int32(0))
	ctx = action.BWContext{Params: []interface{}{"", ""}}
	createServiceInstance.Backward(ctx)
	c.Assert(atomic.LoadInt32(&requests), check.Equals, int32(0))
}

func (s *S) TestInsertServiceInstancName(c *check.C) {
	c.Assert(insertServiceInstance.Name, check.Equals, "insert-service-instance")
}

func (s *S) TestInsertServiceInstancMinParams(c *check.C) {
	c.Assert(insertServiceInstance.MinParams, check.Equals, 2)
}

func (s *S) TestInsertServiceInstanceForward(c *check.C) {
	srv := Service{Name: "mongodb"}
	instance := ServiceInstance{Name: "mysql"}
	ctx := action.FWContext{
		Params: []interface{}{srv, instance},
	}
	_, err := insertServiceInstance.Forward(ctx)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": instance.Name})
	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, check.IsNil)
}

func (s *S) TestInsertServiceInstanceForwardParams(c *check.C) {
	ctx := action.FWContext{Params: []interface{}{"", ""}}
	_, err := insertServiceInstance.Forward(ctx)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Second parameter must be a ServiceInstance.")
}

func (s *S) TestInsertServiceInstanceBackward(c *check.C) {
	srv := Service{Name: "mongodb"}
	instance := ServiceInstance{Name: "mysql"}
	err := s.conn.ServiceInstances().Insert(&instance)
	c.Assert(err, check.IsNil)
	ctx := action.BWContext{
		Params: []interface{}{srv, instance},
	}
	insertServiceInstance.Backward(ctx)
	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, check.NotNil)
}

func (s *S) TestInsertServiceInstanceBackwardParams(c *check.C) {
	instance := ServiceInstance{Name: "mysql"}
	err := s.conn.ServiceInstances().Insert(&instance)
	c.Assert(err, check.IsNil)
	ctx := action.BWContext{
		Params: []interface{}{"", ""},
	}
	insertServiceInstance.Backward(ctx)
	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": instance.Name})
}

func (s *S) TestAddAppToServiceInstanceForward(c *check.C) {
	si := ServiceInstance{Name: "mysql"}
	err := s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	defer s.conn.Apps().Remove(bson.M{"name": a.GetName()})
	ctx := action.FWContext{
		Params: []interface{}{a, si},
	}
	_, err = addAppToServiceInstance.Forward(ctx)
	c.Assert(err, check.IsNil)
	err = s.conn.ServiceInstances().Find(bson.M{"name": si.Name}).One(&si)
	c.Assert(err, check.IsNil)
	c.Assert(len(si.Apps), check.Equals, 1)
}

func (s *S) TestAddAppToServiceInstanceForwardInvalidServiceInstance(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	defer s.conn.Apps().Remove(bson.M{"name": a.GetName()})
	ctx := action.FWContext{
		Params: []interface{}{a, "wrong parameter"},
	}
	_, err := addAppToServiceInstance.Forward(ctx)
	c.Assert(err, check.Not(check.IsNil))
	c.Assert(err, check.ErrorMatches, "^Second parameter must be a ServiceInstance.$")
}

func (s *S) TestAddAppToServiceInstanceForwardInvalidApp(c *check.C) {
	si := ServiceInstance{Name: "mysql"}
	err := s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	ctx := action.FWContext{
		Params: []interface{}{"wrong parameter", si},
	}
	_, err = addAppToServiceInstance.Forward(ctx)
	c.Assert(err, check.Not(check.IsNil))
	c.Assert(err, check.ErrorMatches, "^First parameter must be a bind.App.$")
}

func (s *S) TestAddAppToServiceInstanceForwardTwice(c *check.C) {
	si := ServiceInstance{Name: "mysql"}
	err := s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	defer s.conn.Apps().Remove(bson.M{"name": a.GetName()})
	ctx := action.FWContext{
		Params: []interface{}{a, si},
	}
	_, err = addAppToServiceInstance.Forward(ctx)
	c.Assert(err, check.IsNil)
	_, err = addAppToServiceInstance.Forward(ctx)
	c.Assert(err, check.Not(check.IsNil))
	c.Assert(err, check.ErrorMatches, "^This app is already bound to this service instance.$")
}

func (s *S) TestAddAppToServiceInstanceBackwardRemovesAppFromServiceInstance(c *check.C) {
	si := ServiceInstance{Name: "mysql"}
	err := s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	defer s.conn.Apps().Remove(bson.M{"name": a.GetName()})
	err = si.AddApp(a.GetName())
	c.Assert(err, check.IsNil)
	err = si.update()
	c.Assert(err, check.IsNil)
	ctx := action.BWContext{
		Params: []interface{}{a, si},
	}
	addAppToServiceInstance.Backward(ctx)
	c.Assert(err, check.IsNil)
	err = s.conn.ServiceInstances().Find(bson.M{"name": si.Name}).One(&si)
	c.Assert(err, check.IsNil)
	c.Assert(len(si.Apps), check.Equals, 0)
}

func (s *S) TestSetBindAppActionForwardReturnsEnvVars(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	service := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := service.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(service.Name)
	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = si.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().RemoveId(si.Name)
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	ctx := action.FWContext{
		Params: []interface{}{a, si},
	}
	envs, err := setBindAppAction.Forward(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(envs, check.DeepEquals, map[string]string{
		"DATABASE_USER":     "root",
		"DATABASE_PASSWORD": "s3cr3t",
	})
}

func (s *S) TestSetBindAppActionBackward(c *check.C) {
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	service := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := service.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(service.Name)
	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = si.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().RemoveId(si.Name)
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	bwCtx := action.BWContext{
		Params:   []interface{}{a, si},
		FWResult: nil,
	}
	setBindAppAction.Backward(bwCtx)
	c.Assert(called, check.Equals, true)
}

func (s *S) TestSetTsuruServicesName(c *check.C) {
	c.Assert(setTsuruServices.Name, check.Equals, "set-TSURU_SERVICES-env-var")
}

func (s *S) TestSetTsuruServicesForward(c *check.C) {
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql"}
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	ctx := action.FWContext{
		Params:   []interface{}{a, si},
		Previous: map[string]string{"DATABASE_NAME": "mydb", "DATABASE_USER": "root"},
	}
	result, err := setTsuruServices.Forward(ctx)
	c.Assert(err, check.IsNil)
	instance := bind.ServiceInstance{
		Name: "my-mysql",
		Envs: map[string]string{"DATABASE_NAME": "mydb", "DATABASE_USER": "root"},
	}
	c.Assert(result, check.DeepEquals, instance)
	instances := a.GetInstances("mysql")
	c.Assert(instances, check.DeepEquals, []bind.ServiceInstance{instance})
}

func (s *S) TestSetTsuruServicesForwardWrongFirstParameter(c *check.C) {
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql"}
	ctx := action.FWContext{Params: []interface{}{"something", si}}
	_, err := setTsuruServices.Forward(ctx)
	c.Assert(err.Error(), check.Equals, "First parameter must be a bind.App.")
}

func (s *S) TestSetTsuruServicesForwardWrongSecondParameter(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "python", 1)
	ctx := action.FWContext{Params: []interface{}{a, "something"}}
	_, err := setTsuruServices.Forward(ctx)
	c.Assert(err.Error(), check.Equals, "Second parameter must be a ServiceInstance.")
}

func (s *S) TestSetTsuruServicesBackward(c *check.C) {
	instance := bind.ServiceInstance{
		Name: "my-mysql",
		Envs: map[string]string{"DATABASE_NAME": "mydb", "DATABASE_USER": "root"},
	}
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql"}
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	err := a.AddInstance("mysql", instance, nil)
	c.Assert(err, check.IsNil)
	ctx := action.BWContext{
		Params:   []interface{}{a, si},
		FWResult: instance,
	}
	setTsuruServices.Backward(ctx)
	instances := a.GetInstances("mysql")
	c.Assert(instances, check.HasLen, 0)
}
