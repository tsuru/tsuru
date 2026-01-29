// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/service"
	appTypes "github.com/tsuru/tsuru/types/app"
	eventTypes "github.com/tsuru/tsuru/types/event"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	check "gopkg.in/check.v1"
)

func (s *S) TestDeleteShouldUnbindAppFromInstance(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "my", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := service.Create(context.TODO(), srvc)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "MyInstance", Apps: []string{"whichapp"}, ServiceName: srvc.Name}

	serviceInstancesCollection, err := storagev2.ServiceInstancesCollection()
	c.Assert(err, check.IsNil)

	_, err = serviceInstancesCollection.InsertOne(context.TODO(), instance)
	c.Assert(err, check.IsNil)
	a := appTypes.App{
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
		Target:   eventTypes.Target{Type: "app", Value: a.Name},
		Kind:     permission.PermAppDelete,
		RawOwner: eventTypes.Owner{Type: eventTypes.OwnerTypeUser, Name: s.user.Email},
		Allowed:  event.Allowed(permission.PermApp),
	})
	c.Assert(err, check.IsNil)
	evt.SetLogWriter(buf)
	err = Delete(context.TODO(), app, evt, "")
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Matches, `(?s).*Done removing application\. ----`+"\n$")
	n, err := serviceInstancesCollection.CountDocuments(context.TODO(), mongoBSON.M{"apps": mongoBSON.M{"$in": []string{a.Name}}})
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, int64(0))
}
