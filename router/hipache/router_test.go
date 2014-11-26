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
	ttesting "github.com/tsuru/tsuru/testing"
	rtesting "github.com/tsuru/tsuru/testing/redis"
	"launchpad.net/gocheck"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type S struct {
	fake *rtesting.FakeRedisConn
	pool *redis.Pool
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
	ttesting.ClearAllCollections(s.conn.Collection("router_hipache_tests").Database)
}

func (s *S) SetUpTest(c *gocheck.C) {
	srv, err := config.GetString("hipache:redis-server")
	if err != nil {
		srv = "localhost:6379"
	}
	pool = redis.NewPool(func() (redis.Conn, error) {
		return redis.Dial("tcp", srv)
	}, 10)
	conn = connect()
	rtesting.ClearRedisKeys("frontend*", c)
	rtesting.ClearRedisKeys("cname*", c)
	rtesting.ClearRedisKeys("*.com", c)
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
	defer got.Close()
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
	router := hipacheRouter{}
	err := router.AddBackend("tip")
	c.Assert(err, gocheck.IsNil)
	backends, err := redis.Int(conn.Do("LLEN", "frontend:tip.golang.org"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(1, gocheck.Equals, backends)
}

func (s *S) TestRemoveBackend(c *gocheck.C) {
	router := hipacheRouter{}
	err := router.RemoveBackend("tip")
	c.Assert(err, gocheck.IsNil)
	backends, err := redis.Int(conn.Do("LLEN", "frontend:tip.golang.org"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(0, gocheck.Equals, backends)
}

func (s *S) TestRemoveBackendAlsoRemovesRelatedCNameBackendAndControlRecord(c *gocheck.C) {
	router := hipacheRouter{}
	err := router.AddBackend("tip")
	c.Assert(err, gocheck.IsNil)
	err = router.SetCName("mycname.com", "tip")
	c.Assert(err, gocheck.IsNil)
	cnames, err := redis.Int(conn.Do("LLEN", "cname:tip"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(1, gocheck.Equals, cnames)
	err = router.RemoveBackend("tip")
	c.Assert(err, gocheck.IsNil)
	cnames, err = redis.Int(conn.Do("LLEN", "cname:tip"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(0, gocheck.Equals, cnames)
}

func (s *S) TestAddRouteWithoutAssemblingFrontend(c *gocheck.C) {
	err := hipacheRouter{}.addRoute("test.com", "10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	routes, err := redis.Strings(conn.Do("LRANGE", "test.com", 0, -1))
	c.Assert(routes, gocheck.DeepEquals, []string{"10.10.10.10"})
}

func (s *S) TestAddRoute(c *gocheck.C) {
	router := hipacheRouter{}
	err := router.AddRoute("tip", "http://10.10.10.10:8080")
	c.Assert(err, gocheck.IsNil)
	routes, err := redis.Int(conn.Do("LLEN", "frontend:tip.golang.org"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(1, gocheck.Equals, routes)
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
	pool = redis.NewPool(fakeConnect, 5)
	conn = &rtesting.FailingFakeRedisConn{}
	err := hipacheRouter{}.AddRoute("tip", "http://www.tsuru.io")
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.err.Error(), gocheck.Equals, "Could not add route: I can't do that.")
	c.Assert(e.op, gocheck.Equals, "add")
}

func (s *S) TestAddRouteAlsoUpdatesCNameRecordsWhenExists(c *gocheck.C) {
	router := hipacheRouter{}
	err := router.AddRoute("tip", "http://10.10.10.10:8080")
	c.Assert(err, gocheck.IsNil)
	err = router.SetCName("mycname.com", "tip")
	c.Assert(err, gocheck.IsNil)
	cnameRoutes, err := redis.Int(conn.Do("LLEN", "frontend:mycname.com"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(1, gocheck.Equals, cnameRoutes)
	err = router.AddRoute("tip", "http://10.10.10.11:8080")
	c.Assert(err, gocheck.IsNil)
	cnameRoutes, err = redis.Int(conn.Do("LLEN", "frontend:mycname.com"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(2, gocheck.Equals, cnameRoutes)
}

func (s *S) TestRemoveRoute(c *gocheck.C) {
	router := hipacheRouter{}
	err := router.AddBackend("tip")
	c.Assert(err, gocheck.IsNil)
	err = router.AddRoute("tip", "10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	err = router.RemoveRoute("tip", "10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	err = router.RemoveBackend("tip")
	c.Assert(err, gocheck.IsNil)
	routes, err := redis.Int(conn.Do("LLEN", "frontend:tip.golang.org"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(0, gocheck.Equals, routes)
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
	pool = redis.NewPool(fakeConnect, 5)
	conn = &rtesting.FailingFakeRedisConn{}
	err := hipacheRouter{}.RemoveRoute("tip", "tip.golang.org")
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.err.Error(), gocheck.Equals, "I can't do that.")
	c.Assert(e.op, gocheck.Equals, "remove")
}

func (s *S) TestRemoveRouteAlsoRemovesRespectiveCNameRecord(c *gocheck.C) {
	router := hipacheRouter{}
	err := router.AddBackend("tip")
	c.Assert(err, gocheck.IsNil)
	err = router.AddRoute("tip", "10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	err = router.SetCName("test.com", "tip")
	c.Assert(err, gocheck.IsNil)
	err = router.RemoveRoute("tip", "tip.golang.org")
	c.Assert(err, gocheck.IsNil)
	cnames, err := redis.Int(conn.Do("LLEN", "cname:test.com"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(cnames, gocheck.Equals, 0)
}

func (s *S) TestGetCNames(c *gocheck.C) {
	router := hipacheRouter{}
	err := router.AddBackend("myapp")
	c.Assert(err, gocheck.IsNil)
	err = router.SetCName("coolcname.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	cnames, err := router.getCNames("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(cnames, gocheck.DeepEquals, []string{"coolcname.com"})
}

func (s *S) TestGetCNameIgnoresErrNil(c *gocheck.C) {
	cnames, err := hipacheRouter{}.getCNames("myapp")
	c.Assert(err, gocheck.IsNil)
	c.Assert(cnames, gocheck.DeepEquals, []string{})
}

func (s *S) TestSetCName(c *gocheck.C) {
	router := hipacheRouter{}
	err := router.AddBackend("myapp")
	c.Assert(err, gocheck.IsNil)
	err = router.SetCName("myapp.com", "myapp")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestSetCNameWithPreviousRoutes(c *gocheck.C) {
	router := hipacheRouter{}
	err := router.AddBackend("myapp")
	c.Assert(err, gocheck.IsNil)
	err = router.AddRoute("myapp", "10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	err = router.AddRoute("myapp", "10.10.10.11")
	c.Assert(err, gocheck.IsNil)
	err = router.SetCName("mycname.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	cnameRoutes, err := redis.Strings(conn.Do("LRANGE", "frontend:mycname.com", 0, -1))
	c.Assert(err, gocheck.IsNil)
	c.Assert([]string{"myapp", "10.10.10.10", "10.10.10.11"}, gocheck.DeepEquals, cnameRoutes)
}

func (s *S) TestSetCNameShouldRecordAppAndCNameOnRedis(c *gocheck.C) {
	router := hipacheRouter{}
	err := router.AddBackend("myapp")
	c.Assert(err, gocheck.IsNil)
	err = router.SetCName("mycname.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	cname, err := redis.Strings(conn.Do("LRANGE", "cname:myapp", 0, -1))
	c.Assert(err, gocheck.IsNil)
	c.Assert([]string{"mycname.com"}, gocheck.DeepEquals, cname)
}

func (s *S) TestSetCNameSetsMultipleCNames(c *gocheck.C) {
	router := hipacheRouter{}
	err := router.AddBackend("myapp")
	c.Assert(err, gocheck.IsNil)
	err = router.AddRoute("myapp", "10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	err = router.SetCName("mycname.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	err = router.SetCName("myothercname.com", "myapp")
	cname, err := redis.Strings(conn.Do("LRANGE", "frontend:mycname.com", 0, -1))
	c.Assert(err, gocheck.IsNil)
	c.Assert([]string{"myapp", "10.10.10.10"}, gocheck.DeepEquals, cname)
	cname, err = redis.Strings(conn.Do("LRANGE", "frontend:myothercname.com", 0, -1))
	c.Assert(err, gocheck.IsNil)
	c.Assert([]string{"myapp", "10.10.10.10"}, gocheck.DeepEquals, cname)
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
	router := hipacheRouter{}
	err := router.SetCName("myapp.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	cnames, err := redis.Int(conn.Do("LLEN", "cname:myapp"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(1, gocheck.Equals, cnames)
	err = router.UnsetCName("myapp.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	cnames, err = redis.Int(conn.Do("LLEN", "cname:myapp"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(0, gocheck.Equals, cnames)
}

func (s *S) TestUnsetTwoCNames(c *gocheck.C) {
	router := hipacheRouter{}
	err := router.SetCName("myapp.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	cnames, err := redis.Int(conn.Do("LLEN", "cname:myapp"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(1, gocheck.Equals, cnames)
	err = router.SetCName("myapptwo.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	cnames, err = redis.Int(conn.Do("LLEN", "cname:myapp"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(2, gocheck.Equals, cnames)
	err = router.UnsetCName("myapp.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	cnames, err = redis.Int(conn.Do("LLEN", "cname:myapp"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(1, gocheck.Equals, cnames)
	err = router.UnsetCName("myapptwo.com", "myapp")
	c.Assert(err, gocheck.IsNil)
	cnames, err = redis.Int(conn.Do("LLEN", "cname:myapp"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(0, gocheck.Equals, cnames)
}

func (s *S) TestAddr(c *gocheck.C) {
	router := hipacheRouter{}
	err := router.AddBackend("tip")
	c.Assert(err, gocheck.IsNil)
	err = router.AddRoute("tip", "10.10.10.10")
	c.Assert(err, gocheck.IsNil)
	addr, err := router.Addr("tip")
	c.Assert(err, gocheck.IsNil)
	c.Assert(addr, gocheck.Equals, "tip.golang.org")
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
	pool = redis.NewPool(fakeConnect, 5)
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
	backend1 := "b1"
	backend2 := "b2"
	router := hipacheRouter{}
	router.AddBackend(backend1)
	router.AddRoute(backend1, "http://127.0.0.1")
	router.AddBackend(backend2)
	router.AddRoute(backend2, "http://10.10.10.10")
	err := router.Swap(backend1, backend2)
	c.Assert(err, gocheck.IsNil)
	backend1Routes, err := redis.Strings(conn.Do("LRANGE", "frontend:b2.golang.org", 0, -1))
	c.Assert(err, gocheck.IsNil)
	c.Assert([]string{"b1", "http://127.0.0.1"}, gocheck.DeepEquals, backend1Routes)
	backend2Routes, err := redis.Strings(conn.Do("LRANGE", "frontend:b1.golang.org", 0, -1))
	c.Assert(err, gocheck.IsNil)
	c.Assert([]string{"b2", "http://10.10.10.10"}, gocheck.DeepEquals, backend2Routes)
}
