// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rebuild_test

import (
	"net/url"
	"sort"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/router/routertest"
	"gopkg.in/check.v1"
)

func (s *S) TestRebuildRoutes(c *check.C) {
	a := app.App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = provisiontest.ProvisionerInstance.AddUnits(&a, 3, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.RemoveRoutes(a.Name, []*url.URL{units[2].Address})
	routertest.FakeRouter.AddRoutes(a.Name, []*url.URL{{Scheme: "http", Host: "invalid:1234"}})
	changes, err := rebuild.RebuildRoutes(&a, false)
	c.Assert(err, check.IsNil)
	c.Assert(changes.Added, check.DeepEquals, []string{units[2].Address.String()})
	c.Assert(changes.Removed, check.DeepEquals, []string{"http://invalid:1234"})
	routes, err := routertest.FakeRouter.Routes(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 3)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[0].Address.String()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[1].Address.String()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[2].Address.String()), check.Equals, true)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	addr, err := routertest.FakeRouter.Addr(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.IP, check.Equals, addr)
}

func (s *S) TestRebuildRoutesDRY(c *check.C) {
	a := app.App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = provisiontest.ProvisionerInstance.AddUnits(&a, 3, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.RemoveRoutes(a.Name, []*url.URL{units[2].Address})
	routertest.FakeRouter.AddRoutes(a.Name, []*url.URL{{Scheme: "http", Host: "invalid:1234"}})
	changes, err := rebuild.RebuildRoutes(&a, true)
	c.Assert(err, check.IsNil)
	c.Assert(changes.Added, check.DeepEquals, []string{units[2].Address.String()})
	c.Assert(changes.Removed, check.DeepEquals, []string{"http://invalid:1234"})
	routes, err := routertest.FakeRouter.Routes(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 3)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[0].Address.String()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, "invalid:1234"), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[2].Address.String()), check.Equals, false)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	addr, err := routertest.FakeRouter.Addr(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.IP, check.Equals, addr)
}

func (s *S) TestRebuildRoutesTCPRoutes(c *check.C) {
	a := app.App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = provisiontest.ProvisionerInstance.AddUnits(&a, 3, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	for _, u := range units {
		routertest.FakeRouter.RemoveRoutes(a.Name, []*url.URL{u.Address})
		routertest.FakeRouter.AddRoutes(a.Name, []*url.URL{{Scheme: "tcp", Host: u.Address.Host}})
	}
	changes, err := rebuild.RebuildRoutes(&a, false)
	c.Assert(err, check.IsNil)
	c.Assert(changes.Added, check.IsNil)
	c.Assert(changes.Removed, check.IsNil)
	routes, err := routertest.FakeRouter.Routes(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 3)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[0].Address.Host), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[1].Address.Host), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[2].Address.Host), check.Equals, true)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	addr, err := routertest.FakeRouter.Addr(app.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.IP, check.Equals, addr)
}

type URLList []*url.URL

func (l URLList) Len() int           { return len(l) }
func (l URLList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l URLList) Less(i, j int) bool { return l[i].String() < l[j].String() }

func (s *S) TestRebuildRoutesAfterSwap(c *check.C) {
	a1 := app.App{Name: "my-test-app-1", TeamOwner: s.team.Name}
	err := app.CreateApp(&a1, s.user)
	c.Assert(err, check.IsNil)
	a2 := app.App{Name: "my-test-app-2", TeamOwner: s.team.Name}
	err = app.CreateApp(&a2, s.user)
	c.Assert(err, check.IsNil)
	err = provisiontest.ProvisionerInstance.AddUnits(&a1, 3, "web", nil)
	c.Assert(err, check.IsNil)
	err = provisiontest.ProvisionerInstance.AddUnits(&a2, 2, "web", nil)
	c.Assert(err, check.IsNil)
	units1, err := a1.Units()
	c.Assert(err, check.IsNil)
	units2, err := a2.Units()
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.AddRoutes(a1.Name, []*url.URL{{Scheme: "http", Host: "invalid:1234"}})
	routertest.FakeRouter.RemoveRoutes(a2.Name, []*url.URL{units2[0].Address})
	err = routertest.FakeRouter.Swap(a1.Name, a2.Name, false)
	c.Assert(err, check.IsNil)
	changes1, err := rebuild.RebuildRoutes(&a1, false)
	c.Assert(err, check.IsNil)
	changes2, err := rebuild.RebuildRoutes(&a2, false)
	c.Assert(err, check.IsNil)
	c.Assert(changes1.Added, check.IsNil)
	c.Assert(changes1.Removed, check.DeepEquals, []string{"http://invalid:1234"})
	c.Assert(changes2.Added, check.DeepEquals, []string{units2[0].Address.String()})
	c.Assert(changes2.Removed, check.IsNil)
	routes1, err := routertest.FakeRouter.Routes(a1.Name)
	c.Assert(err, check.IsNil)
	routes2, err := routertest.FakeRouter.Routes(a2.Name)
	c.Assert(err, check.IsNil)
	sort.Sort(URLList(routes1))
	sort.Sort(URLList(routes2))
	c.Assert(routes1, check.DeepEquals, []*url.URL{
		units1[0].Address,
		units1[1].Address,
		units1[2].Address,
	})
	c.Assert(routes2, check.DeepEquals, []*url.URL{
		units2[0].Address,
		units2[1].Address,
	})
	c.Assert(routertest.FakeRouter.HasRoute(a1.Name, units2[0].Address.String()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a1.Name, units2[1].Address.String()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a2.Name, units1[0].Address.String()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a2.Name, units1[1].Address.String()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a2.Name, units1[2].Address.String()), check.Equals, true)
}

func (s *S) TestRebuildRoutesRecreatesBackend(c *check.C) {
	a := app.App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = provisiontest.ProvisionerInstance.AddUnits(&a, 3, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.RemoveBackend(a.Name)
	changes, err := rebuild.RebuildRoutes(&a, false)
	c.Assert(err, check.IsNil)
	sort.Strings(changes.Added)
	c.Assert(changes.Added, check.DeepEquals, []string{
		units[0].Address.String(),
		units[1].Address.String(),
		units[2].Address.String(),
	})
	routes, err := routertest.FakeRouter.Routes(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 3)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[0].Address.String()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[1].Address.String()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[2].Address.String()), check.Equals, true)
}

func (s *S) TestRebuildRoutesBetweenRouters(c *check.C) {
	a := app.App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = provisiontest.ProvisionerInstance.AddUnits(&a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	oldIp := a.IP
	a.Router = "fake-hc"
	_, err = rebuild.RebuildRoutes(&a, false)
	c.Assert(err, check.IsNil)
	c.Assert(a.IP, check.Not(check.Equals), oldIp)
	na, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(na.IP, check.Equals, a.IP)
}

func (s *S) TestRebuildRoutesRecreatesCnames(c *check.C) {
	a := app.App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = provisiontest.ProvisionerInstance.AddUnits(&a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	err = a.AddCName("my.cname.com")
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasCName("my.cname.com"), check.Equals, true)
	err = routertest.FakeRouter.UnsetCName("my.cname.com", a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(routertest.FakeRouter.HasCName("my.cname.com"), check.Equals, false)
	changes, err := rebuild.RebuildRoutes(&a, false)
	c.Assert(err, check.IsNil)
	c.Assert(changes, check.DeepEquals, &rebuild.RebuildRoutesResult{})
	routes, err := routertest.FakeRouter.Routes(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 1)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[0].Address.String()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasCName("my.cname.com"), check.Equals, true)
}
