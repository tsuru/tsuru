// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/service"
	check "gopkg.in/check.v1"
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
	err := service.Create(context.TODO(), srvc)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "MyInstance", Apps: []string{"whichapp"}, ServiceName: srvc.Name}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	a := App{
		Name:      "whichapp",
		Platform:  "python",
		TeamOwner: s.team.Name,
	}
	err = CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	app, err := GetByName(context.TODO(), a.Name)
	c.Assert(err, check.IsNil)
	buf := bytes.NewBuffer(nil)
	evt, err := event.New(context.TODO(), &event.Opts{
		Target:   event.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDelete,
		RawOwner: event.Owner{Type: event.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	evt.SetLogWriter(buf)
	err = Delete(context.TODO(), app, evt, "")
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Matches, `(?s).*Done removing application\.`+"\n$")
	n, err := s.conn.ServiceInstances().Find(bson.M{"apps": bson.M{"$in": []string{a.Name}}}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 0)
}
