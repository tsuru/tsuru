// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/service"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestAppIsABinderApp(c *check.C) {
	var _ bind.App = &App{}
}

func (s *S) TestDeleteShouldUnbindAppFromInstance(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "my", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := srvc.Create()
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "MyInstance", Apps: []string{"whichapp"}, ServiceName: srvc.Name}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	a := App{
		Name:      "whichapp",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	err = CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	app, err := GetByName(a.Name)
	c.Assert(err, check.IsNil)
	Delete(app, nil)
	n, err := s.conn.ServiceInstances().Find(bson.M{"apps": bson.M{"$in": []string{a.Name}}}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
}
