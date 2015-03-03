// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/safe"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestAppShellWithAppName(c *check.C) {
	a := app.App{
		Name:     "someapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, check.IsNil)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, nil)
	url := fmt.Sprintf("/shell?:app=%s&width=140&height=38&term=xterm", a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer([]byte("echo teste"))
	recorder := provisiontest.Hijacker{Conn: &provisiontest.FakeConn{Buf: buf}}
	err = remoteShellHandler(&recorder, request, s.token)
	c.Assert(err, check.IsNil)
	unit := s.provisioner.Units(&a)[0]
	shells := s.provisioner.Shells(unit.Name)
	c.Assert(shells, check.HasLen, 1)
	c.Assert(shells[0].App.GetName(), check.Equals, a.Name)
	c.Assert(shells[0].Width, check.Equals, 140)
	c.Assert(shells[0].Height, check.Equals, 38)
	c.Assert(shells[0].Term, check.Equals, "xterm")
	c.Assert(shells[0].Unit, check.Equals, "")
}

func (s *S) TestAppShellSpecifyUnit(c *check.C) {
	a := app.App{
		Name:     "someapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, check.IsNil)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 5, nil)
	unit := s.provisioner.Units(&a)[3]
	url := fmt.Sprintf("/shell?:app=%s&width=140&height=38&term=xterm&unit=%s", a.Name, unit.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer([]byte("echo teste"))
	recorder := provisiontest.Hijacker{Conn: &provisiontest.FakeConn{Buf: buf}}
	err = remoteShellHandler(&recorder, request, s.token)
	c.Assert(err, check.IsNil)
	shells := s.provisioner.Shells(unit.Name)
	c.Assert(shells, check.HasLen, 1)
	c.Assert(shells[0].App.GetName(), check.Equals, a.Name)
	c.Assert(shells[0].Width, check.Equals, 140)
	c.Assert(shells[0].Height, check.Equals, 38)
	c.Assert(shells[0].Term, check.Equals, "xterm")
	c.Assert(shells[0].Unit, check.Equals, unit.Name)
	for _, u := range s.provisioner.Units(&a) {
		if u.Name != unit.Name {
			c.Check(s.provisioner.Shells(u.Name), check.HasLen, 0)
		}
	}
}

// TODO(fss): drop this in tsr >= 0.12.0
func (s *S) TestAppShellSpecifyUnitLegacy(c *check.C) {
	a := app.App{
		Name:     "someapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, check.IsNil)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 5, nil)
	unit := s.provisioner.Units(&a)[3]
	url := fmt.Sprintf("/shell?:app=%s&width=140&height=38&term=xterm&container_id=%s", a.Name, unit.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	buf := safe.NewBuffer([]byte("echo teste"))
	recorder := provisiontest.Hijacker{Conn: &provisiontest.FakeConn{Buf: buf}}
	err = remoteShellHandler(&recorder, request, s.token)
	c.Assert(err, check.IsNil)
	shells := s.provisioner.Shells(unit.Name)
	c.Assert(shells, check.HasLen, 1)
	c.Assert(shells[0].App.GetName(), check.Equals, a.Name)
	c.Assert(shells[0].Width, check.Equals, 140)
	c.Assert(shells[0].Height, check.Equals, 38)
	c.Assert(shells[0].Term, check.Equals, "xterm")
	c.Assert(shells[0].Unit, check.Equals, unit.Name)
	for _, u := range s.provisioner.Units(&a) {
		if u.Name != unit.Name {
			c.Check(s.provisioner.Shells(u.Name), check.HasLen, 0)
		}
	}
}

func (s *S) TestAppShellHandlerUnhijackable(c *check.C) {
	a := app.App{
		Name:     "someapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	url := fmt.Sprintf("/shell?:app=%s&width=2&height=2", a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = remoteShellHandler(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(e.Message, check.Equals, "cannot hijack connection")
}

func (s *S) TestAppShellFailToHijack(c *check.C) {
	a := app.App{
		Name:     "someapp",
		Platform: "zend",
		Teams:    []string{s.team.Name},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": a.Name})
	defer s.conn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, check.IsNil)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, nil)
	url := fmt.Sprintf("/shell?:app=%s&width=2&height=2", a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := provisiontest.Hijacker{
		Err: fmt.Errorf("are you going to hijack the connection? seriously?")}
	err = remoteShellHandler(&recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(e.Message, check.Equals, recorder.Err.Error())
}
