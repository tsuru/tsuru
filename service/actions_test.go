// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
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
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	instance := ServiceInstance{Name: "mysql"}
	ctx := action.FWContext{
		Params: []interface{}{srv, instance, "my@user", ""},
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
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
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
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	defer s.conn.Services().RemoveId(srv.Name)
	instance := ServiceInstance{Name: "mysql"}
	ctx := action.BWContext{Params: []interface{}{srv, instance, "", "test"}}
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
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
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

func (s *S) TestBindAppDBActionForward(c *check.C) {
	si := ServiceInstance{Name: "mysql"}
	err := s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	defer s.conn.Apps().Remove(bson.M{"name": a.GetName()})
	ctx := action.FWContext{
		Params: []interface{}{&bindPipelineArgs{app: a, serviceInstance: &si}},
	}
	_, err = bindAppDBAction.Forward(ctx)
	c.Assert(err, check.IsNil)
	err = s.conn.ServiceInstances().Find(bson.M{"name": si.Name}).One(&si)
	c.Assert(err, check.IsNil)
	c.Assert(si.Apps, check.HasLen, 1)
}

func (s *S) TestBindAppDBActionForwardInvalidParam(c *check.C) {
	si := ServiceInstance{Name: "mysql"}
	err := s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	ctx := action.FWContext{
		Params: []interface{}{"wrong parameter"},
	}
	_, err = bindAppDBAction.Forward(ctx)
	c.Assert(err, check.Not(check.IsNil))
	c.Assert(err, check.ErrorMatches, "^invalid arguments for pipeline, expected \\*bindPipelineArgs$")
}

func (s *S) TestBindAppDBActionForwardTwice(c *check.C) {
	si := ServiceInstance{Name: "mysql"}
	err := s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"name": si.Name})
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	defer s.conn.Apps().Remove(bson.M{"name": a.GetName()})
	ctx := action.FWContext{
		Params: []interface{}{&bindPipelineArgs{app: a, serviceInstance: &si}},
	}
	_, err = bindAppDBAction.Forward(ctx)
	c.Assert(err, check.IsNil)
	_, err = bindAppDBAction.Forward(ctx)
	c.Assert(err, check.Equals, ErrAppAlreadyBound)
}

func (s *S) TestBindAppDBActionBackwardRemovesAppFromServiceInstance(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	si := ServiceInstance{Name: "mysql", Apps: []string{a.GetName()}}
	err := s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	ctx := action.BWContext{
		Params: []interface{}{&bindPipelineArgs{app: a, serviceInstance: &si}},
	}
	bindAppDBAction.Backward(ctx)
	c.Assert(err, check.IsNil)
	err = s.conn.ServiceInstances().Find(bson.M{"name": si.Name}).One(&si)
	c.Assert(err, check.IsNil)
	c.Assert(si.Apps, check.HasLen, 0)
}

func (s *S) TestBindAppEndpointActionForwardReturnsEnvVars(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	service := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
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
		Params: []interface{}{&bindPipelineArgs{app: a, serviceInstance: &si}},
	}
	envs, err := bindAppEndpointAction.Forward(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(envs, check.DeepEquals, map[string]string{
		"DATABASE_USER":     "root",
		"DATABASE_PASSWORD": "s3cr3t",
	})
}

func (s *S) TestBindAppEndpointActionBackward(c *check.C) {
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	service := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
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
		Params:   []interface{}{&bindPipelineArgs{app: a, serviceInstance: &si}},
		FWResult: nil,
	}
	bindAppEndpointAction.Backward(bwCtx)
	c.Assert(called, check.Equals, true)
}

func (s *S) TestSetBoundEnvsActionName(c *check.C) {
	c.Assert(setBoundEnvsAction.Name, check.Equals, "set-bound-envs")
}

func (s *S) TestSetBoundEnvsActionForward(c *check.C) {
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql"}
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	ctx := action.FWContext{
		Params:   []interface{}{&bindPipelineArgs{app: a, serviceInstance: &si}},
		Previous: map[string]string{"DATABASE_NAME": "mydb", "DATABASE_USER": "root"},
	}
	result, err := setBoundEnvsAction.Forward(ctx)
	c.Assert(err, check.IsNil)
	args := bind.AddInstanceArgs{
		Envs: []bind.ServiceEnvVar{
			{EnvVar: bind.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, ServiceName: "mysql", InstanceName: "my-mysql"},
			{EnvVar: bind.EnvVar{Name: "DATABASE_USER", Value: "root"}, ServiceName: "mysql", InstanceName: "my-mysql"},
		},
	}
	c.Assert(result, check.DeepEquals, args)
	envs := a.GetServiceEnvs()
	c.Assert(envs, check.DeepEquals, args.Envs)
}

func (s *S) TestSetBoundEnvsActionForwardWrongParameter(c *check.C) {
	ctx := action.FWContext{Params: []interface{}{"something"}}
	_, err := setBoundEnvsAction.Forward(ctx)
	c.Assert(err.Error(), check.Equals, "invalid arguments for pipeline, expected *bindPipelineArgs")
}

func (s *S) TestSetBoundEnvsActionBackward(c *check.C) {
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql"}
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	err := a.AddInstance(bind.AddInstanceArgs{
		Envs: []bind.ServiceEnvVar{
			{EnvVar: bind.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, ServiceName: "mysql", InstanceName: "my-mysql"},
			{EnvVar: bind.EnvVar{Name: "DATABASE_USER", Value: "root"}, ServiceName: "mysql", InstanceName: "my-mysql"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	ctx := action.BWContext{
		Params:   []interface{}{&bindPipelineArgs{app: a, serviceInstance: &si}},
		FWResult: nil,
	}
	setBoundEnvsAction.Backward(ctx)
	instances := a.GetServiceEnvs()
	c.Assert(instances, check.HasLen, 0)
}

func (s *S) TestUnbindUnitsForward(c *check.C) {
	var reqs []*http.Request
	var reqLock sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqLock.Lock()
		defer reqLock.Unlock()
		reqs = append(reqs, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	srv := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "static", 10)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	for i := range units {
		err = si.BindUnit(a, &units[i])
		c.Assert(err, check.IsNil)
	}
	buf := bytes.NewBuffer(nil)
	args := bindPipelineArgs{
		app:             a,
		serviceInstance: &si,
		writer:          buf,
	}
	ctx := action.FWContext{Params: []interface{}{&args}}
	_, err = unbindUnits.Forward(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(reqs, check.HasLen, 20)
	for i, req := range reqs {
		if i < 10 {
			c.Assert(req.Method, check.Equals, "POST")
		} else {
			c.Assert(req.Method, check.Equals, "DELETE")
		}
	}
	siDB, err := GetServiceInstance(si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	c.Assert(siDB.BoundUnits, check.DeepEquals, []Unit{})
}

func (s *S) TestUnbindUnitsForwardPartialFailure(c *check.C) {
	var reqs []*http.Request
	var reqLock sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqLock.Lock()
		defer reqLock.Unlock()
		reqs = append(reqs, r)
		if len(reqs) > 14 && len(reqs) <= 20 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("my error"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	srv := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "static", 10)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	for i := range units {
		err = si.BindUnit(a, &units[i])
		c.Assert(err, check.IsNil)
	}
	buf := bytes.NewBuffer(nil)
	args := bindPipelineArgs{
		app:             a,
		serviceInstance: &si,
		writer:          buf,
	}
	ctx := action.FWContext{Params: []interface{}{&args}}
	_, err = unbindUnits.Forward(ctx)
	c.Assert(err.Error(), check.Equals, `Failed to unbind ("/resources/my-mysql/bind"): invalid response: my error (code: 500)`)
	c.Assert(reqs, check.HasLen, 24)
	for i, req := range reqs {
		if i < 10 {
			c.Assert(req.Method, check.Equals, "POST")
		} else if i < 20 {
			c.Assert(req.Method, check.Equals, "DELETE")
		} else {
			c.Assert(req.Method, check.Equals, "POST")
		}
	}
	siDB, err := GetServiceInstance(si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	var unitIDs []string
	for _, u := range siDB.BoundUnits {
		unitIDs = append(unitIDs, u.ID)
	}
	sort.Strings(unitIDs)
	c.Assert(unitIDs, check.DeepEquals, []string{
		"myapp-0",
		"myapp-1",
		"myapp-2",
		"myapp-3",
		"myapp-4",
		"myapp-5",
		"myapp-6",
		"myapp-7",
		"myapp-8",
		"myapp-9",
	})
}

func (s *S) TestUnbindUnitsBackward(c *check.C) {
	var reqs []*http.Request
	var reqLock sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqLock.Lock()
		defer reqLock.Unlock()
		reqs = append(reqs, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	srv := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "static", 4)
	buf := bytes.NewBuffer(nil)
	args := bindPipelineArgs{
		app:             a,
		serviceInstance: &si,
		writer:          buf,
	}
	ctx := action.BWContext{Params: []interface{}{&args}}
	unbindUnits.Backward(ctx)
	c.Assert(reqs, check.HasLen, 4)
	for _, req := range reqs {
		c.Assert(req.Method, check.Equals, "POST")
	}
	siDB, err := GetServiceInstance(si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	var unitIDs []string
	for _, u := range siDB.BoundUnits {
		unitIDs = append(unitIDs, u.ID)
	}
	sort.Strings(unitIDs)
	c.Assert(unitIDs, check.DeepEquals, []string{
		"myapp-0",
		"myapp-1",
		"myapp-2",
		"myapp-3",
	})
}

func (s *S) TestUnbindAppDBForward(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "static", 4)
	srv := Service{Name: "mysql"}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}, Apps: []string{a.GetName()}}
	err = s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(nil)
	args := bindPipelineArgs{
		app:             a,
		serviceInstance: &si,
		writer:          buf,
	}
	ctx := action.FWContext{Params: []interface{}{&args}}
	_, err = unbindAppDB.Forward(ctx)
	c.Assert(err, check.IsNil)
	siDB, err := GetServiceInstance(si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	c.Assert(siDB.Apps, check.DeepEquals, []string{})
}

func (s *S) TestUnbindAppDBBackward(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "static", 4)
	srv := Service{Name: "mysql"}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(nil)
	args := bindPipelineArgs{
		app:             a,
		serviceInstance: &si,
		writer:          buf,
	}
	ctx := action.BWContext{Params: []interface{}{&args}}
	unbindAppDB.Backward(ctx)
	siDB, err := GetServiceInstance(si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	c.Assert(siDB.Apps, check.DeepEquals, []string{a.GetName()})
}

func (s *S) TestUnbindAppEndpointForward(c *check.C) {
	var reqs []*http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	a := provisiontest.NewFakeApp("myapp", "static", 4)
	srv := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(nil)
	args := bindPipelineArgs{
		app:             a,
		serviceInstance: &si,
		writer:          buf,
	}
	ctx := action.FWContext{Params: []interface{}{&args}}
	_, err = unbindAppEndpoint.Forward(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(reqs, check.HasLen, 1)
	c.Assert(reqs[0].Method, check.Equals, "DELETE")
	c.Assert(reqs[0].URL.Path, check.Equals, "/resources/my-mysql/bind-app")
}

func (s *S) TestUnbindAppEndpointForwardNotFound(c *check.C) {
	var reqs []*http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, r)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	a := provisiontest.NewFakeApp("myapp", "static", 4)
	srv := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(nil)
	args := bindPipelineArgs{
		app:             a,
		serviceInstance: &si,
		writer:          buf,
	}
	ctx := action.FWContext{Params: []interface{}{&args}}
	_, err = unbindAppEndpoint.Forward(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(reqs, check.HasLen, 1)
	c.Assert(reqs[0].Method, check.Equals, "DELETE")
	c.Assert(reqs[0].URL.Path, check.Equals, "/resources/my-mysql/bind-app")
}

func (s *S) TestUnbindAppEndpointBackward(c *check.C) {
	var reqs []*http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	a := provisiontest.NewFakeApp("myapp", "static", 4)
	srv := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(nil)
	args := bindPipelineArgs{
		app:             a,
		serviceInstance: &si,
		writer:          buf,
	}
	ctx := action.BWContext{Params: []interface{}{&args}}
	unbindAppEndpoint.Backward(ctx)
	c.Assert(reqs, check.HasLen, 1)
	c.Assert(reqs[0].Method, check.Equals, "POST")
	c.Assert(reqs[0].URL.Path, check.Equals, "/resources/my-mysql/bind-app")
}

func (s *S) TestRemoveBoundEnvsForward(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "static", 4)
	srv := Service{Name: "mysql"}
	err := s.conn.Services().Insert(&srv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(&si)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(bind.AddInstanceArgs{
		Envs: []bind.ServiceEnvVar{
			{EnvVar: bind.EnvVar{Name: "ENV1", Value: "VAL1"}, ServiceName: "mysql", InstanceName: "my-mysql"},
			{EnvVar: bind.EnvVar{Name: "ENV2", Value: "VAL2"}, ServiceName: "mysql", InstanceName: "my-mysql"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(nil)
	args := bindPipelineArgs{
		app:             a,
		serviceInstance: &si,
		writer:          buf,
	}
	ctx := action.FWContext{Params: []interface{}{&args}}
	_, err = removeBoundEnvs.Forward(ctx)
	c.Assert(err, check.IsNil)
	envs := a.GetServiceEnvs()
	c.Assert(envs, check.DeepEquals, []bind.ServiceEnvVar{})
}
