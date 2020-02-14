// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rebuild_test

import (
	"net/http"
	"net/url"
	"sort"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	routerTypes "github.com/tsuru/tsuru/types/router"
	check "gopkg.in/check.v1"
)

func newVersion(c *check.C, a appTypes.App) appTypes.AppVersion {
	version, err := servicemanager.AppVersion.NewAppVersion(appTypes.NewVersionArgs{
		App: a,
	})
	c.Assert(err, check.IsNil)
	err = version.CommitBaseImage()
	c.Assert(err, check.IsNil)
	err = version.CommitSuccessful()
	c.Assert(err, check.IsNil)
	return version
}

func (s *S) TestRebuildRoutes(c *check.C) {
	a := app.App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	version := newVersion(c, &a)
	err = provisiontest.ProvisionerInstance.AddUnits(&a, 3, "web", version, nil)
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.RemoveRoutes(a.Name, []*url.URL{units[2].Address})
	routertest.FakeRouter.AddRoutes(a.Name, []*url.URL{{Scheme: "http", Host: "invalid:1234"}})
	changes, err := rebuild.RebuildRoutes(&a, false)
	c.Assert(err, check.IsNil)
	c.Assert(changes, check.DeepEquals, map[string]rebuild.RebuildRoutesResult{
		"fake": {
			PrefixResults: []rebuild.RebuildPrefixResult{
				{
					Added:   []string{units[2].Address.String()},
					Removed: []string{"http://invalid:1234"},
				},
			},
		},
	})
	routes, err := routertest.FakeRouter.Routes(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 3)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[0].Address.String()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[1].Address.String()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[2].Address.String()), check.Equals, true)
}

func (s *S) TestRebuildRoutesDRY(c *check.C) {
	a := app.App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	version := newVersion(c, &a)
	err = provisiontest.ProvisionerInstance.AddUnits(&a, 3, "web", version, nil)
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.RemoveRoutes(a.Name, []*url.URL{units[2].Address})
	routertest.FakeRouter.AddRoutes(a.Name, []*url.URL{{Scheme: "http", Host: "invalid:1234"}})
	changes, err := rebuild.RebuildRoutes(&a, true)
	c.Assert(err, check.IsNil)
	c.Assert(changes, check.DeepEquals, map[string]rebuild.RebuildRoutesResult{
		"fake": {
			PrefixResults: []rebuild.RebuildPrefixResult{
				{
					Added:   []string{units[2].Address.String()},
					Removed: []string{"http://invalid:1234"},
				},
			},
		},
	})
	routes, err := routertest.FakeRouter.Routes(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 3)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[0].Address.String()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, "invalid:1234"), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[2].Address.String()), check.Equals, false)
}

func (s *S) TestRebuildRoutesTCPRoutes(c *check.C) {
	a := app.App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	version := newVersion(c, &a)
	err = provisiontest.ProvisionerInstance.AddUnits(&a, 3, "web", version, nil)
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	for _, u := range units {
		routertest.FakeRouter.RemoveRoutes(a.Name, []*url.URL{u.Address})
		routertest.FakeRouter.AddRoutes(a.Name, []*url.URL{{Scheme: "tcp", Host: u.Address.Host}})
	}
	changes, err := rebuild.RebuildRoutes(&a, false)
	c.Assert(err, check.IsNil)
	c.Assert(changes, check.DeepEquals, map[string]rebuild.RebuildRoutesResult{
		"fake": {
			PrefixResults: []rebuild.RebuildPrefixResult{
				{
					Added:   nil,
					Removed: nil,
				},
			},
		},
	})
	routes, err := routertest.FakeRouter.Routes(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 3)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[0].Address.Host), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[1].Address.Host), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[2].Address.Host), check.Equals, true)
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
	version1 := newVersion(c, &a1)
	version2 := newVersion(c, &a2)
	err = provisiontest.ProvisionerInstance.AddUnits(&a1, 3, "web", version1, nil)
	c.Assert(err, check.IsNil)
	err = provisiontest.ProvisionerInstance.AddUnits(&a2, 2, "web", version2, nil)
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
	c.Assert(changes1, check.DeepEquals, map[string]rebuild.RebuildRoutesResult{
		"fake": {
			PrefixResults: []rebuild.RebuildPrefixResult{
				{
					Added:   nil,
					Removed: []string{"http://invalid:1234"},
				},
			},
		},
	})
	c.Assert(changes2, check.DeepEquals, map[string]rebuild.RebuildRoutesResult{
		"fake": {
			PrefixResults: []rebuild.RebuildPrefixResult{
				{
					Added:   []string{units2[0].Address.String()},
					Removed: nil,
				},
			},
		},
	})
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
	version := newVersion(c, &a)
	err = provisiontest.ProvisionerInstance.AddUnits(&a, 3, "web", version, nil)
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.RemoveBackend(a.Name)
	changes, err := rebuild.RebuildRoutes(&a, false)
	c.Assert(err, check.IsNil)
	c.Assert(changes["fake"].PrefixResults, check.HasLen, 1)
	sort.Strings(changes["fake"].PrefixResults[0].Added)
	c.Assert(changes, check.DeepEquals, map[string]rebuild.RebuildRoutesResult{
		"fake": {
			PrefixResults: []rebuild.RebuildPrefixResult{
				{
					Added: []string{
						units[0].Address.String(),
						units[1].Address.String(),
						units[2].Address.String(),
					},
					Removed: nil,
				},
			},
		},
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
	version := newVersion(c, &a)
	err = provisiontest.ProvisionerInstance.AddUnits(&a, 1, "web", version, nil)
	c.Assert(err, check.IsNil)
	oldAddrs, err := a.GetAddresses()
	c.Assert(err, check.IsNil)
	a.Router = "fake-hc"
	_, err = rebuild.RebuildRoutes(&a, false)
	c.Assert(err, check.IsNil)
	newAddrs, err := a.GetAddresses()
	c.Assert(err, check.IsNil)
	c.Assert(newAddrs, check.Not(check.DeepEquals), oldAddrs)
}

func (s *S) TestRebuildRoutesRecreatesCnames(c *check.C) {
	a := app.App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	version := newVersion(c, &a)
	err = provisiontest.ProvisionerInstance.AddUnits(&a, 1, "web", version, nil)
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
	c.Assert(changes, check.DeepEquals, map[string]rebuild.RebuildRoutesResult{"fake": {PrefixResults: []rebuild.RebuildPrefixResult{{}}}})
	routes, err := routertest.FakeRouter.Routes(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 1)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, units[0].Address.String()), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasCName("my.cname.com"), check.Equals, true)
}

func (s *S) TestRebuildRoutesSetsHealthcheck(c *check.C) {
	a := app.App{Name: "my-test-app", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	version, err := servicemanager.AppVersion.NewAppVersion(appTypes.NewVersionArgs{
		App: &a,
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"healthcheck": map[string]interface{}{
			"path":          "/healthcheck",
			"status":        http.StatusFound,
			"use_in_router": true,
		},
	}
	err = version.AddData(appTypes.AddVersionDataArgs{
		CustomData: customData,
	})
	c.Assert(err, check.IsNil)
	err = version.CommitSuccessful()
	c.Assert(err, check.IsNil)
	err = routertest.FakeRouter.RemoveHealthcheck("my-test-app")
	c.Assert(err, check.IsNil)
	changes, err := rebuild.RebuildRoutes(&a, false)
	c.Assert(err, check.IsNil)
	c.Assert(changes, check.DeepEquals, map[string]rebuild.RebuildRoutesResult{"fake": {PrefixResults: []rebuild.RebuildPrefixResult{{}}}})
	expected := routerTypes.HealthcheckData{
		Path:   "/healthcheck",
		Status: 302,
	}
	c.Assert(routertest.FakeRouter.GetHealthcheck("my-test-app"), check.DeepEquals, expected)
}

func (s *S) TestRebuildRoutesMultiplePrefixes(c *check.C) {
	a := app.App{Name: "my-test-app", TeamOwner: s.team.Name}
	a.Routers = []appTypes.AppRouter{{Name: "fake"}, {Name: "fake-prefix"}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)

	provisiontest.ProvisionerInstance.MockRoutableAddresses(&a, []appTypes.RoutableAddresses{
		{
			Prefix: "",
			Addresses: []*url.URL{
				{Host: "u1", Scheme: "http"},
				{Host: "u2", Scheme: "http"},
				{Host: "u3", Scheme: "http"},
			},
		},
		{
			Prefix: "web",
			Addresses: []*url.URL{
				{Host: "u2", Scheme: "http"},
				{Host: "u4", Scheme: "http"},
			},
		},
	})

	changes, err := rebuild.RebuildRoutes(&a, false)
	c.Assert(err, check.IsNil)
	c.Assert(changes, check.DeepEquals, map[string]rebuild.RebuildRoutesResult{
		"fake": {
			PrefixResults: []rebuild.RebuildPrefixResult{
				{
					Added:   []string{"http://u1", "http://u2", "http://u3"},
					Removed: nil,
				},
			},
		},
		"fake-prefix": {
			PrefixResults: []rebuild.RebuildPrefixResult{
				{
					Added:   []string{"http://u1", "http://u2", "http://u3"},
					Removed: nil,
				},
				{
					Prefix:  "web",
					Added:   []string{"http://u2", "http://u4"},
					Removed: nil,
				},
			},
		},
	})
	routes, err := routertest.FakeRouter.Routes(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 3)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, "http://u1"), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, "http://u2"), check.Equals, true)
	c.Assert(routertest.FakeRouter.HasRoute(a.Name, "http://u3"), check.Equals, true)
	routes, err = routertest.PrefixRouter.Routes(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(routes, check.HasLen, 3)
	c.Assert(routertest.PrefixRouter.HasRoute(a.Name, "http://u1"), check.Equals, true)
	c.Assert(routertest.PrefixRouter.HasRoute(a.Name, "http://u2"), check.Equals, true)
	c.Assert(routertest.PrefixRouter.HasRoute(a.Name, "http://u3"), check.Equals, true)

	err = routertest.PrefixRouter.RemoveRoutesPrefix(a.Name, appTypes.RoutableAddresses{
		Addresses: []*url.URL{{Host: "u2", Scheme: "http"}},
	}, true)
	c.Assert(err, check.IsNil)
	err = routertest.PrefixRouter.RemoveRoutesPrefix(a.Name, appTypes.RoutableAddresses{
		Prefix:    "web",
		Addresses: []*url.URL{{Host: "u4", Scheme: "http"}},
	}, true)
	c.Assert(err, check.IsNil)
	err = routertest.PrefixRouter.AddRoutesPrefix(a.Name, appTypes.RoutableAddresses{
		Prefix:    "web",
		Addresses: []*url.URL{{Host: "invalid", Scheme: "http"}},
	}, true)
	c.Assert(err, check.IsNil)
	err = routertest.PrefixRouter.AddRoutesPrefix(a.Name, appTypes.RoutableAddresses{
		Prefix:    "old",
		Addresses: []*url.URL{{Host: "u9", Scheme: "http"}},
	}, true)
	c.Assert(err, check.IsNil)

	changes, err = rebuild.RebuildRoutes(&a, false)
	c.Assert(err, check.IsNil)
	c.Assert(changes, check.DeepEquals, map[string]rebuild.RebuildRoutesResult{
		"fake": {
			PrefixResults: []rebuild.RebuildPrefixResult{
				{
					Added:   nil,
					Removed: nil,
				},
			},
		},
		"fake-prefix": {
			PrefixResults: []rebuild.RebuildPrefixResult{
				{
					Added:   []string{"http://u2"},
					Removed: nil,
				},
				{
					Prefix:  "old",
					Removed: []string{"http://u9"},
				},
				{
					Prefix:  "web",
					Added:   []string{"http://u4"},
					Removed: []string{"http://invalid"},
				},
			},
		},
	})

	prefixRoutes, err := routertest.PrefixRouter.RoutesPrefix(a.Name)
	c.Assert(err, check.IsNil)
	sort.Slice(prefixRoutes, func(i, j int) bool {
		return prefixRoutes[i].Prefix < prefixRoutes[j].Prefix
	})
	for k := range prefixRoutes {
		sort.Slice(prefixRoutes[k].Addresses, func(i, j int) bool {
			return prefixRoutes[k].Addresses[i].Host < prefixRoutes[k].Addresses[j].Host
		})
	}
	c.Assert(prefixRoutes, check.DeepEquals, []appTypes.RoutableAddresses{
		{
			Prefix:    "",
			Addresses: []*url.URL{{Host: "u1", Scheme: "http"}, {Host: "u2", Scheme: "http"}, {Host: "u3", Scheme: "http"}},
		},
		{
			Prefix:    "old",
			Addresses: []*url.URL{},
		},
		{
			Prefix:    "web",
			Addresses: []*url.URL{{Host: "u2", Scheme: "http"}, {Host: "u4", Scheme: "http"}},
		},
	})
}
