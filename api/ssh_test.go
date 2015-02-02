// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/safe"
	"github.com/tsuru/tsuru/testing"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

func (s *S) TestAppSshWithAppName(c *gocheck.C) {
	a := app.App{
		Name:     "someapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, nil)
	url := fmt.Sprintf("/ssh?:app=%s&width=2&height=2", a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	buf := safe.NewBuffer([]byte("echo teste"))
	recorder := testing.Hijacker{Conn: &testing.FakeConn{buf}}
	err = sshHandler(&recorder, request, s.token)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestAppSshHandlerUnhijackable(c *gocheck.C) {
	a := app.App{
		Name:     "someapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/ssh?:app=%s&width=2&height=2", a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := httptest.NewRecorder()
	err = sshHandler(recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusInternalServerError)
	c.Assert(e.Message, gocheck.Equals, "cannot hijack connection")
}

func (s *S) TestAppSshFailToHijack(c *gocheck.C) {
	a := app.App{
		Name:     "someapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, gocheck.IsNil)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, nil)
	url := fmt.Sprintf("/ssh?:app=%s&width=2&height=2", a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, gocheck.IsNil)
	recorder := testing.Hijacker{
		Err: fmt.Errorf("are you going to hijack the connection? seriously?")}
	err = sshHandler(&recorder, request, s.token)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Code, gocheck.Equals, http.StatusInternalServerError)
	c.Assert(e.Message, gocheck.Equals, recorder.Err.Error())
}
