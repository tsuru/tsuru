// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestAppIsABinderApp(c *check.C) {
	var _ bind.App = &App{}
}

func (s *S) TestDestroyShouldUnbindAppFromInstance(c *check.C) {
	h := testHandler{}
	tsg := repositorytest.StartGandalfTestServer(&h)
	defer tsg.Close()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "my", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": srvc.Name})
	instance := service.ServiceInstance{Name: "MyInstance", Apps: []string{"whichapp"}, ServiceName: srvc.Name}
	err = instance.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": instance.Name})
	a := App{
		Name:     "whichapp",
		Platform: "python",
		Teams:    []string{},
	}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	app, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	err = Delete(app)
	c.Assert(err, check.IsNil)
	n, err := s.conn.ServiceInstances().Find(bson.M{"apps": bson.M{"$in": []string{a.Name}}}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
}
