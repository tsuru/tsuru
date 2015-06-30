// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/tsurutest"
	"golang.org/x/net/websocket"
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
	defer s.logConn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, check.IsNil)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	m := RunServer(true)
	server := httptest.NewServer(m)
	defer server.Close()
	testServerURL, err := url.Parse(server.URL)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("ws://%s/apps/%s/shell?width=140&height=38&term=xterm", testServerURL.Host, a.Name)
	config, err := websocket.NewConfig(url, "ws://localhost/")
	c.Assert(err, check.IsNil)
	config.Header.Set("Authorization", "bearer "+s.token.GetValue())
	wsConn, err := websocket.DialConfig(config)
	c.Assert(err, check.IsNil)
	defer wsConn.Close()
	_, err = wsConn.Write([]byte("echo test"))
	c.Assert(err, check.IsNil)
	var shells []provision.ShellOptions
	err = tsurutest.WaitCondition(5*time.Second, func() bool {
		unit := s.provisioner.Units(&a)[0]
		shells = s.provisioner.Shells(unit.Name)
		return len(shells) == 1
	})
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
	defer s.logConn.Logs(a.Name).DropCollection()
	err = s.provisioner.Provision(&a)
	c.Assert(err, check.IsNil)
	defer s.provisioner.Destroy(&a)
	s.provisioner.AddUnits(&a, 5, "web", nil)
	unit := s.provisioner.Units(&a)[3]
	m := RunServer(true)
	server := httptest.NewServer(m)
	defer server.Close()
	testServerURL, err := url.Parse(server.URL)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("ws://%s/apps/%s/shell?width=140&height=38&term=xterm&unit=%s", testServerURL.Host, a.Name, unit.Name)
	config, err := websocket.NewConfig(url, "ws://localhost/")
	c.Assert(err, check.IsNil)
	config.Header.Set("Authorization", "bearer "+s.token.GetValue())
	wsConn, err := websocket.DialConfig(config)
	c.Assert(err, check.IsNil)
	defer wsConn.Close()
	_, err = wsConn.Write([]byte("echo test"))
	c.Assert(err, check.IsNil)
	var shells []provision.ShellOptions
	err = tsurutest.WaitCondition(5*time.Second, func() bool {
		shells = s.provisioner.Shells(unit.Name)
		return len(shells) == 1
	})
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
