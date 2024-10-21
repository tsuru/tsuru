// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/router/routertest"
	appTypes "github.com/tsuru/tsuru/types/app"
	permTypes "github.com/tsuru/tsuru/types/permission"
	routerTypes "github.com/tsuru/tsuru/types/router"
	check "gopkg.in/check.v1"
)

func (s *S) TestRoutersAdd(c *check.C) {
	var created routerTypes.DynamicRouter
	s.mockService.DynamicRouter.OnCreate = func(dr routerTypes.DynamicRouter) error {
		created = dr
		return nil
	}
	body := `{"name": "r1", "type": "t1", "config": {"a": "b"}}`
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/routers", strings.NewReader(body))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	c.Assert(created, check.DeepEquals, routerTypes.DynamicRouter{
		Name:   "r1",
		Type:   "t1",
		Config: map[string]interface{}{"a": "b"},
	})
}

func (s *S) TestRoutersUpdate(c *check.C) {
	var updated routerTypes.DynamicRouter
	s.mockService.DynamicRouter.OnUpdate = func(dr routerTypes.DynamicRouter) error {
		updated = dr
		return nil
	}
	body := `{"type": "t1", "config": {"a": "b"}}`
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("PUT", "/routers/r1", strings.NewReader(body))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(updated, check.DeepEquals, routerTypes.DynamicRouter{
		Name:   "r1",
		Type:   "t1",
		Config: map[string]interface{}{"a": "b"},
	})
}

func (s *S) TestRoutersDelete(c *check.C) {
	var removed string
	s.mockService.DynamicRouter.OnRemove = func(rtName string) error {
		removed = rtName
		return nil
	}
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/routers/r1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(removed, check.Equals, "r1")
}

func (s *S) TestRoutersListNoContent(c *check.C) {
	err := config.Unset("routers")
	c.Assert(err, check.IsNil)
	defer config.Set("routers:fake:type", "fake")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/plans/routers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestRoutersList(c *check.C) {
	dr := routerTypes.DynamicRouter{
		Name:   "dr1",
		Type:   "foo",
		Config: map[string]interface{}{"a": "b"},
	}
	s.mockService.DynamicRouter.OnList = func() ([]routerTypes.DynamicRouter, error) {
		return []routerTypes.DynamicRouter{dr}, nil
	}
	s.mockService.DynamicRouter.OnGet = func(name string) (*routerTypes.DynamicRouter, error) {
		return &dr, nil
	}
	config.Set("routers:router1:type", "foo")
	config.Set("routers:router2:type", "bar")
	config.Set("routers:router2:mycfg", "1")
	defer config.Unset("routers:router1")
	defer config.Unset("routers:router2")
	router.Register("foo", func(_ string, _ router.ConfigGetter) (router.Router, error) { return &routertest.FakeRouter, nil })
	router.Register("bar", func(_ string, _ router.ConfigGetter) (router.Router, error) { return &routertest.FakeRouter, nil })
	defer router.Unregister("foo")
	defer router.Unregister("bar")
	recorder := httptest.NewRecorder()
	expected := []routerTypes.PlanRouter{
		{Name: "dr1", Type: "foo", Dynamic: true, Config: map[string]interface{}{"a": "b"}, Info: map[string]string{}},
		{Name: "fake", Type: "fake", Default: true, Info: map[string]string{}},
		{Name: "fake-tls", Type: "fake-tls", Info: map[string]string{}},
		{Name: "router1", Type: "foo", Info: map[string]string{}},
		{Name: "router2", Type: "bar", Config: map[string]interface{}{"mycfg": "1"}, Info: map[string]string{}},
	}
	request, err := http.NewRequest("GET", "/routers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var routers []routerTypes.PlanRouter
	err = json.Unmarshal(recorder.Body.Bytes(), &routers)
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, expected)
}

func (s *S) TestRoutersListRestrictedTokeNoConfig(c *check.C) {
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	config.Set("routers:router1:type", "foo")
	config.Set("routers:router2:type", "bar")
	config.Set("routers:router2:mycfg", "1")
	defer config.Unset("routers:router1:type")
	defer config.Unset("routers:router2:type")
	router.Register("foo", func(_ string, _ router.ConfigGetter) (router.Router, error) { return &routertest.FakeRouter, nil })
	router.Register("bar", func(_ string, _ router.ConfigGetter) (router.Router, error) { return &routertest.FakeRouter, nil })
	defer router.Unregister("foo")
	defer router.Unregister("bar")
	recorder := httptest.NewRecorder()
	expected := []routerTypes.PlanRouter{
		{Name: "fake", Type: "fake", Default: true, Info: map[string]string{}},
		{Name: "fake-tls", Type: "fake-tls", Info: map[string]string{}},
		{Name: "router1", Type: "foo", Info: map[string]string{}},
		{Name: "router2", Type: "bar", Info: map[string]string{}},
	}
	request, err := http.NewRequest("GET", "/routers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var routers []routerTypes.PlanRouter
	err = json.Unmarshal(recorder.Body.Bytes(), &routers)
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, expected)
}

func (s *S) TestRoutersListAppCreatePermissionTeam(c *check.C) {
	config.Set("routers:router1:type", "foo")
	config.Set("routers:router2:type", "bar")
	defer config.Unset("routers:router1:type")
	defer config.Unset("routers:router2:type")
	router.Register("foo", func(_ string, _ router.ConfigGetter) (router.Router, error) { return &routertest.FakeRouter, nil })
	router.Register("bar", func(_ string, _ router.ConfigGetter) (router.Router, error) { return &routertest.FakeRouter, nil })
	defer router.Unregister("foo")
	defer router.Unregister("bar")
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "tsuruteam"),
	})
	err := pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{PoolExpr: "test1", Field: pool.ConstraintTypeRouter, Values: []string{"router1", "router2"}})
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/routers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	var routers []routerTypes.PlanRouter
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	err = json.Unmarshal(recorder.Body.Bytes(), &routers)
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []routerTypes.PlanRouter{
		{Name: "router1", Type: "foo", Info: map[string]string{}},
		{Name: "router2", Type: "bar", Info: map[string]string{}},
	})
}

func (s *S) TestRoutersListWhenPoolHasNoRouterShouldNotReturnError(c *check.C) {
	err := config.Unset("routers")
	c.Assert(err, check.IsNil)
	err = pool.RemovePool(context.TODO(), s.Pool)
	c.Assert(err, check.IsNil)
	router1 := routerTypes.DynamicRouter{Name: "router-1", Type: "api", Config: map[string]interface{}{"key1": "value1", "key2": "value2"}}
	router2 := routerTypes.DynamicRouter{Name: "router-2", Type: "api"}
	router.Register("api", func(_ string, _ router.ConfigGetter) (router.Router, error) { return &routertest.FakeRouter, nil })
	defer router.Unregister("api")
	s.mockService.DynamicRouter.OnList = func() ([]routerTypes.DynamicRouter, error) {
		return []routerTypes.DynamicRouter{router1, router2}, nil
	}
	s.mockService.DynamicRouter.OnGet = func(name string) (*routerTypes.DynamicRouter, error) {
		switch name {
		case "router-1":
			return &router1, nil
		case "router-2":
			return &router2, nil
		default:
			return nil, stderrors.New("some error")
		}
	}
	err = pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool-1"})
	c.Assert(err, check.IsNil)
	err = pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{PoolExpr: "pool-1", Field: pool.ConstraintTypeRouter, Values: []string{"router-1"}})
	c.Assert(err, check.IsNil)
	err = pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{PoolExpr: "pool-1", Field: pool.ConstraintTypeTeam, Values: []string{"my-team"}})
	c.Assert(err, check.IsNil)
	// pool-2 constraint for routers doesn't match any valid router
	err = pool.AddPool(context.TODO(), pool.AddPoolOptions{Name: "pool-2"})
	c.Assert(err, check.IsNil)
	err = pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{PoolExpr: "pool-2", Field: pool.ConstraintTypeRouter, Values: []string{"not-found-router"}})
	c.Assert(err, check.IsNil)
	err = pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{PoolExpr: "pool-2", Field: pool.ConstraintTypeTeam, Values: []string{"my-team"}})
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "my-team"),
	})
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/routers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	var routers []routerTypes.PlanRouter
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	err = json.Unmarshal(recorder.Body.Bytes(), &routers)
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []routerTypes.PlanRouter{{Name: "router-1", Type: "api", Dynamic: true, Info: map[string]string{}}})
}

func (s *S) TestListRoutersWithInfo(c *check.C) {
	routertest.FakeRouter.Reset()
	routertest.FakeRouter.Info = map[string]string{
		"info1": "val1",
		"info2": "val2",
	}
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/routers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	var routers []routerTypes.PlanRouter
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	err = json.Unmarshal(recorder.Body.Bytes(), &routers)
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []routerTypes.PlanRouter{
		{Name: "fake", Type: "fake", Default: true, Info: map[string]string{
			"info1": "val1",
			"info2": "val2",
		}},
		{Name: "fake-tls", Type: "fake-tls", Info: map[string]string{}},
	})
}

func (s *S) TestListAppRouters(c *check.C) {
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppReadRouter,
		Context: permission.Context(permTypes.CtxTeam, "tsuruteam"),
	})
	myapp := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &myapp, s.user)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/1.5/apps/myapp/routers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	var routers []appTypes.AppRouter
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	err = json.Unmarshal(recorder.Body.Bytes(), &routers)
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Opts: map[string]string{}, Type: "fake", Address: "myapp.fakerouter.com", Addresses: []string{"myapp.fakerouter.com"}, Status: "ready"},
	})
}

func (s *S) TestListAppRoutersWithStatus(c *check.C) {
	ctx := context.Background()
	routertest.FakeRouter.Status.Status = router.BackendStatusNotReady
	routertest.FakeRouter.Status.Detail = "burn"
	defer routertest.FakeRouter.Reset()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppReadRouter,
		Context: permission.Context(permTypes.CtxTeam, "tsuruteam"),
	})
	myapp := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name, Router: "none"}
	err := app.CreateApp(context.TODO(), &myapp, s.user)
	c.Assert(err, check.IsNil)
	err = myapp.AddRouter(ctx, appTypes.AppRouter{
		Name: "fake",
	})
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/1.5/apps/myapp/routers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	var routers []appTypes.AppRouter
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	err = json.Unmarshal(recorder.Body.Bytes(), &routers)
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Opts: map[string]string{}, Type: "fake", Status: "not ready", StatusDetail: "burn", Address: "myapp.fakerouter.com", Addresses: []string{"myapp.fakerouter.com"}},
	})
}

func (s *S) TestListAppRoutersEmpty(c *check.C) {
	ctx := context.Background()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppReadRouter,
		Context: permission.Context(permTypes.CtxTeam, "tsuruteam"),
	})
	myapp := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &myapp, s.user)
	c.Assert(err, check.IsNil)
	err = myapp.RemoveRouter(ctx, "fake")
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/1.5/apps/myapp/routers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestAddAppRouter(c *check.C) {
	ctx := context.Background()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateRouterAdd,
		Context: permission.Context(permTypes.CtxTeam, "tsuruteam"),
	})
	myapp := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &myapp, s.user)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(`name=fake-tls&opts.x=y&opts.z=w`)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/1.5/apps/myapp/routers", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	dbApp, err := app.GetByName(context.TODO(), myapp.Name)
	c.Assert(err, check.IsNil)
	routers, err := dbApp.GetRoutersWithAddr(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Opts: map[string]string{}, Type: "fake", Address: "myapp.fakerouter.com", Addresses: []string{"myapp.fakerouter.com"}, Status: "ready"},
		{Name: "fake-tls", Opts: map[string]string{"x": "y", "z": "w"}, Type: "fake-tls", Address: "https://myapp.faketlsrouter.com", Addresses: []string{"https://myapp.faketlsrouter.com"}, Status: "ready"},
	})
}

func (s *S) TestAddAppRouterInvalidRouter(c *check.C) {
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateRouterAdd,
		Context: permission.Context(permTypes.CtxTeam, "tsuruteam"),
	})
	myapp := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &myapp, s.user)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(`name=fake-notfound&opts.x=y&opts.z=w`)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/1.5/apps/myapp/routers", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestAddAppRouterBlockedByConstraint(c *check.C) {
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateRouterAdd,
		Context: permission.Context(permTypes.CtxTeam, "tsuruteam"),
	})
	myapp := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &myapp, s.user)
	c.Assert(err, check.IsNil)
	err = pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{PoolExpr: "*", Field: pool.ConstraintTypeRouter, Values: []string{"fake-tls"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	body := strings.NewReader(`name=fake-tls&opts.x=y&opts.z=w`)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/1.5/apps/myapp/routers", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestUpdateAppRouter(c *check.C) {
	ctx := context.Background()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateRouterUpdate,
		Context: permission.Context(permTypes.CtxTeam, "tsuruteam"),
	})
	myapp := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name, Router: "none"}
	err := app.CreateApp(context.TODO(), &myapp, s.user)
	c.Assert(err, check.IsNil)
	err = myapp.AddRouter(ctx, appTypes.AppRouter{Name: "fake"})
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`opts.x=y&opts.z=w`)
	request, err := http.NewRequest("PUT", "/1.5/apps/myapp/routers/fake", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	dbApp, err := app.GetByName(context.TODO(), myapp.Name)
	c.Assert(err, check.IsNil)
	routers := dbApp.GetRouters()
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Opts: map[string]string{"x": "y", "z": "w"}},
	})
}

func (s *S) TestUpdateAppRouterNotFound(c *check.C) {
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateRouterUpdate,
		Context: permission.Context(permTypes.CtxTeam, "tsuruteam"),
	})
	myapp := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &myapp, s.user)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`opts.x=y&opts.z=w`)
	request, err := http.NewRequest("PUT", "/1.5/apps/myapp/routers/xyz", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestUpdateAppRouterBlockedByConstraint(c *check.C) {
	ctx := context.Background()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateRouterUpdate,
		Context: permission.Context(permTypes.CtxTeam, "tsuruteam"),
	})
	myapp := appTypes.App{Name: "apptest", Platform: "go", TeamOwner: s.team.Name, Router: "none"}
	err := app.CreateApp(context.TODO(), &myapp, s.user)
	c.Assert(err, check.IsNil)
	err = myapp.AddRouter(ctx, appTypes.AppRouter{Name: "fake"})
	c.Assert(err, check.IsNil)
	err = pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{PoolExpr: "*", Field: pool.ConstraintTypeRouter, Values: []string{"fake"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`opts.x=y&opts.z=w`)
	request, err := http.NewRequest("PUT", "/1.5/apps/apptest/routers/fake", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestRemoveAppRouter(c *check.C) {
	ctx := context.Background()
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateRouterRemove,
		Context: permission.Context(permTypes.CtxTeam, "tsuruteam"),
	})
	myapp := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &myapp, s.user)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/1.5/apps/myapp/routers/fake", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	dbApp, err := app.GetByName(context.TODO(), myapp.Name)
	c.Assert(err, check.IsNil)
	routers, err := dbApp.GetRoutersWithAddr(ctx)
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{})
}

func (s *S) TestRemoveAppRouterNotFound(c *check.C) {
	token := userWithPermission(c, permTypes.Permission{
		Scheme:  permission.PermAppUpdateRouterRemove,
		Context: permission.Context(permTypes.CtxTeam, "tsuruteam"),
	})
	myapp := appTypes.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &myapp, s.user)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/1.5/apps/myapp/routers?name=xyz", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}
