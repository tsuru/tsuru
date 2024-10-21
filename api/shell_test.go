// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/tsurutest"
	appTypes "github.com/tsuru/tsuru/types/app"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"golang.org/x/net/websocket"
	check "gopkg.in/check.v1"
)

func (s *S) TestAppShellWithAppName(c *check.C) {
	a := appTypes.App{
		Name:      "someapp",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(context.TODO(), &a, 1, "web", nil, nil)
	c.Assert(err, check.IsNil)
	server := httptest.NewServer(s.testServer)
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
	var shells []provision.ExecOptions
	err = tsurutest.WaitCondition(5*time.Second, func() bool {
		units, unitsErr := s.provisioner.Units(context.TODO(), &a)
		c.Assert(unitsErr, check.IsNil)
		unit := units[0]
		shells = s.provisioner.Execs(unit.ID)
		return len(shells) == 1
	})
	c.Assert(err, check.IsNil)
	c.Assert(shells[0].App.GetName(), check.Equals, a.Name)
	c.Assert(shells[0].Width, check.Equals, 140)
	c.Assert(shells[0].Height, check.Equals, 38)
	c.Assert(shells[0].Term, check.Equals, "xterm")
	units, err := s.provisioner.Units(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(shells[0].Units, check.DeepEquals, []string{units[0].ID})
}

func (s *S) TestAppShellWithAppNameInvalidPermission(c *check.C) {
	a := appTypes.App{
		Name:      "someapp",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(context.TODO(), &a, 1, "web", nil, nil)
	c.Assert(err, check.IsNil)
	server := httptest.NewServer(s.testServer)
	defer server.Close()
	testServerURL, err := url.Parse(server.URL)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	url := fmt.Sprintf("ws://%s/apps/%s/shell?width=140&height=38&term=xterm", testServerURL.Host, a.Name)
	config, err := websocket.NewConfig(url, "ws://localhost/")
	c.Assert(err, check.IsNil)
	config.Header.Set("Authorization", "bearer "+token.GetValue())
	wsConn, err := websocket.DialConfig(config)
	c.Assert(err, check.IsNil)
	defer wsConn.Close()
	_, err = wsConn.Write([]byte("echo test"))
	c.Assert(err, check.IsNil)
	var result string
	err = tsurutest.WaitCondition(5*time.Second, func() bool {
		part, readErr := io.ReadAll(wsConn)
		if readErr != nil {
			return false
		}
		result += string(part)
		return result == "Error: You don't have permission to do this action\n"
	})
	c.Assert(err, check.IsNil)
}

func (s *S) TestAppShellSpecifyUnit(c *check.C) {
	a := appTypes.App{
		Name:      "someapp",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(context.TODO(), &a, 5, "web", nil, nil)
	c.Assert(err, check.IsNil)
	units, err := s.provisioner.Units(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	unit := units[3]
	server := httptest.NewServer(s.testServer)
	defer server.Close()
	testServerURL, err := url.Parse(server.URL)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("ws://%s/apps/%s/shell?width=140&height=38&term=xterm&unit=%s", testServerURL.Host, a.Name, unit.ID)
	config, err := websocket.NewConfig(url, "ws://localhost/")
	c.Assert(err, check.IsNil)
	config.Header.Set("Authorization", "bearer "+s.token.GetValue())
	wsConn, err := websocket.DialConfig(config)
	c.Assert(err, check.IsNil)
	defer wsConn.Close()
	_, err = wsConn.Write([]byte("echo test"))
	c.Assert(err, check.IsNil)
	var shells []provision.ExecOptions
	err = tsurutest.WaitCondition(5*time.Second, func() bool {
		shells = s.provisioner.Execs(unit.ID)
		return len(shells) == 1
	})
	c.Assert(err, check.IsNil)
	c.Assert(shells, check.HasLen, 1)
	c.Assert(shells[0].App.GetName(), check.Equals, a.Name)
	c.Assert(shells[0].Width, check.Equals, 140)
	c.Assert(shells[0].Height, check.Equals, 38)
	c.Assert(shells[0].Term, check.Equals, "xterm")
	c.Assert(shells[0].Units, check.DeepEquals, []string{unit.ID})
	units, err = s.provisioner.Units(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	for _, u := range units {
		if u.ID != unit.ID {
			c.Check(s.provisioner.Execs(u.ID), check.HasLen, 0)
		}
	}
}

func (s *S) TestAppShellIsolated(c *check.C) {
	a := appTypes.App{
		Name:      "someapp",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(context.TODO(), &a, 5, "web", nil, nil)
	c.Assert(err, check.IsNil)
	units, err := s.provisioner.Units(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	unit := units[3]
	server := httptest.NewServer(s.testServer)
	defer server.Close()
	testServerURL, err := url.Parse(server.URL)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("ws://%s/apps/%s/shell?isolated=true&width=140&height=38&term=xterm&unit=%s", testServerURL.Host, a.Name, unit.ID)
	config, err := websocket.NewConfig(url, "ws://localhost/")
	c.Assert(err, check.IsNil)
	config.Header.Set("Authorization", "bearer "+s.token.GetValue())
	wsConn, err := websocket.DialConfig(config)
	c.Assert(err, check.IsNil)
	defer wsConn.Close()
	_, err = wsConn.Write([]byte("echo test"))
	c.Assert(err, check.IsNil)
	var shells []provision.ExecOptions
	err = tsurutest.WaitCondition(5*time.Second, func() bool {
		shells = s.provisioner.Execs("isolated")
		return len(shells) == 1
	})
	c.Assert(err, check.IsNil)
	c.Assert(shells, check.HasLen, 1)
	c.Assert(shells[0].App.GetName(), check.Equals, a.Name)
	c.Assert(shells[0].Width, check.Equals, 140)
	c.Assert(shells[0].Height, check.Equals, 38)
	c.Assert(shells[0].Term, check.Equals, "xterm")
	c.Assert(shells[0].Units, check.HasLen, 0)
	units, err = s.provisioner.Units(context.TODO(), &a)
	c.Assert(err, check.IsNil)
	for _, u := range units {
		if u.ID != unit.ID {
			c.Check(s.provisioner.Execs(u.ID), check.HasLen, 0)
		}
	}
}

func (s *S) TestAppShellUnauthorizedError(c *check.C) {
	a := appTypes.App{
		Name:      "someapp",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(context.TODO(), &a, 1, "web", nil, nil)
	c.Assert(err, check.IsNil)
	server := httptest.NewServer(s.testServer)
	defer server.Close()
	testServerURL, err := url.Parse(server.URL)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("ws://%s/apps/%s/shell?width=140&height=38&term=xterm", testServerURL.Host, a.Name)
	config, err := websocket.NewConfig(url, "ws://localhost/")
	c.Assert(err, check.IsNil)
	wsConn, err := websocket.DialConfig(config)
	c.Assert(err, check.IsNil)
	defer wsConn.Close()
	_, err = wsConn.Write([]byte("echo test"))
	c.Assert(err, check.IsNil)
	var result string
	err = tsurutest.WaitCondition(5*time.Second, func() bool {
		part, readErr := io.ReadAll(wsConn)
		if readErr != nil {
			return false
		}
		result += string(part)
		return result == "Error: no token provided or session expired, please login again\n"
	})
	c.Assert(err, check.IsNil)
}

func (s *S) TestAppShellGenericError(c *check.C) {
	server := httptest.NewServer(s.testServer)
	defer server.Close()
	testServerURL, err := url.Parse(server.URL)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("ws://%s/apps/someapp/shell?width=140&height=38&term=xterm", testServerURL.Host)
	config, err := websocket.NewConfig(url, "ws://localhost/")
	c.Assert(err, check.IsNil)
	config.Header.Set("Authorization", "bearer "+s.token.GetValue())
	wsConn, err := websocket.DialConfig(config)
	c.Assert(err, check.IsNil)
	defer wsConn.Close()
	_, err = wsConn.Write([]byte("echo test"))
	c.Assert(err, check.IsNil)
	var result string
	err = tsurutest.WaitCondition(5*time.Second, func() bool {
		part, readErr := io.ReadAll(wsConn)
		if readErr != nil {
			c.Log(readErr)
			return false
		}
		result += string(part)
		return result == "Error: App someapp not found.\n"
	})
	c.Assert(err, check.IsNil)
}
