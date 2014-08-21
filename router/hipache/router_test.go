// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hipache

import (
	"errors"
	"testing"

	"github.com/garyburd/redigo/redis"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/router"
	rtesting "github.com/tsuru/tsuru/testing/redis"
	"launchpad.net/gocheck"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type S struct {
	pool *redis.Pool
	fake *rtesting.FakeRedisConn
	conn *db.Storage
}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	config.Set("hipache:domain", "golang.org")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "router_hipache_tests")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TearDownSuite(c *gocheck.C) {
	s.conn.Collection("router_hipache_tests").Database.DropDatabase()
}

func (s *S) SetUpTest(c *gocheck.C) {
	s.fake = &rtesting.FakeRedisConn{}
	s.pool = redis.NewPool(fakeConnect, 5)
	pool = s.pool
	conn = s.fake
}

func (s *S) TestConnect(c *gocheck.C) {
	got := connect()
	defer got.Close()
	c.Assert(got, gocheck.NotNil)
	_, err := got.Do("PING")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestConnectWhenPoolIsNil(c *gocheck.C) {
	pool = nil
	got := connect()
	_, err := got.Do("PING")
	c.Assert(err, gocheck.IsNil)
	got.Close()
	c.Assert(pool, gocheck.NotNil)
}

func (s *S) TestConnectWhenConnIsNilAndCannotConnect(c *gocheck.C) {
	config.Set("hipache:redis-server", "127.0.0.1:6380")
	defer config.Unset("hipache:redis-server")
	pool = nil
	got := connect()
	_, err := got.Do("PING")
	c.Assert(err, gocheck.NotNil)
	got.Close()
}

func (s *S) TestShouldBeRegistered(c *gocheck.C) {
	r, err := router.Get("hipache")
	c.Assert(err, gocheck.IsNil)
	_, ok := r.(hipacheRouter)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestAddBackend(c *gocheck.C) {
	conn = &rtesting.ResultCommandRedisConn{
		FakeRedisConn: s.fake,
		DefaultReply:  []interface{}{},
	}
	router := hipacheRouter{}
	err := router.AddBackend("tip")
	c.Assert(err, gocheck.IsNil)
	expected := []rtesting.RedisCommand{
		{Cmd: "RPUSH", Args: []interface{}{"frontend:tip.golang.org", "tip"},
			Type: rtesting.CmdDo},
	}
	c.Assert(s.fake.Cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestRemoveBackend(c *gocheck.C) {
	reply := map[string]interface{}{"GET": ""}
	conn = &rtesting.ResultCommandRedisConn{Reply: reply, FakeRedisConn: s.fake}
	router := hipacheRouter{}
	err := router.RemoveBackend("tip")
	c.Assert(err, gocheck.IsNil)
	expected := []rtesting.RedisCommand{
		{Cmd: "DEL", Args: []interface{}{"frontend:tip.golang.org"},
			Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:tip"},
			Type: rtesting.CmdDo},
	}
	c.Assert(s.fake.Cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestRemoveBackendAlsoRemovesRelatedCNameBackendAndControlRecord(c *gocheck.C) {
	reply := map[string]interface{}{"GET": "mycname.com"}
	conn = &rtesting.ResultCommandRedisConn{Reply: reply, FakeRedisConn: s.fake}
	router := hipacheRouter{}
	err := router.AddBackend("tip")
	c.Assert(err, gocheck.IsNil)
	err = router.RemoveBackend("tip")
	c.Assert(err, gocheck.IsNil)
	expected := []rtesting.RedisCommand{
		{Cmd: "RPUSH", Args: []interface{}{"frontend:tip.golang.org", "tip"},
			Type: rtesting.CmdDo},
		{Cmd: "DEL", Args: []interface{}{"frontend:tip.golang.org"},
			Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:tip"},
			Type: rtesting.CmdDo},
		{Cmd: "DEL", Args: []interface{}{"frontend:mycname.com"},
			Type: rtesting.CmdDo},
		{Cmd: "DEL", Args: []interface{}{"cname:tip"},
			Type: rtesting.CmdDo},
	}
	c.Assert(s.fake.Cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestAddRouteWithoutAssemblingFrontend(c *gocheck.C) {
	err := hipacheRouter{}.addRoute("test.com", "10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	expected := []rtesting.RedisCommand{
		{Cmd: "RPUSH", Args: []interface{}{"test.com", "10.10.10.10"},
			Type: rtesting.CmdDo},
	}
	c.Assert(s.fake.Cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestAddRoute(c *gocheck.C) {
	conn = &rtesting.ResultCommandRedisConn{DefaultReply: "", FakeRedisConn: s.fake}
	router := hipacheRouter{}
	err := router.AddRoute("tip", "http://10.10.10.10:8080")
	c.Assert(err, gocheck.IsNil)
	expected := []rtesting.RedisCommand{
		{Cmd: "RPUSH", Args: []interface{}{"frontend:tip.golang.org", "http://10.10.10.10:8080"},
			Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:tip"},
			Type: rtesting.CmdDo},
	}
	c.Assert(s.fake.Cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestAddTwoRoutes(c *gocheck.C) {
	reply := map[string]interface{}{"GET": "", "SET": "", "LRANGE": []interface{}{[]byte("tip")}, "RPUSH": []interface{}{[]byte{}}}
	conn = &rtesting.ResultCommandRedisConn{Reply: reply, FakeRedisConn: s.fake}
	router := hipacheRouter{}
	err := router.AddRoute("tip", "http://10.10.10.10:8081")
	c.Assert(err, gocheck.IsNil)
	expected := []rtesting.RedisCommand{
		{Cmd: "RPUSH", Args: []interface{}{"frontend:tip.golang.org", "http://10.10.10.10:8081"},
			Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:tip"}, Type: rtesting.CmdDo},
	}
	c.Assert(s.fake.Cmds, gocheck.DeepEquals, expected)
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
	pool = nil
	err := hipacheRouter{}.AddRoute("tip", "http://www.tsuru.io")
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.op, gocheck.Equals, "add")
}

func (s *S) TestAddRouteCommandFailure(c *gocheck.C) {
	conn = &rtesting.FailingFakeRedisConn{}
	err := hipacheRouter{}.AddRoute("tip", "http://www.tsuru.io")
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.err.Error(), gocheck.Equals, "Could not add route: I can't do that.")
	c.Assert(e.op, gocheck.Equals, "add")
}

func (s *S) TestAddRouteAlsoUpdatesCNameRecordsWhenExists(c *gocheck.C) {
	reply := map[string]interface{}{"GET": "mycname.com", "SET": "", "LRANGE": []interface{}{[]byte("http://10.10.10.10:8080")}, "RPUSH": []interface{}{[]byte{}}}
	conn = &rtesting.ResultCommandRedisConn{Reply: reply, FakeRedisConn: s.fake}
	router := hipacheRouter{}
	err := router.AddRoute("tip", "http://10.10.10.10:8080")
	c.Assert(err, gocheck.IsNil)
	err = router.SetCName("mycname.com", "tip")
	c.Assert(err, gocheck.IsNil)
	err = router.AddRoute("tip", "http://10.10.10.11:8080")
	c.Assert(err, gocheck.IsNil)
	expected := []rtesting.RedisCommand{
		{Cmd: "RPUSH", Args: []interface{}{"frontend:tip.golang.org", "http://10.10.10.10:8080"},
			Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:tip"}, Type: rtesting.CmdDo},
		{Cmd: "RPUSH", Args: []interface{}{"frontend:mycname.com", "http://10.10.10.10:8080"},
			Type: rtesting.CmdDo},
		{Cmd: "LRANGE", Args: []interface{}{"frontend:tip.golang.org", 0, -1},
			Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:tip"}, Type: rtesting.CmdDo},
		{Cmd: "DEL", Args: []interface{}{"cname:tip"}, Type: rtesting.CmdDo},
		{Cmd: "DEL", Args: []interface{}{"frontend:mycname.com"}, Type: rtesting.CmdDo},
		{Cmd: "SET", Args: []interface{}{"cname:tip", "mycname.com"}, Type: rtesting.CmdDo},
		{Cmd: "RPUSH", Args: []interface{}{"frontend:mycname.com", "http://10.10.10.10:8080"},
			Type: rtesting.CmdDo},
		{Cmd: "RPUSH", Args: []interface{}{"frontend:tip.golang.org", "http://10.10.10.11:8080"},
			Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:tip"}, Type: rtesting.CmdDo},
		{Cmd: "RPUSH", Args: []interface{}{"frontend:mycname.com", "http://10.10.10.11:8080"},
			Type: rtesting.CmdDo},
	}
	c.Assert(s.fake.Cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestRemoveRoute(c *gocheck.C) {
	reply := map[string]interface{}{"GET": "", "LRANGE": []interface{}{[]byte("10.10.10.11")}}
	conn = &rtesting.ResultCommandRedisConn{Reply: reply, FakeRedisConn: s.fake}
	router := hipacheRouter{}
	err := router.AddBackend("tip")
	c.Assert(err, gocheck.IsNil)
	err = router.RemoveRoute("tip", "tip.golang.org")
	c.Assert(err, gocheck.IsNil)
	expected := []rtesting.RedisCommand{
		{Cmd: "RPUSH", Args: []interface{}{"frontend:tip.golang.org", "tip"},
			Type: rtesting.CmdDo},
		{Cmd: "LREM", Args: []interface{}{"frontend:tip.golang.org", 0, "tip.golang.org"},
			Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:tip"}, Type: rtesting.CmdDo},
	}
	c.Assert(s.fake.Cmds, gocheck.DeepEquals, expected)
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
	pool = nil
	err := hipacheRouter{}.RemoveRoute("tip", "tip.golang.org")
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.op, gocheck.Equals, "remove")
}

func (s *S) TestRemoveRouteCommandFailure(c *gocheck.C) {
	conn = &rtesting.FailingFakeRedisConn{}
	err := hipacheRouter{}.RemoveRoute("tip", "tip.golang.org")
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.err.Error(), gocheck.Equals, "I can't do that.")
	c.Assert(e.op, gocheck.Equals, "remove")
}

func (s *S) TestRemoveRouteAlsoRemovesRespectiveCNameRecord(c *gocheck.C) {
	reply := map[string]interface{}{"GET": "tip.cname.com", "LRANGE": []interface{}{[]byte("10.10.10.11")}}
	conn = &rtesting.ResultCommandRedisConn{Reply: reply, FakeRedisConn: s.fake}
	router := hipacheRouter{}
	err := router.AddBackend("tip")
	c.Assert(err, gocheck.IsNil)
	err = router.RemoveRoute("tip", "tip.golang.org")
	c.Assert(err, gocheck.IsNil)
	expected := []rtesting.RedisCommand{
		{Cmd: "RPUSH", Args: []interface{}{"frontend:tip.golang.org", "tip"},
			Type: rtesting.CmdDo},
		{Cmd: "LREM", Args: []interface{}{"frontend:tip.golang.org", 0, "tip.golang.org"},
			Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:tip"}, Type: rtesting.CmdDo},
		{Cmd: "LREM", Args: []interface{}{"frontend:tip.cname.com", 0, "tip.golang.org"},
			Type: rtesting.CmdDo},
	}
	c.Assert(s.fake.Cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestGetCName(c *gocheck.C) {
	conn = &rtesting.ResultCommandRedisConn{DefaultReply: "coolcname.com", FakeRedisConn: s.fake}
	cname, err := hipacheRouter{}.getCName("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(cname, gocheck.Equals, "coolcname.com")
	expected := []rtesting.RedisCommand{
		{Cmd: "GET", Args: []interface{}{"cname:myapp"}, Type: rtesting.CmdDo},
	}
	c.Assert(s.fake.Cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestGetCNameIgnoresErrNil(c *gocheck.C) {
	reply := map[string]interface{}{"GET": nil}
	conn = &rtesting.ResultCommandRedisConn{Reply: reply, FakeRedisConn: s.fake}
	cname, err := hipacheRouter{}.getCName("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(cname, gocheck.Equals, "")
}

func (s *S) TestSetCName(c *gocheck.C) {
	conn = &rtesting.ResultCommandRedisConn{
		DefaultReply:  []interface{}{[]byte("10.10.10.10")},
		FakeRedisConn: s.fake,
	}
	router := hipacheRouter{}
	err := router.AddBackend("myapp")
	c.Assert(err, gocheck.IsNil)
	err = router.SetCName("myapp.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	expected := []rtesting.RedisCommand{
		{Cmd: "RPUSH", Args: []interface{}{"frontend:myapp.golang.org", "myapp"},
			Type: rtesting.CmdDo},
		{Cmd: "LRANGE", Args: []interface{}{"frontend:myapp.golang.org", 0, -1},
			Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:myapp"}, Type: rtesting.CmdDo},
		{Cmd: "SET", Args: []interface{}{"cname:myapp", "myapp.com"}, Type: rtesting.CmdDo},
		{Cmd: "RPUSH", Args: []interface{}{"frontend:myapp.com", "10.10.10.10"},
			Type: rtesting.CmdDo},
	}
	c.Assert(s.fake.Cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestSetCNameWithPreviousRoutes(c *gocheck.C) {
	reply := map[string]interface{}{
		"GET":    "",
		"SET":    "",
		"LRANGE": []interface{}{[]byte("10.10.10.10"), []byte("10.10.10.11")},
		"RPUSH":  []interface{}{[]byte{}},
	}
	conn = &rtesting.ResultCommandRedisConn{Reply: reply, FakeRedisConn: s.fake}
	router := hipacheRouter{}
	err := router.AddBackend("myapp")
	c.Assert(err, gocheck.IsNil)
	err = router.AddRoute("myapp", "10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	err = router.AddRoute("myapp", "10.10.10.11")
	c.Assert(err, gocheck.IsNil)
	err = router.SetCName("mycname.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	expected := []rtesting.RedisCommand{
		{Cmd: "RPUSH", Args: []interface{}{"frontend:myapp.golang.org", "myapp"},
			Type: rtesting.CmdDo},
		{Cmd: "RPUSH", Args: []interface{}{"frontend:myapp.golang.org", "10.10.10.10"},
			Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:myapp"}, Type: rtesting.CmdDo},
		{Cmd: "RPUSH", Args: []interface{}{"frontend:myapp.golang.org", "10.10.10.11"},
			Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:myapp"}, Type: rtesting.CmdDo},
		{Cmd: "LRANGE", Args: []interface{}{"frontend:myapp.golang.org", 0, -1},
			Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:myapp"}, Type: rtesting.CmdDo},
		{Cmd: "SET", Args: []interface{}{"cname:myapp", "mycname.com"}, Type: rtesting.CmdDo},
		{Cmd: "RPUSH", Args: []interface{}{"frontend:mycname.com", "10.10.10.10"},
			Type: rtesting.CmdDo},
		{Cmd: "RPUSH", Args: []interface{}{"frontend:mycname.com", "10.10.10.11"},
			Type: rtesting.CmdDo},
	}
	c.Assert(s.fake.Cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestSetCNameShouldRecordAppAndCNameOnRedis(c *gocheck.C) {
	conn = &rtesting.ResultCommandRedisConn{
		DefaultReply:  []interface{}{[]byte("mycname.com")},
		FakeRedisConn: s.fake,
	}
	router := hipacheRouter{}
	err := router.AddBackend("myapp")
	c.Assert(err, gocheck.IsNil)
	err = router.SetCName("mycname.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	expected := rtesting.RedisCommand{
		Cmd:  "SET",
		Args: []interface{}{"cname:myapp", "mycname.com"},
		Type: rtesting.CmdDo,
	}
	c.Assert(s.fake.Cmds[3], gocheck.DeepEquals, expected)
}

func (s *S) TestSetCNameRemovesPreviousDefinedCNamesAndKeepItsRoutes(c *gocheck.C) {
	reply := map[string]interface{}{"GET": "mycname.com", "SET": "", "LRANGE": []interface{}{[]byte("10.10.10.10")}, "RPUSH": []interface{}{[]byte{}}}
	conn = &rtesting.ResultCommandRedisConn{Reply: reply, FakeRedisConn: s.fake}
	router := hipacheRouter{}
	err := router.AddBackend("myapp")
	c.Assert(err, gocheck.IsNil)
	err = router.AddRoute("myapp", "10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	err = router.SetCName("mycname.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	err = router.SetCName("myothercname.com", "myapp")
	expected := []rtesting.RedisCommand{
		{Cmd: "RPUSH", Args: []interface{}{"frontend:myapp.golang.org", "myapp"}, Type: rtesting.CmdDo},
		{Cmd: "RPUSH", Args: []interface{}{"frontend:myapp.golang.org", "10.10.10.10"}, Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:myapp"}, Type: rtesting.CmdDo},
		{Cmd: "RPUSH", Args: []interface{}{"frontend:mycname.com", "10.10.10.10"}, Type: rtesting.CmdDo},
		{Cmd: "LRANGE", Args: []interface{}{"frontend:myapp.golang.org", 0, -1}, Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:myapp"}, Type: rtesting.CmdDo},
		{Cmd: "DEL", Args: []interface{}{"cname:myapp"}, Type: rtesting.CmdDo},
		{Cmd: "DEL", Args: []interface{}{"frontend:mycname.com"}, Type: rtesting.CmdDo},
		{Cmd: "SET", Args: []interface{}{"cname:myapp", "mycname.com"}, Type: rtesting.CmdDo},
		{Cmd: "RPUSH", Args: []interface{}{"frontend:mycname.com", "10.10.10.10"}, Type: rtesting.CmdDo},
		{Cmd: "LRANGE", Args: []interface{}{"frontend:myapp.golang.org", 0, -1}, Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:myapp"}, Type: rtesting.CmdDo},
		{Cmd: "DEL", Args: []interface{}{"cname:myapp"}, Type: rtesting.CmdDo},
		{Cmd: "DEL", Args: []interface{}{"frontend:mycname.com"}, Type: rtesting.CmdDo},
		{Cmd: "SET", Args: []interface{}{"cname:myapp", "myothercname.com"}, Type: rtesting.CmdDo},
		{Cmd: "RPUSH", Args: []interface{}{"frontend:myothercname.com", "10.10.10.10"}, Type: rtesting.CmdDo},
	}
	c.Assert(s.fake.Cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestSetCNameValidatesCNameAccordingToDomainConfig(c *gocheck.C) {
	reply := map[string]interface{}{"GET": "", "SET": "", "LRANGE": []interface{}{[]byte{}}, "RPUSH": []interface{}{[]byte{}}}
	conn = &rtesting.ResultCommandRedisConn{Reply: reply, FakeRedisConn: s.fake}
	router := hipacheRouter{}
	err := router.SetCName("mycname.golang.org", "myapp")
	c.Assert(err, gocheck.NotNil)
	expected := "Could not setCName route: Invalid CNAME mycname.golang.org. You can't use tsuru's application domain."
	c.Assert(err.Error(), gocheck.Equals, expected)
}

func (s *S) TestUnsetCName(c *gocheck.C) {
	conn = &rtesting.ResultCommandRedisConn{
		DefaultReply:  []interface{}{},
		FakeRedisConn: s.fake,
	}
	err := hipacheRouter{}.UnsetCName("myapp.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	expected := []rtesting.RedisCommand{
		{Cmd: "DEL", Args: []interface{}{"cname:myapp"}, Type: rtesting.CmdDo},
		{Cmd: "DEL", Args: []interface{}{"frontend:myapp.com"}, Type: rtesting.CmdDo},
	}
	c.Assert(s.fake.Cmds, gocheck.DeepEquals, expected)
}

func (s *S) TestAddr(c *gocheck.C) {
	conn = &rtesting.ResultCommandRedisConn{
		DefaultReply:  []interface{}{[]byte("10.10.10.10:8080")},
		FakeRedisConn: s.fake,
	}
	addr, err := hipacheRouter{}.Addr("tip")
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, "tip.golang.org")
	expected := []rtesting.RedisCommand{
		{Cmd: "LRANGE", Args: []interface{}{"frontend:tip.golang.org", 0, 0}, Type: rtesting.CmdDo},
	}
	c.Assert(s.fake.Cmds, gocheck.DeepEquals, expected)
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
	pool = nil
	addr, err := hipacheRouter{}.Addr("tip")
	c.Assert(addr, gocheck.Equals, "")
	e, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.op, gocheck.Equals, "get")
}

func (s *S) TestAddrCommandFailure(c *gocheck.C) {
	conn = &rtesting.FailingFakeRedisConn{}
	addr, err := hipacheRouter{}.Addr("tip")
	c.Assert(addr, gocheck.Equals, "")
	e, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.op, gocheck.Equals, "get")
	c.Assert(e.err.Error(), gocheck.Equals, "I can't do that.")
}

func (s *S) TestAddrRouteNotFound(c *gocheck.C) {
	conn = &rtesting.ResultCommandRedisConn{
		DefaultReply:  []interface{}{},
		FakeRedisConn: s.fake,
	}
	addr, err := hipacheRouter{}.Addr("tip")
	c.Assert(addr, gocheck.Equals, "")
	c.Assert(err, gocheck.Equals, router.ErrRouteNotFound)
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
	cmds := []rtesting.RedisCommand{
		{Cmd: "LREM", Args: []interface{}{"frontend:myapp.com", 0, "10.10.10.10"}, Type: rtesting.CmdDo},
	}
	c.Assert(s.fake.Cmds, gocheck.DeepEquals, cmds)
}

func (s *S) TestRoutes(c *gocheck.C) {
	reply := map[string]interface{}{"GET": "tip", "SET": "", "LRANGE": []interface{}{[]byte("http://10.10.10.10:8080")}}
	conn = &rtesting.ResultCommandRedisConn{Reply: reply, FakeRedisConn: s.fake}
	router := hipacheRouter{}
	err := router.AddRoute("tip", "http://10.10.10.10:8080")
	c.Assert(err, gocheck.IsNil)
	routes, err := router.Routes("tip")
	c.Assert(err, gocheck.IsNil)
	c.Assert(routes, gocheck.DeepEquals, []string{"http://10.10.10.10:8080"})
}

func (s *S) TestSwap(c *gocheck.C) {
	reply := map[string]interface{}{
		"LRANGE": []interface{}{[]byte("http://127.0.0.1")},
	}
	conn = &rtesting.ResultCommandRedisConn{Reply: reply, FakeRedisConn: s.fake}
	backend1 := "b1"
	backend2 := "b2"
	router := hipacheRouter{}
	router.AddBackend(backend1)
	router.AddRoute(backend1, "http://127.0.0.1")
	router.AddBackend(backend2)
	router.AddRoute(backend2, "http://10.10.10.10")
	err := router.Swap(backend1, backend2)
	c.Assert(err, gocheck.IsNil)
	cmds := []rtesting.RedisCommand{
		{Cmd: "RPUSH", Args: []interface{}{"frontend:b1.golang.org", "b1"}, Type: rtesting.CmdDo},
		{Cmd: "RPUSH", Args: []interface{}{"frontend:b1.golang.org", "http://127.0.0.1"}, Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:b1"}, Type: rtesting.CmdDo},
		{Cmd: "RPUSH", Args: []interface{}{"frontend:b2.golang.org", "b2"}, Type: rtesting.CmdDo},
		{Cmd: "RPUSH", Args: []interface{}{"frontend:b2.golang.org", "http://10.10.10.10"}, Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:b2"}, Type: rtesting.CmdDo},
		{Cmd: "LRANGE", Args: []interface{}{"frontend:b1.golang.org", 0, -1}, Type: rtesting.CmdDo},
		{Cmd: "LRANGE", Args: []interface{}{"frontend:b2.golang.org", 0, -1}, Type: rtesting.CmdDo},
		{Cmd: "RPUSH", Args: []interface{}{"frontend:b2.golang.org", "http://127.0.0.1"}, Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:b2"}, Type: rtesting.CmdDo},
		{Cmd: "LREM", Args: []interface{}{"frontend:b1.golang.org", 0, "http://127.0.0.1"}, Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:b1"}, Type: rtesting.CmdDo},
		{Cmd: "RPUSH", Args: []interface{}{"frontend:b1.golang.org", "http://127.0.0.1"}, Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:b1"}, Type: rtesting.CmdDo},
		{Cmd: "LREM", Args: []interface{}{"frontend:b2.golang.org", 0, "http://127.0.0.1"}, Type: rtesting.CmdDo},
		{Cmd: "GET", Args: []interface{}{"cname:b2"}, Type: rtesting.CmdDo},
	}
	c.Assert(s.fake.Cmds, gocheck.DeepEquals, cmds)
}
