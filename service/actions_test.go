// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"github.com/globocom/tsuru/action"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
)

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
