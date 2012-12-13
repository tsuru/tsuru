// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/globocom/commandmocker"
	"github.com/globocom/tsuru/api/bind"
	"github.com/globocom/tsuru/api/service"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

func (s *S) TestAppIsABinderApp(c *C) {
	var _ bind.App = &App{}
}

func (s *S) TestDestroyShouldUnbindAppFromInstance(c *C) {
	h := testHandler{}
	tsg := s.t.StartGandalfTestServer(&h)
	defer tsg.Close()
	dir, err := commandmocker.Add("juju", "")
	c.Assert(err, IsNil)
	defer commandmocker.Remove(dir)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "my", Endpoint: map[string]string{"production": ts.URL}}
	err = srvc.Create()
	c.Assert(err, IsNil)
	defer db.Session.Services().Remove(bson.M{"_id": srvc.Name})
	instance := service.ServiceInstance{Name: "MyInstance", Apps: []string{"whichapp"}, ServiceName: srvc.Name}
	err = instance.Create()
	c.Assert(err, IsNil)
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": instance.Name})
	a := App{
		Name:      "whichapp",
		Framework: "",
		Teams:     []string{},
		Units: []Unit{
			{Ip: "10.10.10.10", Machine: 1},
		},
	}
	err = CreateApp(&a)
	c.Assert(err, IsNil)
	err = a.Destroy()
	c.Assert(err, IsNil)
	n, err := db.Session.ServiceInstances().Find(bson.M{"apps": bson.M{"$in": []string{a.Name}}}).Count()
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 0)
}
