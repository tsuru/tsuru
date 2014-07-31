// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/tsuru/tsuru/log/testing"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/queue"
	"github.com/tsuru/tsuru/service"
	"labix.org/v2/mgo/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"strings"
)

func (s *S) TestHandleMessageErrors(c *gocheck.C) {
	var data = []struct {
		action      string
		args        []string
		unitName    string
		expectedLog string
	}{
		{
			action:      "unknown-action",
			args:        []string{"does not matter"},
			expectedLog: `Error handling "unknown-action": invalid action.`,
		},
		{
			action:      BindService,
			args:        []string{"nemesis", "xxxx"},
			expectedLog: "Unknown unit in the message.",
		},
		{
			action:      BindService,
			args:        []string{"unknown-app", "xxx"},
			expectedLog: `Error handling "bind-service": app "unknown-app" does not exist.`,
		},
		{
			action:      BindService,
			expectedLog: `Error handling "bind-service": this action requires at least 2 arguments.`,
		},
	}
	a := App{Name: "nemesis"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	s.provisioner.AddUnit(&a, provision.Unit{Name: "totem/0", Status: provision.StatusBuilding})
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	for _, d := range data {
		logger := testing.NewFakeLogger().(*testing.FakeLogger)
		message := queue.Message{Action: d.action}
		if len(d.args) > 0 {
			message.Args = d.args
		}
		handle(&message)
		content := strings.Replace(logger.Buf.String(), "\n", "", -1)
		c.Check(content, gocheck.Equals, d.expectedLog)
	}
}

func (s *S) TestUnitListStarted(c *gocheck.C) {
	var tests = []struct {
		input    []provision.Unit
		expected bool
	}{
		{
			[]provision.Unit{
				{Status: "started"},
				{Status: "started"},
				{Status: "started"},
			},
			true,
		},
		{nil, true},
		{
			[]provision.Unit{
				{Status: "started"},
				{Status: "blabla"},
			},
			false,
		},
		{
			[]provision.Unit{
				{Status: "started"},
				{Status: "unreachable"},
			},
			true,
		},
	}
	for _, t := range tests {
		l := unitList(t.input)
		if got := l.Started(); got != t.expected {
			c.Errorf("l.Started(): want %v. Got %v.", t.expected, got)
		}
	}
}

func (s *S) TestUnitListState(c *gocheck.C) {
	var tests = []struct {
		input    []provision.Unit
		expected string
	}{
		{
			[]provision.Unit{{Status: "started"}, {Status: "started"}}, "started",
		},
		{nil, ""},
		{
			[]provision.Unit{{Status: "started"}, {Status: "pending"}}, "",
		},
		{
			[]provision.Unit{{Status: "error"}}, "error",
		},
		{
			[]provision.Unit{{Status: "pending"}}, "pending",
		},
	}
	for _, t := range tests {
		l := unitList(t.input)
		if got := l.State(); got != t.expected {
			c.Errorf("l.State(): want %q. Got %q.", t.expected, got)
		}
	}
}

func (s *S) TestEnqueueUsesInternalQueue(c *gocheck.C) {
	Enqueue(queue.Message{Action: "do-something"})
	dqueue, _ := qfactory.Get("default")
	_, err := dqueue.Get(1e6)
	c.Assert(err, gocheck.NotNil)
	msg, err := aqueue().Get(1e6)
	c.Assert(err, gocheck.IsNil)
	c.Assert(msg.Action, gocheck.Equals, "do-something")
}

func (s *S) TestHandleBindServiceMessage(c *gocheck.C) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}}
	err := srvc.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Services().Remove(bson.M{"_id": "mysql"})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	instance.Create()
	defer s.conn.ServiceInstances().Remove(bson.M{"_id": "my-mysql"})
	a := App{
		Name: "nemesis",
	}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	s.provisioner.Provision(&a)
	s.provisioner.AddUnits(&a, 1)
	defer s.provisioner.Destroy(&a)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	err = instance.AddApp(a.Name)
	c.Assert(err, gocheck.IsNil)
	err = s.conn.ServiceInstances().Update(bson.M{"name": instance.Name}, instance)
	c.Assert(err, gocheck.IsNil)
	message := queue.Message{Action: BindService, Args: []string{a.Name, a.Units()[0].Name}}
	handle(&message)
	c.Assert(called, gocheck.Equals, true)
}
