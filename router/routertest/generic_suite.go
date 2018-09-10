// Copyright 2015 tsuru authors. All rights reserved.
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

type FakeApp struct {
	Name      string
	Pool      string
	Teams     []string
	TeamOwner string
}

func (r FakeApp) GetName() string {
	return r.Name
}

func (r FakeApp) GetPool() string {
	return r.Pool
}

func (r FakeApp) GetTeamOwner() string {
	return r.TeamOwner
}

func (r FakeApp) GetTeamsName() []string {
	return r.Teams
}

const (
	testBackend1 = "backend1"
	testBackend2 = "backend2"
)

type urlHostChecker struct {
	check.CheckerInfo
}

func (c *urlHostChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 2 {
		return false, "expected 2 params"
	}
	vals := make([][]*url.URL, 2)
	for i, p := range params {
		switch v := p.(type) {
		case *url.URL:
			vals[i] = []*url.URL{v}
		case url.URL:
			vals[i] = []*url.URL{&v}
		case []*url.URL:
			vals[i] = v
		}
		for j := range vals[i] {
			vals[i][j] = &url.URL{Host: vals[i][j].Host}
		}
	}
	return check.DeepEquals.Check([]interface{}{vals[0], vals[1]}, names)
}

var HostEquals check.Checker = &urlHostChecker{
	check.CheckerInfo{Name: "HostEquals", Params: []string{"obtained", "expected"}},
}

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
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	addr, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr})
	c.Assert(err, check.IsNil)
	routes, err := s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, HostEquals, []*url.URL{addr})
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRouteAddBackendOptsAndRoute(c *check.C) {
	optsRouter, ok := s.Router.(router.OptsRouter)
	if !ok {
		c.Skip(fmt.Sprintf("%T does not implement OptsRouter", s.Router))
	}
	err := optsRouter.AddBackendOpts(FakeApp{Name: testBackend1}, map[string]string{})
	c.Assert(err, check.IsNil)
	err = optsRouter.AddBackendOpts(FakeApp{Name: testBackend1}, nil)
	c.Assert(err, check.Equals, router.ErrBackendExists)
	addr, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr})
	c.Assert(err, check.IsNil)
	routes, err := s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, HostEquals, []*url.URL{addr})
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRouteRemoveRouteAndBackend(c *check.C) {
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr1})
	c.Assert(err, check.IsNil)
	addr2, err := url.Parse("http://10.10.10.11:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr2})
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveRoutes(testBackend1, []*url.URL{addr1})
	c.Assert(err, check.IsNil)
	routes, err := s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, HostEquals, []*url.URL{addr2})
	err = s.Router.RemoveRoutes(testBackend1, []*url.URL{addr2})
	c.Assert(err, check.IsNil)
	routes, err = s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, HostEquals, []*url.URL{})
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
	routes, _ = s.Router.Routes(testBackend1)
	c.Assert(routes, check.HasLen, 0)
}

func (s *RouterSuite) TestRouteAddDupBackend(c *check.C) {
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	err = s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.Equals, router.ErrBackendExists)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRouteAddRouteInvalidBackend(c *check.C) {
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes("backend1", []*url.URL{addr1})
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
}

type URLList []*url.URL

func (l URLList) Len() int           { return len(l) }
func (l URLList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l URLList) Less(i, j int) bool { return l[i].Host < l[j].Host }

func (s *RouterSuite) TestRouteAddRoutes(c *check.C) {
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
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
	c.Assert(routes, HostEquals, []*url.URL{addr1, addr2})
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRouteAddRoutesIgnoreRepeated(c *check.C) {
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr1})
	c.Assert(err, check.IsNil)
	addr2, err := url.Parse("http://10.10.10.11:8080")
	c.Assert(err, check.IsNil)
	addr3, err := url.Parse("tcp://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr1, addr2, addr3})
	c.Assert(err, check.IsNil)
	routes, err := s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	sort.Sort(URLList(routes))
	c.Assert(routes, HostEquals, []*url.URL{addr1, addr2})
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRouteRemoveRoutes(c *check.C) {
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
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
	c.Assert(routes, HostEquals, []*url.URL{addr1, addr2})
	err = s.Router.RemoveRoutes(testBackend1, []*url.URL{addr1, addr2})
	c.Assert(err, check.IsNil)
	routes, err = s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, HostEquals, []*url.URL{})
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRouteRemoveRoutesIgnoreNonExisting(c *check.C) {
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
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
	c.Assert(routes, HostEquals, []*url.URL{addr1, addr2})
	addr3, err := url.Parse("http://10.10.10.12:8080")
	c.Assert(err, check.IsNil)
	addr1Tcp, err := url.Parse("tcp://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveRoutes(testBackend1, []*url.URL{addr1Tcp, addr3, addr2})
	c.Assert(err, check.IsNil)
	routes, err = s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, HostEquals, []*url.URL{})
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestSwap(c *check.C) {
	addr1, _ := url.Parse("http://127.0.0.1:8080")
	addr2, _ := url.Parse("http://10.10.10.10:8080")
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	backend1OrigAddr, err := s.Router.Addr(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr1})
	c.Assert(err, check.IsNil)
	err = s.Router.AddBackend(FakeApp{Name: testBackend2})
	c.Assert(err, check.IsNil)
	backend2OrigAddr, err := s.Router.Addr(testBackend2)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend2, []*url.URL{addr2})
	c.Assert(err, check.IsNil)
	err = s.Router.Swap(testBackend1, testBackend2, false)
	c.Assert(err, check.IsNil)
	backAddr1, err := s.Router.Addr(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr1, check.Equals, backend2OrigAddr)
	backAddr2, err := s.Router.Addr(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr2, check.Equals, backend1OrigAddr)
	routes, err := s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, HostEquals, []*url.URL{addr1})
	routes, err = s.Router.Routes(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(routes, HostEquals, []*url.URL{addr2})
	addr3, _ := url.Parse("http://127.0.0.2:8080")
	addr4, _ := url.Parse("http://10.10.10.11:8080")
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr3})
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend2, []*url.URL{addr4})
	c.Assert(err, check.IsNil)
	routes, err = s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 2)
	routesStrs := []string{routes[0].Host, routes[1].Host}
	sort.Strings(routesStrs)
	c.Assert(routesStrs, check.DeepEquals, []string{addr1.Host, addr3.Host})
	routes, err = s.Router.Routes(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 2)
	routesStrs = []string{routes[0].Host, routes[1].Host}
	sort.Strings(routesStrs)
	c.Assert(routesStrs, check.DeepEquals, []string{addr2.Host, addr4.Host})
	err = s.Router.Swap(testBackend1, testBackend2, false)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend2)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestSwapWithAddBackend(c *check.C) {
	addr1, _ := url.Parse("http://127.0.0.1:8080")
	addr2, _ := url.Parse("http://10.10.10.10:8080")
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	backend1OrigAddr, err := s.Router.Addr(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr1})
	c.Assert(err, check.IsNil)
	err = s.Router.AddBackend(FakeApp{Name: testBackend2})
	c.Assert(err, check.IsNil)
	backend2OrigAddr, err := s.Router.Addr(testBackend2)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend2, []*url.URL{addr2})
	c.Assert(err, check.IsNil)
	err = s.Router.Swap(testBackend1, testBackend2, false)
	c.Assert(err, check.IsNil)
	err = s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.Equals, router.ErrBackendExists)
	err = s.Router.AddBackend(FakeApp{Name: testBackend2})
	c.Assert(err, check.Equals, router.ErrBackendExists)
	backAddr1, err := s.Router.Addr(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr1, check.Equals, backend2OrigAddr)
	backAddr2, err := s.Router.Addr(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr2, check.Equals, backend1OrigAddr)
	routes, err := s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, HostEquals, []*url.URL{addr1})
	routes, err = s.Router.Routes(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(routes, HostEquals, []*url.URL{addr2})
	addr3, _ := url.Parse("http://127.0.0.2:8080")
	addr4, _ := url.Parse("http://10.10.10.11:8080")
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr3})
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend2, []*url.URL{addr4})
	c.Assert(err, check.IsNil)
	routes, err = s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 2)
	routesStrs := []string{routes[0].Host, routes[1].Host}
	sort.Strings(routesStrs)
	c.Assert(routesStrs, check.DeepEquals, []string{addr1.Host, addr3.Host})
	routes, err = s.Router.Routes(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 2)
	routesStrs = []string{routes[0].Host, routes[1].Host}
	sort.Strings(routesStrs)
	c.Assert(routesStrs, check.DeepEquals, []string{addr2.Host, addr4.Host})
	err = s.Router.Swap(testBackend1, testBackend2, false)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend2)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestSwapTwice(c *check.C) {
	addr1, _ := url.Parse("http://127.0.0.1:8080")
	addr2, _ := url.Parse("http://10.10.10.10:8080")
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	backend1OrigAddr, err := s.Router.Addr(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr1})
	c.Assert(err, check.IsNil)
	err = s.Router.AddBackend(FakeApp{Name: testBackend2})
	c.Assert(err, check.IsNil)
	backend2OrigAddr, err := s.Router.Addr(testBackend2)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend2, []*url.URL{addr2})
	c.Assert(err, check.IsNil)
	isSwapped, _, err := router.IsSwapped(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(isSwapped, check.Equals, false)
	isSwapped, swappedWith, err := router.IsSwapped(testBackend2)
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
	c.Assert(backAddr1, check.Equals, backend2OrigAddr)
	backAddr2, err := s.Router.Addr(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr2, check.Equals, backend1OrigAddr)
	routes, err := s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, HostEquals, []*url.URL{addr1})
	routes, err = s.Router.Routes(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(routes, HostEquals, []*url.URL{addr2})
	err = s.Router.Swap(testBackend1, testBackend2, false)
	c.Assert(err, check.IsNil)
	isSwapped, swappedWith, err = router.IsSwapped(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(isSwapped, check.Equals, false)
	c.Assert(swappedWith, check.Equals, testBackend1)
	backAddr1, err = s.Router.Addr(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr1, check.Equals, backend1OrigAddr)
	backAddr2, err = s.Router.Addr(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr2, check.Equals, backend2OrigAddr)
	routes, err = s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, HostEquals, []*url.URL{addr1})
	routes, err = s.Router.Routes(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(routes, HostEquals, []*url.URL{addr2})
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend2)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestSwapCNameOnly(c *check.C) {
	cnameRouter, ok := s.Router.(router.CNameRouter)
	if !ok {
		c.Skip(fmt.Sprintf("%T does not implement CNameRouter", s.Router))
	}
	addr1, _ := url.Parse("http://127.0.0.1:8080")
	addr2, _ := url.Parse("http://10.10.10.10:8080")
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	backend1OrigAddr, err := s.Router.Addr(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr1})
	c.Assert(err, check.IsNil)
	err = s.Router.AddBackend(FakeApp{Name: testBackend2})
	c.Assert(err, check.IsNil)
	backend2OrigAddr, err := s.Router.Addr(testBackend2)
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend2, []*url.URL{addr2})
	c.Assert(err, check.IsNil)
	err = cnameRouter.SetCName("myapp1.cool.domain", testBackend1)
	c.Assert(err, check.IsNil)
	err = cnameRouter.SetCName("myapp2.cool.domain", testBackend2)
	c.Assert(err, check.IsNil)
	names1, err := cnameRouter.CNames(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(names1, check.DeepEquals, []*url.URL{
		{Host: "myapp1.cool.domain"},
	})
	names2, err := cnameRouter.CNames(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(names2, check.DeepEquals, []*url.URL{
		{Host: "myapp2.cool.domain"},
	})
	err = s.Router.Swap(testBackend1, testBackend2, true)
	c.Assert(err, check.IsNil)
	names1, err = cnameRouter.CNames(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(names1, check.DeepEquals, []*url.URL{
		{Host: "myapp2.cool.domain"},
	})
	names2, err = cnameRouter.CNames(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(names2, check.DeepEquals, []*url.URL{
		{Host: "myapp1.cool.domain"},
	})
	backAddr1, err := s.Router.Addr(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr1, check.Equals, backend1OrigAddr)
	backAddr2, err := s.Router.Addr(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr2, check.Equals, backend2OrigAddr)
	err = s.Router.Swap(testBackend1, testBackend2, true)
	c.Assert(err, check.IsNil)
	names1, err = cnameRouter.CNames(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(names1, check.DeepEquals, []*url.URL{
		{Host: "myapp1.cool.domain"},
	})
	names2, err = cnameRouter.CNames(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(names2, check.DeepEquals, []*url.URL{
		{Host: "myapp2.cool.domain"},
	})
	backAddr1, err = s.Router.Addr(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr1, check.Equals, backend1OrigAddr)
	backAddr2, err = s.Router.Addr(testBackend2)
	c.Assert(err, check.IsNil)
	c.Assert(backAddr2, check.Equals, backend2OrigAddr)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend2)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRouteAddDupCName(c *check.C) {
	cnameRouter, ok := s.Router.(router.CNameRouter)
	if !ok {
		c.Skip(fmt.Sprintf("%T does not implement CNameRouter", s.Router))
	}
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr1})
	c.Assert(err, check.IsNil)
	err = cnameRouter.SetCName("my.host.com", testBackend1)
	c.Assert(err, check.IsNil)
	err = cnameRouter.SetCName("my.host.com", testBackend1)
	c.Assert(err, check.Equals, router.ErrCNameExists)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestCNames(c *check.C) {
	cnameRouter, ok := s.Router.(router.CNameRouter)
	if !ok {
		c.Skip(fmt.Sprintf("%T does not implement CNameRouter", s.Router))
	}
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr1})
	c.Assert(err, check.IsNil)
	err = cnameRouter.SetCName("my.host.com", testBackend1)
	c.Assert(err, check.IsNil)
	err = cnameRouter.SetCName("my.host2.com", testBackend1)
	c.Assert(err, check.IsNil)
	cnames, err := cnameRouter.CNames(testBackend1)
	c.Assert(err, check.IsNil)
	url1 := &url.URL{Host: "my.host.com"}
	c.Assert(err, check.IsNil)
	url2 := &url.URL{Host: "my.host2.com"}
	c.Assert(err, check.IsNil)
	expected := []*url.URL{url1, url2}
	sort.Sort(URLList(cnames))
	c.Assert(cnames, HostEquals, expected)
	err = cnameRouter.UnsetCName("my.host.com", testBackend1)
	c.Assert(err, check.IsNil)
	err = cnameRouter.UnsetCName("my.host.com", testBackend1)
	c.Assert(err, check.Equals, router.ErrCNameNotFound)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestSetUnsetCName(c *check.C) {
	cnameRouter, ok := s.Router.(router.CNameRouter)
	if !ok {
		c.Skip(fmt.Sprintf("%T does not implement CNameRouter", s.Router))
	}
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr1})
	c.Assert(err, check.IsNil)
	err = cnameRouter.SetCName("my.host.com", testBackend1)
	c.Assert(err, check.IsNil)
	err = cnameRouter.UnsetCName("my.host.com", testBackend1)
	c.Assert(err, check.IsNil)
	err = cnameRouter.UnsetCName("my.host.com", testBackend1)
	c.Assert(err, check.Equals, router.ErrCNameNotFound)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestSetCNameInvalidBackend(c *check.C) {
	cnameRouter, ok := s.Router.(router.CNameRouter)
	if !ok {
		c.Skip(fmt.Sprintf("%T does not implement CNameRouter", s.Router))
	}
	err := cnameRouter.SetCName("my.cname", testBackend1)
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
}

func (s *RouterSuite) TestSetCNameSubdomainError(c *check.C) {
	cnameRouter, ok := s.Router.(router.CNameRouter)
	if !ok {
		c.Skip(fmt.Sprintf("%T does not implement CNameRouter", s.Router))
	}
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr1})
	c.Assert(err, check.IsNil)
	err = cnameRouter.SetCName("my.host.com", testBackend1)
	c.Assert(err, check.IsNil)
	addr, err := s.Router.Addr(testBackend1)
	c.Assert(err, check.IsNil)
	err = cnameRouter.SetCName("sub."+addr, testBackend1)
	c.Assert(err, check.Equals, router.ErrCNameNotAllowed)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRemoveBackendWithCName(c *check.C) {
	cnameRouter, ok := s.Router.(router.CNameRouter)
	if !ok {
		c.Skip(fmt.Sprintf("%T does not implement CNameRouter", s.Router))
	}
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr1})
	c.Assert(err, check.IsNil)
	err = cnameRouter.SetCName("my.host.com", testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	err = cnameRouter.SetCName("my.host.com", testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRemoveBackendAfterSwap(c *check.C) {
	addr1, _ := url.Parse("http://127.0.0.1")
	addr2, _ := url.Parse("http://10.10.10.10")
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr1})
	c.Assert(err, check.IsNil)
	err = s.Router.AddBackend(FakeApp{Name: testBackend2})
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend2, []*url.URL{addr2})
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
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr1})
	c.Assert(err, check.IsNil)
	err = s.Router.AddRoutes(testBackend1, []*url.URL{addr2})
	c.Assert(err, check.IsNil)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
	err = s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	routes, err := s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(routes, HostEquals, []*url.URL{})
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRemoveBackendKeepsInRouter(c *check.C) {
	_, err := router.Retrieve(testBackend1)
	c.Assert(err, check.Equals, router.ErrBackendNotFound)
	err = s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	name, err := router.Retrieve(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, testBackend1)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
	name, err = router.Retrieve(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(name, check.Equals, testBackend1)
}

func (s *RouterSuite) TestSetHealthcheck(c *check.C) {
	hcRouter, ok := s.Router.(router.CustomHealthcheckRouter)
	if !ok {
		c.Skip(fmt.Sprintf("%T does not implement CustomHealthcheckRouter", s.Router))
	}
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
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

func (s *RouterSuite) TestGetInfo(c *check.C) {
	infoRouter, ok := s.Router.(router.InfoRouter)
	if !ok {
		c.Skip(fmt.Sprintf("%T does not implement InfoRouter", s.Router))
	}
	msg, err := infoRouter.GetInfo()
	c.Assert(err, check.IsNil)
	c.Assert(msg, check.NotNil)
}

func (s *RouterSuite) TestGetStatus(c *check.C) {
	statusRouter, ok := s.Router.(router.StatusRouter)
	if !ok {
		c.Skip(fmt.Sprintf("%T does not implement CustomHealthcheckRouter", s.Router))
	}
	err := s.Router.AddBackend(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	status, _, err := statusRouter.GetBackendStatus(testBackend1)
	c.Assert(err, check.IsNil)
	c.Assert(status, check.Equals, router.BackendStatusReady)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestRouteAddRoutesAsync(c *check.C) {
	asyncRouter, ok := s.Router.(router.AsyncRouter)
	if !ok {
		c.Skip(fmt.Sprintf("%T does not implement AsyncRouter", s.Router))
	}
	err := asyncRouter.AddBackendAsync(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	addr2, err := url.Parse("http://10.10.10.11:8080")
	c.Assert(err, check.IsNil)
	err = asyncRouter.AddRoutesAsync(testBackend1, []*url.URL{addr1, addr2})
	c.Assert(err, check.IsNil)
	routes, err := s.Router.Routes(testBackend1)
	c.Assert(err, check.IsNil)
	sort.Sort(URLList(routes))
	c.Assert(routes, HostEquals, []*url.URL{addr1, addr2})
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}

func (s *RouterSuite) TestCNamesAsync(c *check.C) {
	cnameRouter, ok := s.Router.(router.CNameRouter)
	if !ok {
		c.Skip(fmt.Sprintf("%T does not implement CNameRouter", s.Router))
	}
	asyncRouter, ok := s.Router.(router.AsyncRouter)
	if !ok {
		c.Skip(fmt.Sprintf("%T does not implement AsyncRouter", s.Router))
	}
	err := asyncRouter.AddBackendAsync(FakeApp{Name: testBackend1})
	c.Assert(err, check.IsNil)
	addr1, err := url.Parse("http://10.10.10.10:8080")
	c.Assert(err, check.IsNil)
	err = asyncRouter.AddRoutesAsync(testBackend1, []*url.URL{addr1})
	c.Assert(err, check.IsNil)
	err = asyncRouter.SetCNameAsync("my.host.com", testBackend1)
	c.Assert(err, check.IsNil)
	err = asyncRouter.SetCNameAsync("my.host2.com", testBackend1)
	c.Assert(err, check.IsNil)
	cnames, err := cnameRouter.CNames(testBackend1)
	c.Assert(err, check.IsNil)
	url1 := &url.URL{Host: "my.host.com"}
	c.Assert(err, check.IsNil)
	url2 := &url.URL{Host: "my.host2.com"}
	c.Assert(err, check.IsNil)
	expected := []*url.URL{url1, url2}
	sort.Sort(URLList(cnames))
	c.Assert(cnames, HostEquals, expected)
	err = cnameRouter.UnsetCName("my.host.com", testBackend1)
	c.Assert(err, check.IsNil)
	err = cnameRouter.UnsetCName("my.host.com", testBackend1)
	c.Assert(err, check.Equals, router.ErrCNameNotFound)
	err = s.Router.RemoveBackend(testBackend1)
	c.Assert(err, check.IsNil)
}
