// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hipache

import (
	"errors"
	"github.com/globocom/config"
	"launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type S struct {
	conn fakeConn
}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	config.Set("hipache:domain", "golang.org")
}

func (s *S) SetUpTest(c *gocheck.C) {
	s.conn = fakeConn{}
	conn = &s.conn
}

func (s *S) TestConnect(c *gocheck.C) {
	got, err := connect()
	c.Assert(err, gocheck.IsNil)
	c.Assert(got, gocheck.DeepEquals, &s.conn)
}

func (s *S) TestConnectWhenConnIsNil(c *gocheck.C) {
	conn = nil
	got, err := connect()
	c.Assert(err, gocheck.IsNil)
	defer got.Close()
	c.Assert(conn, gocheck.DeepEquals, got)
}

func (s *S) TestConnectWhenConnIsNilAndCannotConnect(c *gocheck.C) {
	config.Set("hipache:redis-server", "127.0.0.1:6380")
	defer config.Unset("hipache:redis-server")
	conn = nil
	got, err := connect()
	c.Assert(got, gocheck.IsNil)
	c.Assert(conn, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestAddRoute(c *gocheck.C) {
	router := Router{}
	err := router.AddRoute("tip", "http://10.10.10.10:8080")
	c.Assert(err, gocheck.IsNil)
	expected := []command{
		{cmd: "RPUSH", args: []interface{}{"frontend:tip.golang.org", "http://10.10.10.10:8080"}},
	}
	c.Assert(s.conn.cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestAddRouteNoDomainConfigured(c *gocheck.C) {
	old, _ := config.Get("hipache:domain")
	defer config.Set("hipache:domain", old)
	config.Unset("hipache:domain")
	err := Router{}.AddRoute("tip", "http://10.10.10.10:8080")
	c.Assert(err, gocheck.NotNil)
	_, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestAddRouteConnectFailure(c *gocheck.C) {
	config.Set("hipache:redis-server", "127.0.0.1:6380")
	defer config.Unset("hipache:redis-server")
	conn = nil
	err := Router{}.AddRoute("tip", "http://www.tsuru.io")
	c.Assert(err, gocheck.NotNil)
	_, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestAddRouteCommandFailure(c *gocheck.C) {
	conn = &failingFakeConn{}
	err := Router{}.AddRoute("tip", "http://www.tsuru.io")
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.err.Error(), gocheck.Equals, "I can't do that.")
}

func (s *S) TestRouteError(c *gocheck.C) {
	err := &routeError{errors.New("Fatal error.")}
	c.Assert(err.Error(), gocheck.Equals, "Could not add route: Fatal error.")
}
