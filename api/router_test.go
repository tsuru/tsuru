// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/router"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

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
	config.Set("routers:router1:type", "foo")
	config.Set("routers:router2:type", "bar")
	defer config.Unset("routers:router1:type")
	defer config.Unset("routers:router2:type")
	recorder := httptest.NewRecorder()
	expected := []router.PlanRouter{
		{Name: "fake", Type: "fake", Default: true},
		{Name: "fake-tls", Type: "fake-tls"},
		{Name: "router1", Type: "foo"},
		{Name: "router2", Type: "bar"},
	}
	request, err := http.NewRequest("GET", "/routers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var routers []router.PlanRouter
	err = json.Unmarshal(recorder.Body.Bytes(), &routers)
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, expected)
}

func (s *S) TestRoutersListAppCreatePermissionTeam(c *check.C) {
	config.Set("routers:router1:type", "foo")
	config.Set("routers:router2:type", "bar")
	defer config.Unset("routers:router1:type")
	defer config.Unset("routers:router2:type")
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permission.CtxTeam, "tsuruteam"),
	})
	err := pool.SetPoolConstraint(&pool.PoolConstraint{PoolExpr: "test1", Field: "router", Values: []string{"router1", "router2"}})
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/routers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	var routers []router.PlanRouter
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	err = json.Unmarshal(recorder.Body.Bytes(), &routers)
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []router.PlanRouter{
		{Name: "router1", Type: "foo"},
		{Name: "router2", Type: "bar"},
	})
}

func (s *S) TestListAppRouters(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppReadRouter,
		Context: permission.Context(permission.CtxTeam, "tsuruteam"),
	})
	myapp := app.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(&myapp, s.user)
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
		{Name: "fake", Opts: map[string]string{}, Address: "myapp.fakerouter.com"},
	})
}

func (s *S) TestListAppRoutersEmpty(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppReadRouter,
		Context: permission.Context(permission.CtxTeam, "tsuruteam"),
	})
	myapp := app.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(&myapp, s.user)
	c.Assert(err, check.IsNil)
	err = myapp.RemoveRouter("fake")
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/1.5/apps/myapp/routers", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestAddAppRouter(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateRouterAdd,
		Context: permission.Context(permission.CtxTeam, "tsuruteam"),
	})
	myapp := app.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(&myapp, s.user)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(`name=fake-tls&opts.x=y&opts.z=w`)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/1.5/apps/myapp/routers", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	dbApp, err := app.GetByName(myapp.Name)
	c.Assert(err, check.IsNil)
	routers, err := dbApp.GetRoutersWithAddr()
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Opts: map[string]string{}, Address: "myapp.fakerouter.com"},
		{Name: "fake-tls", Opts: map[string]string{"x": "y", "z": "w"}, Address: "myapp.faketlsrouter.com"},
	})
}

func (s *S) TestAddAppRouterInvalidRouter(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateRouterAdd,
		Context: permission.Context(permission.CtxTeam, "tsuruteam"),
	})
	myapp := app.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(&myapp, s.user)
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

func (s *S) TestUpdateAppRouter(c *check.C) {
	config.Set("routers:fake-opts:type", "fake-opts")
	defer config.Unset("routers:fake-opts:type")
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateRouterUpdate,
		Context: permission.Context(permission.CtxTeam, "tsuruteam"),
	})
	myapp := app.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(&myapp, s.user)
	c.Assert(err, check.IsNil)
	err = myapp.AddRouter(appTypes.AppRouter{Name: "fake-opts"})
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`opts.x=y&opts.z=w`)
	request, err := http.NewRequest("PUT", "/1.5/apps/myapp/routers/fake-opts", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	dbApp, err := app.GetByName(myapp.Name)
	c.Assert(err, check.IsNil)
	routers := dbApp.GetRouters()
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{
		{Name: "fake", Opts: map[string]string{}},
		{Name: "fake-opts", Opts: map[string]string{"x": "y", "z": "w"}},
	})
}

func (s *S) TestUpdateAppRouterNotFound(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateRouterUpdate,
		Context: permission.Context(permission.CtxTeam, "tsuruteam"),
	})
	myapp := app.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(&myapp, s.user)
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

func (s *S) TestRemoveAppRouter(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateRouterRemove,
		Context: permission.Context(permission.CtxTeam, "tsuruteam"),
	})
	myapp := app.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(&myapp, s.user)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/1.5/apps/myapp/routers/fake", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	dbApp, err := app.GetByName(myapp.Name)
	c.Assert(err, check.IsNil)
	routers, err := dbApp.GetRoutersWithAddr()
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []appTypes.AppRouter{})
}

func (s *S) TestRemoveAppRouterNotFound(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateRouterRemove,
		Context: permission.Context(permission.CtxTeam, "tsuruteam"),
	})
	myapp := app.App{Name: "myapp", Platform: "go", TeamOwner: s.team.Name}
	err := app.CreateApp(&myapp, s.user)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/1.5/apps/myapp/routers?name=xyz", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}
