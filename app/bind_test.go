// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/testing"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

func (s *S) TestAppIsABinderApp(c *gocheck.C) {
	var _ bind.App = &App{}
}

func (s *S) TestDestroyShouldUnbindAppFromInstance(c *gocheck.C) {
	h := testHandler{}
	tsg := testing.StartGandalfTestServer(&h)
	defer tsg.Close()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "my", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": srvc.Name})
	instance := service.ServiceInstance{Name: "MyInstance", Apps: []string{"whichapp"}, ServiceName: srvc.Name}
	err = instance.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": instance.Name})
	a := App{
		Name:     "whichapp",
		Platform: "python",
		Teams:    []string{},
	}
	err = CreateApp(&a, s.user)
	c.Assert(err, gocheck.IsNil)
	app, err := GetByName(a.Name)
	c.Assert(err, gocheck.IsNil)
	err = Delete(app)
	c.Assert(err, gocheck.IsNil)
	n, err := s.conn.ServiceInstances().Find(bson.M{"apps": bson.M{"$in": []string{a.Name}}}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 0)
}
