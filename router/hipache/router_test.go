// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hipache

import (
	"errors"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/router"
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

func (s *S) TestShouldBeRegistered(c *gocheck.C) {
	r, err := router.Get("hipache")
	c.Assert(err, gocheck.IsNil)
	_, ok := r.(hipacheRouter)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestAddBackend(c *gocheck.C) {
	conn = &resultCommandConn{defaultReply: []interface{}{}, fakeConn: &s.conn}
	router := hipacheRouter{}
	err := router.AddBackend("tip")
	c.Assert(err, gocheck.IsNil)
	expected := []command{
		{cmd: "RPUSH", args: []interface{}{"frontend:tip.golang.org", "tip"}},
	}
	c.Assert(s.conn.cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestRemoveBackend(c *gocheck.C) {
	conn = &resultCommandConn{defaultReply: []interface{}{}, fakeConn: &s.conn}
	router := hipacheRouter{}
	err := router.RemoveBackend("tip")
	c.Assert(err, gocheck.IsNil)
	expected := []command{
		{cmd: "DEL", args: []interface{}{"frontend:tip.golang.org"}},
	}
	c.Assert(s.conn.cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestAddRouteWithoutAssemblingFrontend(c *gocheck.C) {
	err := hipacheRouter{}.addRoute("test.com", "10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	expected := []command{{cmd: "RPUSH", args: []interface{}{"test.com", "10.10.10.10"}}}
	c.Assert(s.conn.cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestAddRoute(c *gocheck.C) {
	conn = &resultCommandConn{defaultReply: "", fakeConn: &s.conn}
	router := hipacheRouter{}
	err := router.AddRoute("tip", "http://10.10.10.10:8080")
	c.Assert(err, gocheck.IsNil)
	expected := []command{
		{cmd: "RPUSH", args: []interface{}{"frontend:tip.golang.org", "http://10.10.10.10:8080"}},
		{cmd: "GET", args: []interface{}{"cname:tip"}},
	}
	c.Assert(s.conn.cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestAddTwoRoutes(c *gocheck.C) {
	reply := map[string]interface{}{"GET": "", "SET": "", "LRANGE": []interface{}{[]byte("tip")}, "RPUSH": []interface{}{[]byte{}}}
	conn = &resultCommandConn{reply: reply, fakeConn: &s.conn}
	router := hipacheRouter{}
	err := router.AddRoute("tip", "http://10.10.10.10:8081")
	c.Assert(err, gocheck.IsNil)
	expected := []command{
		{cmd: "RPUSH", args: []interface{}{"frontend:tip.golang.org", "http://10.10.10.10:8081"}},
		{cmd: "GET", args: []interface{}{"cname:tip"}},
	}
	c.Assert(s.conn.cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestAddRouteNoDomainConfigured(c *gocheck.C) {
	old, _ := config.Get("hipache:domain")
	defer config.Set("hipache:domain", old)
	config.Unset("hipache:domain")
	err := hipacheRouter{}.AddRoute("tip", "http://10.10.10.10:8080")
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.op, gocheck.Equals, "add")
}

func (s *S) TestAddRouteConnectFailure(c *gocheck.C) {
	config.Set("hipache:redis-server", "127.0.0.1:6380")
	defer config.Unset("hipache:redis-server")
	conn = nil
	err := hipacheRouter{}.AddRoute("tip", "http://www.tsuru.io")
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.op, gocheck.Equals, "add")
}

func (s *S) TestAddRouteCommandFailure(c *gocheck.C) {
	conn = &failingFakeConn{}
	err := hipacheRouter{}.AddRoute("tip", "http://www.tsuru.io")
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.err.Error(), gocheck.Equals, "Could not add route: I can't do that.")
	c.Assert(e.op, gocheck.Equals, "add")
}

func (s *S) TestAddRouteAlsoUpdatesCNameRecordsWhenExists(c *gocheck.C) {
	reply := map[string]interface{}{"GET": "mycname.com", "SET": "", "LRANGE": []interface{}{[]byte("http://10.10.10.10:8080")}, "RPUSH": []interface{}{[]byte{}}}
	conn = &resultCommandConn{reply: reply, fakeConn: &s.conn}
	router := hipacheRouter{}
	err := router.AddRoute("tip", "http://10.10.10.10:8080")
	c.Assert(err, gocheck.IsNil)
	err = router.SetCName("mycname.com", "tip")
	c.Assert(err, gocheck.IsNil)
	err = router.AddRoute("tip", "http://10.10.10.11:8080")
	c.Assert(err, gocheck.IsNil)
	expected := []command{
		{cmd: "RPUSH", args: []interface{}{"frontend:tip.golang.org", "http://10.10.10.10:8080"}}, // AddRoute
		{cmd: "GET", args: []interface{}{"cname:tip"}},                                            // AddRoute
		{cmd: "RPUSH", args: []interface{}{"frontend:mycname.com", "http://10.10.10.10:8080"}},    // AddRoute, collateral due to fixed redis GET output
		{cmd: "LRANGE", args: []interface{}{"frontend:tip.golang.org", 0, -1}},                    // SetCName
		{cmd: "SET", args: []interface{}{"cname:tip", "mycname.com"}},                             // SetCName
		{cmd: "RPUSH", args: []interface{}{"frontend:mycname.com", "http://10.10.10.10:8080"}},    // SetCName
		{cmd: "RPUSH", args: []interface{}{"frontend:tip.golang.org", "http://10.10.10.11:8080"}}, // AddRoute
		{cmd: "GET", args: []interface{}{"cname:tip"}},                                            // AddRoute
		{cmd: "RPUSH", args: []interface{}{"frontend:mycname.com", "http://10.10.10.11:8080"}},    // AddRoute
	}
	c.Assert(s.conn.cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestRemoveRoute(c *gocheck.C) {
	err := hipacheRouter{}.RemoveRoute("tip", "tip.golang.org")
	c.Assert(err, gocheck.IsNil)
	expected := []command{
		{cmd: "LREM", args: []interface{}{"frontend:tip.golang.org", 0, "tip.golang.org"}},
	}
	c.Assert(s.conn.cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestRemoveRouteNoDomainConfigured(c *gocheck.C) {
	old, _ := config.Get("hipache:domain")
	defer config.Set("hipache:domain", old)
	config.Unset("hipache:domain")
	err := hipacheRouter{}.RemoveRoute("tip", "tip.golang.org")
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.op, gocheck.Equals, "remove")
}

func (s *S) TestRemoveRouteConnectFailure(c *gocheck.C) {
	config.Set("hipache:redis-server", "127.0.0.1:6380")
	defer config.Unset("hipache:redis-server")
	conn = nil
	err := hipacheRouter{}.RemoveRoute("tip", "tip.golang.org")
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.op, gocheck.Equals, "remove")
}

func (s *S) TestRemoveRouteCommandFailure(c *gocheck.C) {
	conn = &failingFakeConn{}
	err := hipacheRouter{}.RemoveRoute("tip", "tip.golang.org")
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.err.Error(), gocheck.Equals, "I can't do that.")
	c.Assert(e.op, gocheck.Equals, "remove")
}

func (s *S) TestSetCName(c *gocheck.C) {
	conn = &resultCommandConn{defaultReply: []interface{}{[]byte("10.10.10.10")}, fakeConn: &s.conn}
	err := hipacheRouter{}.SetCName("myapp.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	expected := []command{
		{cmd: "LRANGE", args: []interface{}{"frontend:myapp.golang.org", 0, -1}},
		{cmd: "SET", args: []interface{}{"cname:myapp", "myapp.com"}},
		{cmd: "RPUSH", args: []interface{}{"frontend:myapp.com", "10.10.10.10"}},
	}
	c.Assert(s.conn.cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestSetCNameWithPreviousRoutes(c *gocheck.C) {
	reply := map[string]interface{}{"GET": "", "SET": "", "LRANGE": []interface{}{[]byte("10.10.10.10"), []byte("10.10.10.11")}, "RPUSH": []interface{}{[]byte{}}}
	conn = &resultCommandConn{reply: reply, fakeConn: &s.conn}
	router := hipacheRouter{}
	err := router.AddRoute("myapp", "10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	err = router.AddRoute("myapp", "10.10.10.11")
	c.Assert(err, gocheck.IsNil)
	err = router.SetCName("mycname.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	expected := []command{
		{cmd: "RPUSH", args: []interface{}{"frontend:myapp.golang.org", "10.10.10.10"}}, // AddRoute call
		{cmd: "GET", args: []interface{}{"cname:myapp"}},                                // AddRoute call
		{cmd: "RPUSH", args: []interface{}{"frontend:myapp.golang.org", "10.10.10.11"}}, // AddRoute call
		{cmd: "GET", args: []interface{}{"cname:myapp"}},                                // AddRoute call
		{cmd: "LRANGE", args: []interface{}{"frontend:myapp.golang.org", 0, -1}},
		{cmd: "SET", args: []interface{}{"cname:myapp", "mycname.com"}},
		{cmd: "RPUSH", args: []interface{}{"frontend:mycname.com", "10.10.10.10"}},
		{cmd: "RPUSH", args: []interface{}{"frontend:mycname.com", "10.10.10.11"}},
	}
	c.Assert(s.conn.cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestSetCNameShouldRecordAppAndCNameOnRedis(c *gocheck.C) {
	conn = &resultCommandConn{defaultReply: []interface{}{[]byte("mycname.com")}, fakeConn: &s.conn}
	router := hipacheRouter{}
	err := router.SetCName("mycname.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	expected := command{cmd: "SET", args: []interface{}{"cname:myapp", "mycname.com"}}
	c.Assert(s.conn.cmds[1], gocheck.DeepEquals, expected)
}

func (s *S) TestUnsetCName(c *gocheck.C) {
	conn = &resultCommandConn{defaultReply: []interface{}{}, fakeConn: &s.conn}
	err := hipacheRouter{}.UnsetCName("myapp.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	expected := []command{
		{cmd: "DEL", args: []interface{}{"cname:myapp"}},
		{cmd: "DEL", args: []interface{}{"frontend:myapp.com"}},
	}
	c.Assert(s.conn.cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestAddr(c *gocheck.C) {
	conn = &resultCommandConn{defaultReply: []interface{}{[]byte("10.10.10.10:8080")}, fakeConn: &s.conn}
	addr, err := hipacheRouter{}.Addr("tip")
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, "tip.golang.org")
	expected := []command{
		{cmd: "LRANGE", args: []interface{}{"frontend:tip.golang.org", 0, 0}},
	}
	c.Assert(s.conn.cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestAddrNoDomainConfigured(c *gocheck.C) {
	old, _ := config.Get("hipache:domain")
	defer config.Set("hipache:domain", old)
	config.Unset("hipache:domain")
	addr, err := hipacheRouter{}.Addr("tip")
	c.Assert(addr, gocheck.Equals, "")
	e, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.op, gocheck.Equals, "get")
}

func (s *S) TestAddrConnectFailure(c *gocheck.C) {
	config.Set("hipache:redis-server", "127.0.0.1:6380")
	defer config.Unset("hipache:redis-server")
	conn = nil
	addr, err := hipacheRouter{}.Addr("tip")
	c.Assert(addr, gocheck.Equals, "")
	e, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.op, gocheck.Equals, "get")
}

func (s *S) TestAddrCommandFailure(c *gocheck.C) {
	conn = &failingFakeConn{}
	addr, err := hipacheRouter{}.Addr("tip")
	c.Assert(addr, gocheck.Equals, "")
	e, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.op, gocheck.Equals, "get")
	c.Assert(e.err.Error(), gocheck.Equals, "I can't do that.")
}

func (s *S) TestAddrRouteNotFound(c *gocheck.C) {
	conn = &resultCommandConn{defaultReply: []interface{}{}, fakeConn: &s.conn}
	addr, err := hipacheRouter{}.Addr("tip")
	c.Assert(addr, gocheck.Equals, "")
	c.Assert(err, gocheck.Equals, errRouteNotFound)
}

func (s *S) TestRouteError(c *gocheck.C) {
	err := &routeError{"add", errors.New("Fatal error.")}
	c.Assert(err.Error(), gocheck.Equals, "Could not add route: Fatal error.")
	err = &routeError{"del", errors.New("Fatal error.")}
	c.Assert(err.Error(), gocheck.Equals, "Could not del route: Fatal error.")
}

func (s *S) TestRemoveElement(c *gocheck.C) {
	err := hipacheRouter{}.removeElement("frontend:myapp.com", "10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	cmds := []command{
		{cmd: "LREM", args: []interface{}{"frontend:myapp.com", 0, "10.10.10.10"}},
	}
	c.Assert(s.conn.cmds, gocheck.DeepEquals, cmds)
}
