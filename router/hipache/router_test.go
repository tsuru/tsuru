// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hipache

import (
	"errors"
	"sync"
	"testing"

	"github.com/garyburd/redigo/redis"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/router"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type S struct {
	fake *FakeRedisConn
	conn *db.Storage
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("hipache:domain", "golang.org")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "router_hipache_tests")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	dbtest.ClearAllCollections(s.conn.Collection("router_hipache_tests").Database)
}

func (s *S) SetUpTest(c *check.C) {
	srv, err := config.GetString("hipache:redis-server")
	if err != nil {
		srv = "localhost:6379"
	}
	rtest := hipacheRouter{prefix: "hipache", pool: redis.NewPool(func() (redis.Conn, error) {
		return redis.Dial("tcp", srv)
	}, 10)}
	conn = rtest.connect()
	ClearRedisKeys("frontend*", c)
	ClearRedisKeys("cname*", c)
	ClearRedisKeys("*.com", c)
}

func (s *S) TestStressRace(c *check.C) {
	rtest := hipacheRouter{prefix: "hipache"}
	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn := rtest.connect()
			_, err := conn.Do("PING")
			c.Assert(err, check.IsNil)
		}()
	}
	wg.Wait()
}

func (s *S) TestConnect(c *check.C) {
	rtest := hipacheRouter{prefix: "hipache"}
	got := rtest.connect()
	defer got.Close()
	c.Assert(got, check.NotNil)
	_, err := got.Do("PING")
	c.Assert(err, check.IsNil)
}

func (s *S) TestConnectWithPassword(c *check.C) {
	config.Set("hipache:redis-password", "123456")
	defer config.Unset("hipache:redis-password")
	rtest := hipacheRouter{prefix: "hipache"}
	got := rtest.connect()
	defer got.Close()
	c.Assert(got, check.NotNil)
	_, err := got.Do("PING")
	c.Assert(err, check.ErrorMatches, "ERR Client sent AUTH, but no password is set")
}

func (s *S) TestConnectWhenPoolIsNil(c *check.C) {
	rtest := hipacheRouter{prefix: "hipache"}
	got := rtest.connect()
	defer got.Close()
	_, err := got.Do("PING")
	c.Assert(err, check.IsNil)
	got.Close()
	c.Assert(rtest.pool, check.NotNil)
}

func (s *S) TestConnectWhenConnIsNilAndCannotConnect(c *check.C) {
	config.Set("hipache:redis-server", "127.0.0.1:6380")
	defer config.Unset("hipache:redis-server")
	rtest := hipacheRouter{prefix: "hipache"}
	got := rtest.connect()
	_, err := got.Do("PING")
	c.Assert(err, check.NotNil)
	got.Close()
}

func (s *S) TestShouldBeRegistered(c *check.C) {
	r, err := router.Get("hipache")
	c.Assert(err, check.IsNil)
	_, ok := r.(*hipacheRouter)
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestShouldBeRegisteredAllowingPrefixes(c *check.C) {
	config.Set("routers:inst1:type", "hipache")
	config.Set("routers:inst2:type", "hipache")
	defer config.Unset("routers:inst1:type")
	defer config.Unset("routers:inst2:type")
	got1, err := router.Get("inst1")
	c.Assert(err, check.IsNil)
	got2, err := router.Get("inst2")
	c.Assert(err, check.IsNil)
	r1, ok := got1.(*hipacheRouter)
	c.Assert(ok, check.Equals, true)
	c.Assert(r1.prefix, check.Equals, "routers:inst1")
	r2, ok := got2.(*hipacheRouter)
	c.Assert(ok, check.Equals, true)
	c.Assert(r2.prefix, check.Equals, "routers:inst2")
}

func (s *S) TestAddBackend(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	err := router.AddBackend("tip")
	c.Assert(err, check.IsNil)
	backends, err := redis.Int(conn.Do("LLEN", "frontend:tip.golang.org"))
	c.Assert(err, check.IsNil)
	c.Assert(1, check.Equals, backends)
}

func (s *S) TestRemoveBackend(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	err := router.RemoveBackend("tip")
	c.Assert(err, check.IsNil)
	backends, err := redis.Int(conn.Do("LLEN", "frontend:tip.golang.org"))
	c.Assert(err, check.IsNil)
	c.Assert(0, check.Equals, backends)
}

func (s *S) TestRemoveBackendAlsoRemovesRelatedCNameBackendAndControlRecord(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	err := router.AddBackend("tip")
	c.Assert(err, check.IsNil)
	err = router.SetCName("mycname.com", "tip")
	c.Assert(err, check.IsNil)
	cnames, err := redis.Int(conn.Do("LLEN", "cname:tip"))
	c.Assert(err, check.IsNil)
	c.Assert(1, check.Equals, cnames)
	err = router.RemoveBackend("tip")
	c.Assert(err, check.IsNil)
	cnames, err = redis.Int(conn.Do("LLEN", "cname:tip"))
	c.Assert(err, check.IsNil)
	c.Assert(0, check.Equals, cnames)
}

func (s *S) TestAddRouteWithoutAssemblingFrontend(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	err := router.addRoute("test.com", "10.10.10.10")
	c.Assert(err, check.IsNil)
	routes, err := redis.Strings(conn.Do("LRANGE", "test.com", 0, -1))
	c.Assert(routes, check.DeepEquals, []string{"10.10.10.10"})
}

func (s *S) TestAddRoute(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	err := router.AddRoute("tip", "http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	routes, err := redis.Int(conn.Do("LLEN", "frontend:tip.golang.org"))
	c.Assert(err, check.IsNil)
	c.Assert(1, check.Equals, routes)
}

func (s *S) TestAddRouteNoDomainConfigured(c *check.C) {
	old, _ := config.Get("hipache:domain")
	defer config.Set("hipache:domain", old)
	config.Unset("hipache:domain")
	router := hipacheRouter{prefix: "hipache"}
	err := router.AddRoute("tip", "http://10.10.10.10:8080")
	c.Assert(err, check.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.op, check.Equals, "add")
}

func (s *S) TestAddRouteConnectFailure(c *check.C) {
	config.Set("hipache:redis-server", "127.0.0.1:6380")
	defer config.Unset("hipache:redis-server")
	router := hipacheRouter{prefix: "hipache"}
	err := router.AddRoute("tip", "http://www.tsuru.io")
	c.Assert(err, check.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.op, check.Equals, "add")
}

func (s *S) TestAddRouteCommandFailure(c *check.C) {
	conn = &FailingFakeRedisConn{}
	router := hipacheRouter{prefix: "hipache", pool: redis.NewPool(fakeConnect, 5)}
	err := router.AddRoute("tip", "http://www.tsuru.io")
	c.Assert(err, check.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.err.Error(), check.Equals, "Could not add route: I can't do that.")
	c.Assert(e.op, check.Equals, "add")
}

func (s *S) TestAddRouteAlsoUpdatesCNameRecordsWhenExists(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	err := router.AddRoute("tip", "http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = router.SetCName("mycname.com", "tip")
	c.Assert(err, check.IsNil)
	cnameRoutes, err := redis.Int(conn.Do("LLEN", "frontend:mycname.com"))
	c.Assert(err, check.IsNil)
	c.Assert(1, check.Equals, cnameRoutes)
	err = router.AddRoute("tip", "http://10.10.10.11:8080")
	c.Assert(err, check.IsNil)
	cnameRoutes, err = redis.Int(conn.Do("LLEN", "frontend:mycname.com"))
	c.Assert(err, check.IsNil)
	c.Assert(2, check.Equals, cnameRoutes)
}

func (s *S) TestRemoveRoute(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	err := router.AddBackend("tip")
	c.Assert(err, check.IsNil)
	err = router.AddRoute("tip", "10.10.10.10")
	c.Assert(err, check.IsNil)
	err = router.RemoveRoute("tip", "10.10.10.10")
	c.Assert(err, check.IsNil)
	err = router.RemoveBackend("tip")
	c.Assert(err, check.IsNil)
	routes, err := redis.Int(conn.Do("LLEN", "frontend:tip.golang.org"))
	c.Assert(err, check.IsNil)
	c.Assert(0, check.Equals, routes)
}

func (s *S) TestRemoveRouteNoDomainConfigured(c *check.C) {
	old, _ := config.Get("hipache:domain")
	defer config.Set("hipache:domain", old)
	config.Unset("hipache:domain")
	router := hipacheRouter{prefix: "hipache"}
	err := router.RemoveRoute("tip", "tip.golang.org")
	c.Assert(err, check.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.op, check.Equals, "remove")
}

func (s *S) TestRemoveRouteConnectFailure(c *check.C) {
	config.Set("hipache:redis-server", "127.0.0.1:6380")
	defer config.Unset("hipache:redis-server")
	router := hipacheRouter{prefix: "hipache"}
	err := router.RemoveRoute("tip", "tip.golang.org")
	c.Assert(err, check.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.op, check.Equals, "remove")
}

func (s *S) TestRemoveRouteCommandFailure(c *check.C) {
	conn = &FailingFakeRedisConn{}
	router := hipacheRouter{prefix: "hipache", pool: redis.NewPool(fakeConnect, 5)}
	err := router.RemoveRoute("tip", "tip.golang.org")
	c.Assert(err, check.NotNil)
	e, ok := err.(*routeError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.err.Error(), check.Equals, "I can't do that.")
	c.Assert(e.op, check.Equals, "remove")
}

func (s *S) TestRemoveRouteAlsoRemovesRespectiveCNameRecord(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	err := router.AddBackend("tip")
	c.Assert(err, check.IsNil)
	err = router.AddRoute("tip", "10.10.10.10")
	c.Assert(err, check.IsNil)
	err = router.SetCName("test.com", "tip")
	c.Assert(err, check.IsNil)
	err = router.RemoveRoute("tip", "tip.golang.org")
	c.Assert(err, check.IsNil)
	cnames, err := redis.Int(conn.Do("LLEN", "cname:test.com"))
	c.Assert(err, check.IsNil)
	c.Assert(cnames, check.Equals, 0)
}

func (s *S) TestHealthCheck(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	c.Assert(router.HealthCheck(), check.IsNil)
}

func (s *S) TestHealthCheckFailure(c *check.C) {
	config.Set("super-hipache:redis-server", "localhost:6739")
	defer config.Unset("super-hipache:redis-server")
	router := hipacheRouter{prefix: "super-hipache"}
	err := router.HealthCheck()
	c.Assert(err, check.NotNil)
}

func (s *S) TestGetCNames(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	err := router.AddBackend("myapp")
	c.Assert(err, check.IsNil)
	err = router.SetCName("coolcname.com", "myapp")
	c.Assert(err, check.IsNil)
	cnames, err := router.getCNames("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(cnames, check.DeepEquals, []string{"coolcname.com"})
}

func (s *S) TestGetCNameIgnoresErrNil(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	cnames, err := router.getCNames("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(cnames, check.DeepEquals, []string{})
}

func (s *S) TestSetCName(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	err := router.AddBackend("myapp")
	c.Assert(err, check.IsNil)
	err = router.SetCName("myapp.com", "myapp")
	c.Assert(err, check.IsNil)
}

func (s *S) TestSetCNameWithPreviousRoutes(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	err := router.AddBackend("myapp")
	c.Assert(err, check.IsNil)
	err = router.AddRoute("myapp", "10.10.10.10")
	c.Assert(err, check.IsNil)
	err = router.AddRoute("myapp", "10.10.10.11")
	c.Assert(err, check.IsNil)
	err = router.SetCName("mycname.com", "myapp")
	c.Assert(err, check.IsNil)
	cnameRoutes, err := redis.Strings(conn.Do("LRANGE", "frontend:mycname.com", 0, -1))
	c.Assert(err, check.IsNil)
	c.Assert([]string{"myapp", "10.10.10.10", "10.10.10.11"}, check.DeepEquals, cnameRoutes)
}

func (s *S) TestSetCNameShouldRecordAppAndCNameOnRedis(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	err := router.AddBackend("myapp")
	c.Assert(err, check.IsNil)
	err = router.SetCName("mycname.com", "myapp")
	c.Assert(err, check.IsNil)
	cname, err := redis.Strings(conn.Do("LRANGE", "cname:myapp", 0, -1))
	c.Assert(err, check.IsNil)
	c.Assert([]string{"mycname.com"}, check.DeepEquals, cname)
}

func (s *S) TestSetCNameSetsMultipleCNames(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	err := router.AddBackend("myapp")
	c.Assert(err, check.IsNil)
	err = router.AddRoute("myapp", "10.10.10.10")
	c.Assert(err, check.IsNil)
	err = router.SetCName("mycname.com", "myapp")
	c.Assert(err, check.IsNil)
	err = router.SetCName("myothercname.com", "myapp")
	cname, err := redis.Strings(conn.Do("LRANGE", "frontend:mycname.com", 0, -1))
	c.Assert(err, check.IsNil)
	c.Assert([]string{"myapp", "10.10.10.10"}, check.DeepEquals, cname)
	cname, err = redis.Strings(conn.Do("LRANGE", "frontend:myothercname.com", 0, -1))
	c.Assert(err, check.IsNil)
	c.Assert([]string{"myapp", "10.10.10.10"}, check.DeepEquals, cname)
}

func (s *S) TestSetCNameValidatesCNameAccordingToDomainConfig(c *check.C) {
	reply := map[string]interface{}{"GET": "", "SET": "", "LRANGE": []interface{}{[]byte{}}, "RPUSH": []interface{}{[]byte{}}}
	conn = &ResultCommandRedisConn{Reply: reply, FakeRedisConn: s.fake}
	router := hipacheRouter{prefix: "hipache"}
	err := router.SetCName("mycname.golang.org", "myapp")
	c.Assert(err, check.NotNil)
	expected := "Could not setCName route: Invalid CNAME mycname.golang.org. You can't use tsuru's application domain."
	c.Assert(err.Error(), check.Equals, expected)
}

func (s *S) TestSetCNameDoesNotBlockSuffixDomain(c *check.C) {
	reply := map[string]interface{}{"GET": "", "SET": "", "LRANGE": []interface{}{[]byte{}}, "RPUSH": []interface{}{[]byte{}}}
	conn = &ResultCommandRedisConn{Reply: reply, FakeRedisConn: s.fake}
	router := hipacheRouter{prefix: "hipache"}
	err := router.SetCName("mycname.golang.org.br", "myapp")
	c.Assert(err, check.IsNil)
}

func (s *S) TestUnsetCName(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	err := router.SetCName("myapp.com", "myapp")
	c.Assert(err, check.IsNil)
	cnames, err := redis.Int(conn.Do("LLEN", "cname:myapp"))
	c.Assert(err, check.IsNil)
	c.Assert(1, check.Equals, cnames)
	err = router.UnsetCName("myapp.com", "myapp")
	c.Assert(err, check.IsNil)
	cnames, err = redis.Int(conn.Do("LLEN", "cname:myapp"))
	c.Assert(err, check.IsNil)
	c.Assert(0, check.Equals, cnames)
}

func (s *S) TestUnsetTwoCNames(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	err := router.SetCName("myapp.com", "myapp")
	c.Assert(err, check.IsNil)
	cnames, err := redis.Int(conn.Do("LLEN", "cname:myapp"))
	c.Assert(err, check.IsNil)
	c.Assert(1, check.Equals, cnames)
	err = router.SetCName("myapptwo.com", "myapp")
	c.Assert(err, check.IsNil)
	cnames, err = redis.Int(conn.Do("LLEN", "cname:myapp"))
	c.Assert(err, check.IsNil)
	c.Assert(2, check.Equals, cnames)
	err = router.UnsetCName("myapp.com", "myapp")
	c.Assert(err, check.IsNil)
	cnames, err = redis.Int(conn.Do("LLEN", "cname:myapp"))
	c.Assert(err, check.IsNil)
	c.Assert(1, check.Equals, cnames)
	err = router.UnsetCName("myapptwo.com", "myapp")
	c.Assert(err, check.IsNil)
	cnames, err = redis.Int(conn.Do("LLEN", "cname:myapp"))
	c.Assert(err, check.IsNil)
	c.Assert(0, check.Equals, cnames)
}

func (s *S) TestAddr(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	err := router.AddBackend("tip")
	c.Assert(err, check.IsNil)
	err = router.AddRoute("tip", "10.10.10.10")
	c.Assert(err, check.IsNil)
	addr, err := router.Addr("tip")
	c.Assert(err, check.IsNil)
	c.Assert(addr, check.Equals, "tip.golang.org")
}

func (s *S) TestAddrNoDomainConfigured(c *check.C) {
	old, _ := config.Get("hipache:domain")
	defer config.Set("hipache:domain", old)
	config.Unset("hipache:domain")
	router := hipacheRouter{prefix: "hipache"}
	addr, err := router.Addr("tip")
	c.Assert(addr, check.Equals, "")
	e, ok := err.(*routeError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.op, check.Equals, "get")
}

func (s *S) TestAddrConnectFailure(c *check.C) {
	config.Set("hipache:redis-server", "127.0.0.1:6380")
	defer config.Unset("hipache:redis-server")
	router := hipacheRouter{prefix: "hipache"}
	addr, err := router.Addr("tip")
	c.Assert(addr, check.Equals, "")
	e, ok := err.(*routeError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.op, check.Equals, "get")
}

func (s *S) TestAddrCommandFailure(c *check.C) {
	conn = &FailingFakeRedisConn{}
	router := hipacheRouter{prefix: "hipache", pool: redis.NewPool(fakeConnect, 5)}
	addr, err := router.Addr("tip")
	c.Assert(addr, check.Equals, "")
	e, ok := err.(*routeError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.op, check.Equals, "get")
	c.Assert(e.err.Error(), check.Equals, "I can't do that.")
}

func (s *S) TestAddrRouteNotFound(c *check.C) {
	conn = &ResultCommandRedisConn{
		DefaultReply:  []interface{}{},
		FakeRedisConn: s.fake,
	}
	r := hipacheRouter{prefix: "hipache"}
	addr, err := r.Addr("tip")
	c.Assert(addr, check.Equals, "")
	c.Assert(err, check.Equals, router.ErrRouteNotFound)
}

func (s *S) TestRouteError(c *check.C) {
	err := &routeError{"add", errors.New("Fatal error.")}
	c.Assert(err.Error(), check.Equals, "Could not add route: Fatal error.")
	err = &routeError{"del", errors.New("Fatal error.")}
	c.Assert(err.Error(), check.Equals, "Could not del route: Fatal error.")
}

func (s *S) TestRemoveElement(c *check.C) {
	router := hipacheRouter{prefix: "hipache"}
	err := router.removeElement("frontend:myapp.com", "10.10.10.10")
	c.Assert(err, check.IsNil)
}

func (s *S) TestRoutes(c *check.C) {
	reply := map[string]interface{}{"GET": "tip", "SET": "", "LRANGE": []interface{}{[]byte("http://10.10.10.10:8080")}}
	conn = &ResultCommandRedisConn{Reply: reply, FakeRedisConn: s.fake}
	router := hipacheRouter{prefix: "hipache"}
	err := router.AddRoute("tip", "http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	routes, err := router.Routes("tip")
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []string{"http://10.10.10.10:8080"})
}

func (s *S) TestSwap(c *check.C) {
	backend1 := "b1"
	backend2 := "b2"
	router := hipacheRouter{prefix: "hipache"}
	router.AddBackend(backend1)
	router.AddRoute(backend1, "http://127.0.0.1")
	router.AddBackend(backend2)
	router.AddRoute(backend2, "http://10.10.10.10")
	err := router.Swap(backend1, backend2)
	c.Assert(err, check.IsNil)
	backend1Routes, err := redis.Strings(conn.Do("LRANGE", "frontend:b2.golang.org", 0, -1))
	c.Assert(err, check.IsNil)
	c.Assert([]string{"b1", "http://127.0.0.1"}, check.DeepEquals, backend1Routes)
	backend2Routes, err := redis.Strings(conn.Do("LRANGE", "frontend:b1.golang.org", 0, -1))
	c.Assert(err, check.IsNil)
	c.Assert([]string{"b2", "http://10.10.10.10"}, check.DeepEquals, backend2Routes)
}
