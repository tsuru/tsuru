// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package routertest

import (
	"fmt"
	"net/url"
	"sort"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/router"
	"gopkg.in/check.v1"
)

const (
	testBackend1 = "backend1"
	testBackend2 = "backend2"
)

type RouterSuite struct {
	Router            router.Router
	SetUpSuiteFunc    func(c *check.C)
	SetUpTestFunc     func(c *check.C)
	TearDownSuiteFunc func(c *check.C)
	TearDownTestFunc  func(c *check.C)
}

func (s *RouterSuite) SetUpSuite(c *check.C) {
	if s.SetUpSuiteFunc != nil {
		s.SetUpSuiteFunc(c)
	}
}

func (s *RouterSuite) SetUpTest(c *check.C) {
	if s.SetUpTestFunc != nil {
		s.SetUpTestFunc(c)
	}
	c.Logf("generic router test for %T", s.Router)
}

func (s *RouterSuite) TearDownSuite(c *check.C) {
	if s.TearDownSuiteFunc != nil {
		s.TearDownSuiteFunc(c)
	}
	if _, err := config.GetString("database:name"); err == nil {
		conn, err := db.Conn()
		c.Assert(err, check.IsNil)
		defer conn.Close()
		conn.Apps().Database.DropDatabase()
	}
}

func (s *RouterSuite) TearDownTest(c *check.C) {
	if s.TearDownTestFunc != nil {
		s.TearDownTestFunc(c)
	}
}

func (s *RouterSuite) TestRouteAddBackendAndRoute(c *check.C) {
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	addr, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend1, addr)
	c.Assert(err, check.IsNil)
	routes, err := s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []*url.URL{addr})
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRouteRemoveRouteAndBackend(c *check.C) {
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend1, addr1)
	addr2, err := url.Parse("http://10.10.10.11:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend1, addr2)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveRoute(testBackend1, addr1)
	c.Assert(err, check.IsNil)
	routes, err := s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []*url.URL{addr2})
	err = s.Router.RemoveRoute(testBackend1, addr2)
	c.Assert(err, check.IsNil)
	routes, err = s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []*url.URL{})
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
	_, err = s.Router.Routes(testBackend1)
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
}

func (s *RouterSuite) TestRouteRemoveUnknownRoute(c *check.C) {
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveRoute(testBackend1, addr1)
	c.Assert(err, check.Equals, router.ErrRouteNotFound)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRouteAddDupBackend(c *check.C) {
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddBackend(testBackend1)
	c.Assert(err, check.Equals, router.ErrBackendExists)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRouteAddDupRoute(c *check.C) {
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend1, addr1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend1, addr1)
	c.Assert(err, check.Equals, router.ErrRouteExists)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRouteAddRouteInvalidBackend(c *check.C) {
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute("backend1", addr1)
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
}

type URLList []*url.URL

func (l URLList) Len() int           { return len(l) }
func (l URLList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l URLList) Less(i, j int) bool { return l[i].String() < l[j].String() }

func (s *RouterSuite) TestRouteAddRoutes(c *check.C) {
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	addr2, err := url.Parse("http://10.10.10.11:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr1, addr2})
	c.Assert(err, check.IsNil)
	routes, err := s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	sort.Sort(URLList(routes))
	c.Assert(routes, check.DeepEquals, []*url.URL{addr1, addr2})
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRouteAddRoutesIgnoreRepeated(c *check.C) {
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend1, addr1)
	c.Assert(err, check.IsNil)
	addr2, err := url.Parse("http://10.10.10.11:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr1, addr2})
	c.Assert(err, check.IsNil)
	routes, err := s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	sort.Sort(URLList(routes))
	c.Assert(routes, check.DeepEquals, []*url.URL{addr1, addr2})
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRouteRemoveRoutes(c *check.C) {
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	addr2, err := url.Parse("http://10.10.10.11:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr1, addr2})
	c.Assert(err, check.IsNil)
	routes, err := s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	sort.Sort(URLList(routes))
	c.Assert(routes, check.DeepEquals, []*url.URL{addr1, addr2})
	err = s.Router.RemoveRoutes(testBackend1, []*url.URL{addr1, addr2})
	c.Assert(err, check.IsNil)
	routes, err = s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []*url.URL{})
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRouteRemoveRoutesIgnoreNonExisting(c *check.C) {
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	addr2, err := url.Parse("http://10.10.10.11:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr1, addr2})
	c.Assert(err, check.IsNil)
	routes, err := s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	sort.Sort(URLList(routes))
	c.Assert(routes, check.DeepEquals, []*url.URL{addr1, addr2})
	addr3, err := url.Parse("http://10.10.10.12:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveRoutes(testBackend1, []*url.URL{addr1, addr3, addr2})
	c.Assert(err, check.IsNil)
	routes, err = s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []*url.URL{})
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestSwap(c *check.C) {
	addr1, _ := url.Parse("http://127.0.0.1")
	addr2, _ := url.Parse("http://10.10.10.10")
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend1, addr1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddBackend(testBackend2)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend2, addr2)
	c.Assert(err, check.IsNil)
	err = s.Router.Swap(testBackend1, testBackend2, false)
	c.Assert(err, check.IsNil)
	backAddr1, err := s.Router.Addr(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr1[:len(testBackend2)], check.Equals, testBackend2)
	backAddr2, err := s.Router.Addr(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr2[:len(testBackend1)], check.Equals, testBackend1)
	routes, err := s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Check(routes, check.DeepEquals, []*url.URL{addr1})
	routes, err = s.Router.Routes(testBackend2)
	c.Assert(err, check.IsNil)
	c.Check(routes, check.DeepEquals, []*url.URL{addr2})
	addr3, _ := url.Parse("http://127.0.0.2")
	addr4, _ := url.Parse("http://10.10.10.11")
	err = s.Router.AddRoute(testBackend1, addr3)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend2, addr4)
	c.Assert(err, check.IsNil)
	routes, err = s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 2)
	routesStrs := []string{routes[0].String(), routes[1].String()}
	sort.Strings(routesStrs)
	c.Check(routesStrs, check.DeepEquals, []string{addr1.String(), addr3.String()})
	routes, err = s.Router.Routes(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 2)
	routesStrs = []string{routes[0].String(), routes[1].String()}
	sort.Strings(routesStrs)
	c.Check(routesStrs, check.DeepEquals, []string{addr2.String(), addr4.String()})
	err = s.Router.Swap(testBackend1, testBackend2, false)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend2)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestSwapTwice(c *check.C) {
	addr1, _ := url.Parse("http://127.0.0.1")
	addr2, _ := url.Parse("http://10.10.10.10")
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend1, addr1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddBackend(testBackend2)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend2, addr2)
	c.Assert(err, check.IsNil)
	isSwapped, swappedWith, err := router.IsSwapped(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(isSwapped, check.Equals, false)
	isSwapped, swappedWith, err = router.IsSwapped(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(isSwapped, check.Equals, false)
	c.Assert(swappedWith, check.Equals, testBackend2)
	err = s.Router.Swap(testBackend1, testBackend2, false)
	c.Assert(err, check.IsNil)
	isSwapped, swappedWith, err = router.IsSwapped(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(isSwapped, check.Equals, true)
	c.Assert(swappedWith, check.Equals, testBackend2)
	isSwapped, swappedWith, err = router.IsSwapped(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(isSwapped, check.Equals, true)
	c.Assert(swappedWith, check.Equals, testBackend1)
	backAddr1, err := s.Router.Addr(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr1[:len(testBackend2)], check.Equals, testBackend2)
	backAddr2, err := s.Router.Addr(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr2[:len(testBackend1)], check.Equals, testBackend1)
	routes, err := s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Check(routes, check.DeepEquals, []*url.URL{addr1})
	routes, err = s.Router.Routes(testBackend2)
	c.Assert(err, check.IsNil)
	c.Check(routes, check.DeepEquals, []*url.URL{addr2})
	err = s.Router.Swap(testBackend1, testBackend2, false)
	c.Assert(err, check.IsNil)
	isSwapped, swappedWith, err = router.IsSwapped(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(isSwapped, check.Equals, false)
	c.Assert(swappedWith, check.Equals, testBackend1)
	backAddr1, err = s.Router.Addr(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr1[:len(testBackend1)], check.Equals, testBackend1)
	backAddr2, err = s.Router.Addr(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr2[:len(testBackend2)], check.Equals, testBackend2)
	routes, err = s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Check(routes, check.DeepEquals, []*url.URL{addr1})
	routes, err = s.Router.Routes(testBackend2)
	c.Assert(err, check.IsNil)
	c.Check(routes, check.DeepEquals, []*url.URL{addr2})
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend2)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRouteAddDupCName(c *check.C) {
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend1, addr1)
	c.Assert(err, check.IsNil)
	err = s.Router.SetCName("my.host.com", testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.SetCName("my.host.com", testBackend1)
	c.Assert(err, check.Equals, router.ErrCNameExists)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestCNames(c *check.C) {
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend1, addr1)
	c.Assert(err, check.IsNil)
	err = s.Router.SetCName("my.host.com", testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.SetCName("my.host2.com", testBackend1)
	c.Assert(err, check.IsNil)
	cnames, err := s.Router.CNames(testBackend1)
	url1, err := url.Parse("my.host.com")
	c.Assert(err, check.IsNil)
	url2, err := url.Parse("my.host2.com")
	c.Assert(err, check.IsNil)
	c.Assert(err, check.IsNil)
	expected := []*url.URL{url1, url2}
	sort.Sort(URLList(cnames))
	c.Assert(cnames, check.DeepEquals, expected)
	err = s.Router.UnsetCName("my.host.com", testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.UnsetCName("my.host.com", testBackend1)
	c.Assert(err, check.Equals, router.ErrCNameNotFound)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestSetUnsetCName(c *check.C) {
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend1, addr1)
	c.Assert(err, check.IsNil)
	err = s.Router.SetCName("my.host.com", testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.UnsetCName("my.host.com", testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.UnsetCName("my.host.com", testBackend1)
	c.Assert(err, check.Equals, router.ErrCNameNotFound)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestSetCNameInvalidBackend(c *check.C) {
	err := s.Router.SetCName("my.cname", testBackend1)
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
}

func (s *RouterSuite) TestSetCNameSubdomainError(c *check.C) {
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend1, addr1)
	c.Assert(err, check.IsNil)
	err = s.Router.SetCName("my.host.com", testBackend1)
	c.Assert(err, check.IsNil)
	addr, err := s.Router.Addr(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.SetCName("sub."+addr, testBackend1)
	c.Assert(err, check.Equals, router.ErrCNameNotAllowed)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRemoveBackendWithCName(c *check.C) {
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend1, addr1)
	c.Assert(err, check.IsNil)
	err = s.Router.SetCName("my.host.com", testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.SetCName("my.host.com", testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRemoveBackendAfterSwap(c *check.C) {
	addr1, _ := url.Parse("http://127.0.0.1")
	addr2, _ := url.Parse("http://10.10.10.10")
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend1, addr1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddBackend(testBackend2)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend2, addr2)
	c.Assert(err, check.IsNil)
	err = s.Router.Swap(testBackend1, testBackend2, false)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.Equals, router.ErrBackendSwapped)
	err = s.Router.Swap(testBackend1, testBackend2, false)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend2)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRemoveBackendWithoutRemoveRoutes(c *check.C) {
	addr1, _ := url.Parse("http://127.0.0.1")
	addr2, _ := url.Parse("http://10.10.10.10")
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend1, addr1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoute(testBackend1, addr2)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	routes, err := s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.DeepEquals, []*url.URL{})
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestSetHealthcheck(c *check.C) {
	hcRouter, ok := s.Router.(router.CustomHealthcheckRouter)
	if !ok {
		c.Skip(fmt.Sprintf("%T does not implement CustomHealthcheckRouter", s.Router))
	}
	err := s.Router.AddBackend(testBackend1)
	c.Assert(err, check.IsNil)
	hcData := router.HealthcheckData{
		Path:   "/",
		Status: 200,
		Body:   "WORKING",
	}
	err = hcRouter.SetHealthcheck(testBackend1, hcData)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}
