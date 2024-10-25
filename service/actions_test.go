// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/provision/provisiontest"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	jobTypes "github.com/tsuru/tsuru/types/job"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	check "gopkg.in/check.v1"
)

func (s *S) TestNotifyCreateServiceInstanceMinParams(c *check.C) {
	c.Assert(notifyCreateServiceInstance.MinParams, check.Equals, 3)
}

func (s *S) TestNotifyCreateServiceInstanceName(c *check.C) {
	c.Assert(notifyCreateServiceInstance.Name, check.Equals, "notify-create-service-instance")
}

func (s *S) TestNotifyCreateServiceInstanceForward(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{Name: "mysql"}
	evt := createEvt(c)
	ctx := action.FWContext{
		Params: []interface{}{srv, &instance, evt, ""},
	}
	r, err := notifyCreateServiceInstance.Forward(ctx)
	c.Assert(err, check.IsNil)
	a, ok := r.(*ServiceInstance)
	c.Assert(ok, check.Equals, true)
	c.Assert(a.Name, check.Equals, instance.Name)
	c.Assert(atomic.LoadInt32(&requests), check.Equals, int32(1))
}

func (s *S) TestNotifyCreateServiceInstanceForwardInvalidParams(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	ctx := action.FWContext{Params: []interface{}{"", "", ""}}
	_, err = notifyCreateServiceInstance.Forward(ctx)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "First parameter must be a Service.")
	ctx = action.FWContext{Params: []interface{}{srv, "", ""}}
	_, err = notifyCreateServiceInstance.Forward(ctx)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Second parameter must be a *ServiceInstance.")
	instance := ServiceInstance{Name: "mysql"}
	ctx = action.FWContext{Params: []interface{}{srv, &instance, 1}}
	_, err = notifyCreateServiceInstance.Forward(ctx)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Third parameter must be an event.")
}

func (s *S) TestNotifyCreateServiceInstanceBackward(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{Name: "mysql"}
	evt := createEvt(c)
	ctx := action.BWContext{Params: []interface{}{srv, &instance, evt, "test"}}
	notifyCreateServiceInstance.Backward(ctx)
	c.Assert(atomic.LoadInt32(&requests), check.Equals, int32(1))
}

func (s *S) TestNotifyCreateServiceInstanceBackwardParams(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	ctx := action.BWContext{Params: []interface{}{srv, ""}}
	notifyCreateServiceInstance.Backward(ctx)
	c.Assert(atomic.LoadInt32(&requests), check.Equals, int32(0))
	ctx = action.BWContext{Params: []interface{}{"", ""}}
	notifyCreateServiceInstance.Backward(ctx)
	c.Assert(atomic.LoadInt32(&requests), check.Equals, int32(0))
}

func (s *S) TestCreateServiceInstanceName(c *check.C) {
	c.Assert(createServiceInstance.Name, check.Equals, "create-service-instance")
}

func (s *S) TestCreateServiceInstanceMinParams(c *check.C) {
	c.Assert(createServiceInstance.MinParams, check.Equals, 2)
}

func (s *S) TestCreateServiceInstanceForward(c *check.C) {
	srv := Service{Name: "mongodb"}
	instance := ServiceInstance{Name: "mysql"}
	ctx := action.FWContext{
		Params: []interface{}{srv, &instance},
	}
	_, err := createServiceInstance.Forward(ctx)
	c.Assert(err, check.IsNil)

	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)

	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": instance.Name}).Decode(&instance)
	c.Assert(err, check.IsNil)
}

func (s *S) TestCreateServiceInstanceForwardParams(c *check.C) {
	ctx := action.FWContext{Params: []interface{}{"", ""}}
	_, err := createServiceInstance.Forward(ctx)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Second parameter must be a *ServiceInstance.")
}

func (s *S) TestCreateServiceInstanceBackward(c *check.C) {
	srv := Service{Name: "mongodb"}
	instance := ServiceInstance{Name: "mysql"}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &instance)
	c.Assert(err, check.IsNil)
	ctx := action.BWContext{
		Params: []interface{}{srv, &instance},
	}
	createServiceInstance.Backward(ctx)
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": instance.Name}).Decode(&instance)
	c.Assert(err, check.NotNil)
}

func (s *S) TestCreateServiceInstanceBackwardParams(c *check.C) {
	instance := ServiceInstance{Name: "mysql"}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &instance)
	c.Assert(err, check.IsNil)
	ctx := action.BWContext{
		Params: []interface{}{"", ""},
	}
	createServiceInstance.Backward(ctx)
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": instance.Name}).Decode(&instance)
	c.Assert(err, check.IsNil)
}

func (s *S) TestUpdateServiceInstanceName(c *check.C) {
	c.Assert(updateServiceInstance.Name, check.Equals, "update-service-instance")
}

func (s *S) TestUpdateServiceInstanceMinParams(c *check.C) {
	c.Assert(updateServiceInstance.MinParams, check.Equals, 3)
}

func (s *S) TestUpdateServiceInstanceForward(c *check.C) {
	srv := Service{Name: "mongodb"}
	instance := ServiceInstance{Name: "dbname"}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	updatedInstance := ServiceInstance{Description: "new description", Tags: []string{"tag-a", "tag-b"}, TeamOwner: "new-owner"}
	ctx := action.FWContext{
		Params: []interface{}{srv, instance, updatedInstance},
	}
	_, err = updateServiceInstance.Forward(ctx)
	c.Assert(err, check.IsNil)
	var si ServiceInstance
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": instance.Name}).Decode(&si)
	c.Assert(err, check.IsNil)
	c.Assert(si.Description, check.Equals, updatedInstance.Description)
	c.Assert(si.Tags, check.DeepEquals, updatedInstance.Tags)
	c.Assert(si.TeamOwner, check.DeepEquals, updatedInstance.TeamOwner)
	c.Assert(si.Teams, check.DeepEquals, []string{updatedInstance.TeamOwner})
}

func (s *S) TestUpdateServiceInstanceForwardParams(c *check.C) {
	ctx := action.FWContext{Params: []interface{}{"", ""}}
	_, err := updateServiceInstance.Forward(ctx)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Second parameter must be a ServiceInstance.")
	ctx = action.FWContext{Params: []interface{}{"", ServiceInstance{}, ""}}
	_, err = updateServiceInstance.Forward(ctx)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Third parameter must be a ServiceInstance.")
}

func (s *S) TestUpdateServiceInstanceBackward(c *check.C) {
	srv := Service{Name: "mongodb"}
	instance := ServiceInstance{Name: "dbname", ServiceName: srv.Name, Description: "old description"}
	updatedInstance := ServiceInstance{Name: instance.Name, ServiceName: instance.ServiceName, Description: "new description"}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &updatedInstance)
	c.Assert(err, check.IsNil)
	ctx := action.BWContext{
		Params: []interface{}{srv, instance, updatedInstance},
	}
	updateServiceInstance.Backward(ctx)
	var si ServiceInstance
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": instance.Name}).Decode(&si)
	c.Assert(err, check.IsNil)
	c.Assert(si.Description, check.Equals, instance.Description)
}

func (s *S) TestUpdateServiceInstanceBackwardParams(c *check.C) {
	srv := Service{Name: "mongodb"}
	instance := ServiceInstance{Name: "dbname", ServiceName: srv.Name, Description: "old description"}
	updatedInstance := ServiceInstance{Name: instance.Name, ServiceName: instance.ServiceName, Description: "new description"}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &updatedInstance)
	c.Assert(err, check.IsNil)
	ctx := action.BWContext{
		Params: []interface{}{"", "", ""},
	}
	updateServiceInstance.Backward(ctx)
	var si ServiceInstance
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": instance.Name}).Decode(&si)
	c.Assert(err, check.IsNil)
	c.Assert(si.Description, check.Equals, updatedInstance.Description)
}

func (s *S) TestNotifyUpdateServiceInstanceName(c *check.C) {
	c.Assert(notifyUpdateServiceInstance.Name, check.Equals, "notify-update-service-instance")
}

func (s *S) TestNotifyUpdateServiceInstanceMinParams(c *check.C) {
	c.Assert(notifyUpdateServiceInstance.MinParams, check.Equals, 4)
}

func (s *S) TestNotifyUpdateServiceInstanceForward(c *check.C) {
	var requests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		atomic.AddInt32(&requests, 1)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	instance := ServiceInstance{Name: "mysql"}
	evt := createEvt(c)
	ctx := action.FWContext{
		Params: []interface{}{srv, &instance, evt, ""},
	}
	r, err := notifyCreateServiceInstance.Forward(ctx)
	c.Assert(err, check.IsNil)
	a, ok := r.(*ServiceInstance)
	c.Assert(ok, check.Equals, true)
	c.Assert(a.Name, check.Equals, instance.Name)
	c.Assert(atomic.LoadInt32(&requests), check.Equals, int32(1))
}

func (s *S) TestNotifyUpdateServiceInstanceForwardParams(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srv := Service{Name: "mongodb", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	ctx := action.FWContext{Params: []interface{}{""}}
	_, err = notifyUpdateServiceInstance.Forward(ctx)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "First parameter must be a Service.")
	ctx = action.FWContext{Params: []interface{}{srv, "", ""}}
	_, err = notifyUpdateServiceInstance.Forward(ctx)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Second parameter must be a ServiceInstance.")
	ctx = action.FWContext{Params: []interface{}{srv, ServiceInstance{}, "", nil}}
	_, err = notifyUpdateServiceInstance.Forward(ctx)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Third parameter must be an event.")
	evt := createEvt(c)
	ctx = action.FWContext{Params: []interface{}{srv, ServiceInstance{}, "", evt, nil}}
	_, err = notifyUpdateServiceInstance.Forward(ctx)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "RequestID should be a string.")
}

func (s *S) TestBindAppDBActionForward(c *check.C) {
	si := ServiceInstance{Name: "mysql"}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	evt := createEvt(c)
	ctx := action.FWContext{
		Params: []interface{}{&bindAppPipelineArgs{app: a, serviceInstance: &si, event: evt}},
	}
	_, err = bindAppDBAction.Forward(ctx)
	c.Assert(err, check.IsNil)
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": si.Name}).Decode(&si)
	c.Assert(err, check.IsNil)
	c.Assert(si.Apps, check.HasLen, 1)
}

func (s *S) TestBindAppDBActionForwardInvalidParam(c *check.C) {
	si := ServiceInstance{Name: "mysql"}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)
	ctx := action.FWContext{
		Params: []interface{}{"wrong parameter"},
	}
	_, err = bindAppDBAction.Forward(ctx)
	c.Assert(err, check.Not(check.IsNil))
	c.Assert(err, check.ErrorMatches, "^invalid arguments for pipeline, expected \\*bindAppPipelineArgs.$")
}

func (s *S) TestBindAppDBActionForwardTwice(c *check.C) {
	si := ServiceInstance{Name: "mysql"}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	evt := createEvt(c)
	ctx := action.FWContext{
		Params: []interface{}{&bindAppPipelineArgs{app: a, serviceInstance: &si, event: evt}},
	}
	_, err = bindAppDBAction.Forward(ctx)
	c.Assert(err, check.IsNil)
	_, err = bindAppDBAction.Forward(ctx)
	c.Assert(err, check.Equals, ErrAppAlreadyBound)
}

func (s *S) TestBindAppDBActionBackwardRemovesAppFromServiceInstance(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	si := ServiceInstance{Name: "mysql", Apps: []string{a.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)
	evt := createEvt(c)
	ctx := action.BWContext{
		Params: []interface{}{&bindAppPipelineArgs{app: a, serviceInstance: &si, event: evt}},
	}
	bindAppDBAction.Backward(ctx)
	c.Assert(err, check.IsNil)
	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": si.Name}).Decode(&si)
	c.Assert(err, check.IsNil)
	c.Assert(si.Apps, check.HasLen, 0)
}

func (s *S) TestBindAppEndpointActionForwardReturnsEnvVars(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	service := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err := Create(context.TODO(), service)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), si)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	evt := createEvt(c)
	ctx := action.FWContext{
		Params: []interface{}{&bindAppPipelineArgs{app: a, serviceInstance: &si, event: evt}},
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
	service := Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "s3cr3t", OwnerTeams: []string{s.team.Name}}
	err := Create(context.TODO(), service)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), si)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	evt := createEvt(c)
	bwCtx := action.BWContext{
		Params:   []interface{}{&bindAppPipelineArgs{app: a, serviceInstance: &si, event: evt}},
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
	evt := createEvt(c)
	ctx := action.FWContext{
		Params:   []interface{}{&bindAppPipelineArgs{app: a, serviceInstance: &si, event: evt}},
		Previous: map[string]string{"DATABASE_NAME": "mydb", "DATABASE_USER": "root"},
	}
	result, err := setBoundEnvsAction.Forward(ctx)
	c.Assert(err, check.IsNil)
	args := bindTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, ServiceName: "mysql", InstanceName: "my-mysql"},
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root"}, ServiceName: "mysql", InstanceName: "my-mysql"},
		},
	}
	c.Assert(result, check.DeepEquals, args)
	envs := a.GetServiceEnvs()
	c.Assert(envs, check.DeepEquals, args.Envs)
}

func (s *S) TestSetBoundEnvsActionForwardWrongParameter(c *check.C) {
	ctx := action.FWContext{Params: []interface{}{"something"}}
	_, err := setBoundEnvsAction.Forward(ctx)
	c.Assert(err.Error(), check.Equals, "invalid arguments for pipeline, expected *bindAppPipelineArgs.")
}

func (s *S) TestSetBoundEnvsActionBackward(c *check.C) {
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql"}
	a := provisiontest.NewFakeApp("myapp", "static", 1)
	err := a.AddInstance(context.TODO(), bindTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, ServiceName: "mysql", InstanceName: "my-mysql"},
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root"}, ServiceName: "mysql", InstanceName: "my-mysql"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	evt := createEvt(c)
	ctx := action.BWContext{
		Params:   []interface{}{&bindAppPipelineArgs{app: a, serviceInstance: &si, event: evt}},
		FWResult: nil,
	}
	setBoundEnvsAction.Backward(ctx)
	instances := a.GetServiceEnvs()
	c.Assert(instances, check.HasLen, 0)
}

func (s *S) TestUnbindAppDBForward(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "static", 4)
	srv := Service{Name: "mysql"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}, Apps: []string{a.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(nil)
	evt := createEvt(c)
	args := bindAppPipelineArgs{
		event:           evt,
		app:             a,
		serviceInstance: &si,
		writer:          buf,
	}
	ctx := action.FWContext{Params: []interface{}{&args}}
	_, err = unbindAppDB.Forward(ctx)
	c.Assert(err, check.IsNil)
	siDB, err := GetServiceInstance(context.TODO(), si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	c.Assert(siDB.Apps, check.DeepEquals, []string{})
}

func (s *S) TestUnbindAppDBBackward(c *check.C) {
	a := provisiontest.NewFakeApp("myapp", "static", 4)
	srv := Service{Name: "mysql"}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(nil)
	evt := createEvt(c)
	args := bindAppPipelineArgs{
		event:           evt,
		app:             a,
		serviceInstance: &si,
		writer:          buf,
	}
	ctx := action.BWContext{Params: []interface{}{&args}}
	unbindAppDB.Backward(ctx)
	siDB, err := GetServiceInstance(context.TODO(), si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	c.Assert(siDB.Apps, check.DeepEquals, []string{a.Name})
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
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(nil)
	evt := createEvt(c)
	args := bindAppPipelineArgs{
		event:           evt,
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
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(nil)
	evt := createEvt(c)
	args := bindAppPipelineArgs{
		event:           evt,
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
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(nil)
	evt := createEvt(c)
	args := bindAppPipelineArgs{
		event:           evt,
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
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)
	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)
	err = a.AddInstance(context.TODO(), bindTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "ENV1", Value: "VAL1"}, ServiceName: "mysql", InstanceName: "my-mysql"},
			{EnvVar: bindTypes.EnvVar{Name: "ENV2", Value: "VAL2"}, ServiceName: "mysql", InstanceName: "my-mysql"},
		},
		ShouldRestart: true,
	})
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(nil)
	evt := createEvt(c)
	args := bindAppPipelineArgs{
		event:           evt,
		app:             a,
		serviceInstance: &si,
		writer:          buf,
	}
	ctx := action.FWContext{Params: []interface{}{&args}}
	_, err = removeBoundEnvs.Forward(ctx)
	c.Assert(err, check.IsNil)
	envs := a.GetServiceEnvs()
	c.Assert(envs, check.DeepEquals, []bindTypes.ServiceEnvVar{})
}

func (s *S) TestBindJobDBActionForwardInvalidParam(c *check.C) {
	si := ServiceInstance{
		Name: "mysql",
	}

	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)

	ctx := action.FWContext{
		Params: []interface{}{"wrong parameter"},
	}
	_, err = bindJobDBAction.Forward(ctx)
	c.Assert(err, check.Not(check.IsNil))
	c.Assert(err, check.ErrorMatches, "^invalid arguments for pipeline, expected \\*bindJobPipelineArgs.$")
}

func (s *S) TestBindJobDBActionJobAlreadyBound(c *check.C) {
	si := ServiceInstance{
		Name: "mysql",
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)

	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	evt := createEvt(c)
	ctx := action.FWContext{
		Params: []interface{}{&bindJobPipelineArgs{job: job, serviceInstance: &si, event: evt}},
	}
	_, err = bindJobDBAction.Forward(ctx)
	c.Assert(err, check.IsNil)
	_, err = bindJobDBAction.Forward(ctx)
	c.Assert(err, check.Equals, ErrJobAlreadyBound)
}

func (s *S) TestBindJobDBActionBackwardRemovesAppFromServiceInstance(c *check.C) {
	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	si := ServiceInstance{
		Name: "mysql",
		Jobs: []string{job.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)

	evt := createEvt(c)
	ctx := action.BWContext{
		Params: []interface{}{&bindJobPipelineArgs{job: job, serviceInstance: &si, event: evt}},
	}
	bindJobDBAction.Backward(ctx)

	err = serviceInstancesCollection.FindOne(context.TODO(), mongoBSON.M{"name": si.Name}).Decode(&si)
	c.Assert(err, check.IsNil)
	c.Assert(si.Apps, check.HasLen, 0)
}

func (s *S) TestBindJobEndpointActionForwardReturnsEnvVars(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()

	service := Service{
		Name:       "mysql",
		Endpoint:   map[string]string{"production": ts.URL},
		Password:   "s3cr3t",
		OwnerTeams: []string{s.team.Name},
	}
	err := Create(context.TODO(), service)
	c.Assert(err, check.IsNil)

	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), si)
	c.Assert(err, check.IsNil)

	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	evt := createEvt(c)
	ctx := action.FWContext{
		Params: []interface{}{&bindJobPipelineArgs{job: job, serviceInstance: &si, event: evt}},
	}
	envs, err := bindJobEndpointAction.Forward(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(envs, check.DeepEquals, map[string]string{
		"DATABASE_USER":     "root",
		"DATABASE_PASSWORD": "s3cr3t",
	})
}

func (s *S) TestBindJobEndpointActionBackward(c *check.C) {
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	service := Service{
		Name:       "mysql",
		Endpoint:   map[string]string{"production": ts.URL},
		Password:   "s3cr3t",
		OwnerTeams: []string{s.team.Name},
	}
	err := Create(context.TODO(), service)
	c.Assert(err, check.IsNil)

	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), si)
	c.Assert(err, check.IsNil)

	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	evt := createEvt(c)
	bwCtx := action.BWContext{
		Params:   []interface{}{&bindJobPipelineArgs{job: job, serviceInstance: &si, event: evt}},
		FWResult: nil,
	}
	bindJobEndpointAction.Backward(bwCtx)
	c.Assert(called, check.Equals, true)
}

func (s *S) TestSetJobBoundEnvsActionForward(c *check.C) {
	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
	}
	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	evt := createEvt(c)
	ctx := action.FWContext{
		Params:   []interface{}{&bindJobPipelineArgs{job: job, serviceInstance: &si, event: evt}},
		Previous: map[string]string{"DATABASE_NAME": "mydb", "DATABASE_USER": "root"},
	}
	result, err := setJobBoundEnvsAction.Forward(ctx)
	c.Assert(err, check.IsNil)

	args := jobTypes.AddInstanceArgs{
		Envs: []bindTypes.ServiceEnvVar{
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_NAME", Value: "mydb"}, ServiceName: "mysql", InstanceName: "my-mysql"},
			{EnvVar: bindTypes.EnvVar{Name: "DATABASE_USER", Value: "root"}, ServiceName: "mysql", InstanceName: "my-mysql"},
		},
	}
	c.Assert(result, check.DeepEquals, args)
}

func (s *S) TestSetJobBoundEnvsActionForwardWrongParameter(c *check.C) {
	ctx := action.FWContext{Params: []interface{}{"something"}}
	_, err := setJobBoundEnvsAction.Forward(ctx)
	c.Assert(err.Error(), check.Equals, "invalid arguments for pipeline, expected *bindJobPipelineArgs.")
}

func (s *S) TestSetJobBoundEnvsActionBackward(c *check.C) {
	var rmCalled bool
	s.mockService.JobService.OnRemoveServiceEnv = func(job *jobTypes.Job, rmArgs jobTypes.RemoveInstanceArgs) error {
		argsToBeRemoved := jobTypes.RemoveInstanceArgs{
			ServiceName:  "mysql",
			InstanceName: "my-mysql",
		}
		c.Assert(rmArgs, check.Equals, argsToBeRemoved)
		rmCalled = true
		return nil
	}

	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
	}
	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")

	evt := createEvt(c)
	ctx := action.BWContext{
		Params:   []interface{}{&bindJobPipelineArgs{job: job, serviceInstance: &si, event: evt}},
		FWResult: nil,
	}
	setJobBoundEnvsAction.Backward(ctx)

	c.Assert(rmCalled, check.Equals, true)
}

func (s *S) TestUnbindJobDBForward(c *check.C) {
	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	srv := Service{
		Name: "mysql",
	}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)

	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Jobs:        []string{job.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)

	buf := bytes.NewBuffer(nil)
	evt := createEvt(c)
	args := bindJobPipelineArgs{
		event:           evt,
		job:             job,
		serviceInstance: &si,
		writer:          buf,
	}
	ctx := action.FWContext{Params: []interface{}{&args}}
	_, err = unbindJobDB.Forward(ctx)
	c.Assert(err, check.IsNil)

	siDB, err := GetServiceInstance(context.TODO(), si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	c.Assert(siDB.Jobs, check.DeepEquals, []string{})
}

func (s *S) TestUnbindJobDBBackward(c *check.C) {
	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	srv := Service{
		Name: "mysql",
	}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)

	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)

	buf := bytes.NewBuffer(nil)
	evt := createEvt(c)
	args := bindJobPipelineArgs{
		event:           evt,
		job:             job,
		serviceInstance: &si,
		writer:          buf,
	}
	ctx := action.BWContext{Params: []interface{}{&args}}
	unbindJobDB.Backward(ctx)
	siDB, err := GetServiceInstance(context.TODO(), si.ServiceName, si.Name)
	c.Assert(err, check.IsNil)
	c.Assert(siDB.Jobs, check.DeepEquals, []string{job.Name})
}

func (s *S) TestUnbindJobEndpointForward(c *check.C) {
	var reqs []*http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	srv := Service{
		Name:     "mysql",
		Endpoint: map[string]string{"production": ts.URL},
		Password: "s3cr3t",
	}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)

	si := ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)

	buf := bytes.NewBuffer(nil)
	evt := createEvt(c)
	args := bindJobPipelineArgs{
		event:           evt,
		job:             job,
		serviceInstance: &si,
		writer:          buf,
	}
	ctx := action.FWContext{Params: []interface{}{&args}}
	_, err = unbindJobEndpoint.Forward(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(reqs, check.HasLen, 1)
	c.Assert(reqs[0].Method, check.Equals, "DELETE")
	c.Assert(reqs[0].URL.Path, check.Equals, "/resources/my-mysql/binds/jobs/test-job")
}

func (s *S) TestUnbindJobEndpointForwardNotFound(c *check.C) {
	var reqs []*http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, r)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	srv := Service{
		Name:     "mysql",
		Endpoint: map[string]string{"production": ts.URL},
		Password: "s3cr3t",
	}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)

	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)

	buf := bytes.NewBuffer(nil)
	evt := createEvt(c)
	args := bindJobPipelineArgs{
		event:           evt,
		job:             job,
		serviceInstance: &si,
		writer:          buf,
	}
	ctx := action.FWContext{Params: []interface{}{&args}}
	_, err = unbindJobEndpoint.Forward(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(reqs, check.HasLen, 1)
	c.Assert(reqs[0].Method, check.Equals, "DELETE")
	c.Assert(reqs[0].URL.Path, check.Equals, "/resources/my-mysql/binds/jobs/test-job")
}

func (s *S) TestUnbindJobEndpointBackward(c *check.C) {
	var reqs []*http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	srv := Service{
		Name:     "mysql",
		Endpoint: map[string]string{"production": ts.URL},
		Password: "s3cr3t",
	}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)

	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)

	buf := bytes.NewBuffer(nil)
	evt := createEvt(c)
	args := bindJobPipelineArgs{
		event:           evt,
		job:             job,
		serviceInstance: &si,
		writer:          buf,
	}
	ctx := action.BWContext{Params: []interface{}{&args}}
	unbindJobEndpoint.Backward(ctx)
	c.Assert(reqs, check.HasLen, 1)
	c.Assert(reqs[0].Method, check.Equals, "PUT")
	c.Assert(reqs[0].URL.Path, check.Equals, "/resources/my-mysql/binds/jobs/test-job")
}

func (s *S) TestRemoveJobBoundEnvsForward(c *check.C) {
	var rmCalled bool
	s.mockService.JobService.OnRemoveServiceEnv = func(job *jobTypes.Job, rmArgs jobTypes.RemoveInstanceArgs) error {
		argsToBeRemoved := jobTypes.RemoveInstanceArgs{
			ServiceName:  "mysql",
			InstanceName: "my-mysql",
		}
		c.Assert(rmArgs, check.Equals, argsToBeRemoved)
		rmCalled = true
		return nil
	}

	srv := Service{
		Name: "mysql",
	}
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	_, err = servicesCollection.InsertOne(context.TODO(), &srv)
	c.Assert(err, check.IsNil)

	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)
	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)

	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	evt := createEvt(c)
	args := bindJobPipelineArgs{
		event:           evt,
		job:             job,
		serviceInstance: &si,
	}
	ctx := action.FWContext{Params: []interface{}{&args}}
	_, err = removeJobBoundEnvs.Forward(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(rmCalled, check.Equals, true)
}

func (s *S) TestReloadJobProvisionerForwardForCronJob(c *check.C) {
	var reloadCalled bool
	s.mockService.JobService.OnUpdateJobProv = func(job *jobTypes.Job) error {
		reloadCalled = true
		return nil
	}

	si := ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}

	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)

	_, err = serviceInstancesCollection.InsertOne(context.TODO(), &si)
	c.Assert(err, check.IsNil)

	job := provisiontest.NewFakeJob("test-job", "test-pool", "test-team-owner")
	evt := createEvt(c)
	args := bindJobPipelineArgs{
		event:           evt,
		job:             job,
		serviceInstance: &si,
	}
	ctx := action.FWContext{Params: []interface{}{&args}}
	_, err = reloadJobProvisioner.Forward(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(reloadCalled, check.Equals, true)
}
