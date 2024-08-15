// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/cezarsa/form"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/pool"
	authTypes "github.com/tsuru/tsuru/types/auth"
	eventTypes "github.com/tsuru/tsuru/types/event"
	permTypes "github.com/tsuru/tsuru/types/permission"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	check "gopkg.in/check.v1"
)

func (s *S) TestAddPoolNameIsRequired(c *check.C) {
	b := bytes.NewBufferString("name=")
	request, err := http.NewRequest(http.MethodPost, "/pools", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, pool.ErrPoolNameIsRequired.Error()+"\n")
}

func (s *S) TestAddPoolDefaultPoolAlreadyExists(c *check.C) {
	b := bytes.NewBufferString("name=pool1&default=true")
	req, err := http.NewRequest(http.MethodPost, "/pools", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusConflict)
	c.Assert(rec.Body.String(), check.Equals, pool.ErrDefaultPoolAlreadyExists.Error()+"\n")
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypePool, Value: "pool1"},
		Owner:  s.token.GetUserName(),
		Kind:   "pool.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "pool1"},
			{"name": "default", "value": "true"},
		},
		ErrorMatches: `Default pool already exists\.`,
	}, eventtest.HasEvent)
}

func (s *S) TestAddPoolAlreadyExists(c *check.C) {
	req, err := http.NewRequest(http.MethodPost, "/pools", bytes.NewBufferString("name=pool1"))
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
	rec = httptest.NewRecorder()
	req.Body = io.NopCloser(bytes.NewBufferString("name=pool1"))
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusConflict)
	c.Assert(rec.Body.String(), check.Equals, pool.ErrPoolAlreadyExists.Error()+"\n")
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypePool, Value: "pool1"},
		Owner:  s.token.GetUserName(),
		Kind:   "pool.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "pool1"},
		},
		ErrorMatches: `Pool already exists\.`,
	}, eventtest.HasEvent)
}

func (s *S) TestAddPool(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	b := bytes.NewBufferString("name=pool1")
	req, err := http.NewRequest(http.MethodPost, "/pools", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
	c.Assert(err, check.IsNil)
	_, err = pool.GetPoolByName(context.TODO(), "pool1")
	c.Assert(err, check.IsNil)
	b = bytes.NewBufferString("name=pool2&public=true")
	req, err = http.NewRequest(http.MethodPost, "/pools", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
	p, err := pool.GetPoolByName(context.TODO(), "pool2")
	c.Assert(err, check.IsNil)
	teams, err := p.GetTeams(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypePool, Value: "pool1"},
		Owner:  s.token.GetUserName(),
		Kind:   "pool.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "pool1"},
		},
	}, eventtest.HasEvent)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypePool, Value: "pool2"},
		Owner:  s.token.GetUserName(),
		Kind:   "pool.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "pool2"},
			{"name": "public", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRemovePoolNotFound(c *check.C) {
	req, err := http.NewRequest(http.MethodDelete, "/pools/not-found", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestRemovePoolHandler(c *check.C) {
	opts := pool.AddPoolOptions{
		Name: "pool1",
	}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodDelete, "/pools/pool1", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	_, err = pool.GetPoolByName(context.TODO(), "pool1")
	c.Assert(err, check.Equals, pool.ErrPoolNotFound)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypePool, Value: "pool1"},
		Owner:  s.token.GetUserName(),
		Kind:   "pool.delete",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": "pool1"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRemovePoolHandlerWithApp(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	opts := pool.AddPoolOptions{Name: "pool1"}
	a := app.App{
		Name:      "test",
		Platform:  "python",
		TeamOwner: s.team.Name,
		Pool:      opts.Name,
	}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodDelete, "/pools/pool1", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	expectedError := "This pool has apps, you need to migrate or remove them before removing the pool\n"
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusForbidden)
	c.Assert(rec.Body.String(), check.Equals, expectedError)
}

func (s *S) TestRemovePoolUserWithoutAppPerms(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	opts := pool.AddPoolOptions{Name: "pool1"}
	newUser := auth.User{
		Email: "newuser@example.com",
	}
	err := newUser.Create(context.TODO())
	c.Assert(err, check.IsNil)
	defer newUser.Delete(context.TODO())
	a := app.App{
		Name:      "test",
		Platform:  "python",
		TeamOwner: s.team.Name,
		Pool:      opts.Name,
	}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = app.CreateApp(context.TODO(), &a, &newUser)
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodDelete, "/pools/pool1", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	expectedError := "This pool has apps, you need to migrate or remove them before removing the pool\n"
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusForbidden)
	c.Assert(rec.Body.String(), check.Equals, expectedError)
}

func (s *S) TestAddTeamsToPoolWithoutTeam(c *check.C) {
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	b := strings.NewReader("")
	req, err := http.NewRequest(http.MethodPost, "/pools/pool1/team", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestAddTeamsToPool(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	b := strings.NewReader("team=tsuruteam")
	req, err := http.NewRequest(http.MethodPost, "/pools/pool1/team", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	p2, err := pool.GetPoolByName(context.TODO(), "pool1")
	c.Assert(err, check.IsNil)
	teams, err := p2.GetTeams(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypePool, Value: "pool1"},
		Owner:  s.token.GetUserName(),
		Kind:   "pool.update.team.add",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": "pool1"},
			{"name": "team", "value": s.team.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAddTeamsToPoolWithPoolContextPermission(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermPoolUpdateTeamAdd,
		Context: permission.Context(permTypes.CtxPool, "pool1"),
	})
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	b := strings.NewReader("team=tsuruteam")
	req, err := http.NewRequest(http.MethodPost, "/pools/pool1/team", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	_, err = pool.GetPoolByName(context.TODO(), "pool1")
	c.Assert(err, check.IsNil)
}

func (s *S) TestAddTeamsToPoolNotFound(c *check.C) {
	b := strings.NewReader("team=test")
	req, err := http.NewRequest(http.MethodPost, "/pools/notfound/team", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestRemoveTeamsFromPoolNotFound(c *check.C) {
	req, err := http.NewRequest(http.MethodDelete, "/pools/not-found/team?team=team", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestRemoveTeamsFromPoolWithoutTeam(c *check.C) {
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), p.Name, []string{"test"})
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodDelete, "/pools/pool1/team", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestRemoveTeamsFromPoolHandler(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), p.Name, []string{s.team.Name})
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), p.Name, []string{"ateam"})
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodDelete, "/pools/pool1/team?team=ateam", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	var p2 pool.Pool

	poolCollection, err := storagev2.PoolCollection()
	c.Assert(err, check.IsNil)
	err = poolCollection.FindOne(context.TODO(), mongoBSON.M{"_id": p.Name}).Decode(&p2)
	c.Assert(err, check.IsNil)

	c.Assert(err, check.IsNil)
	teams, err := p2.GetTeams(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypePool, Value: "pool1"},
		Owner:  s.token.GetUserName(),
		Kind:   "pool.update.team.remove",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": "pool1"},
			{"name": "team", "value": "ateam"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRemoveTeamsFromPoolWithPoolContextPermission(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermPoolUpdateTeamRemove,
		Context: permission.Context(permTypes.CtxPool, "pool1"),
	})
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), p.Name, []string{s.team.Name})
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), p.Name, []string{"ateam"})
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodDelete, "/pools/pool1/team?team=ateam", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	var p2 pool.Pool

	poolCollection, err := storagev2.PoolCollection()
	c.Assert(err, check.IsNil)
	err = poolCollection.FindOne(context.TODO(), mongoBSON.M{"_id": p.Name}).Decode(&p2)
	c.Assert(err, check.IsNil)

	c.Assert(err, check.IsNil)
	teams, err := p2.GetTeams(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.DeepEquals, []string{s.team.Name})
}

func (s *S) TestPoolListPublicPool(c *check.C) {
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name, Public: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "pool2"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	defaultPool, err := pool.GetDefaultPool(context.TODO())
	c.Assert(err, check.IsNil)
	token := userWithPermission(c)
	req, err := http.NewRequest(http.MethodGet, "/pools", nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = poolList(rec, req, token)
	c.Assert(err, check.IsNil)
	var pools []pool.PoolInfo
	err = json.NewDecoder(rec.Body).Decode(&pools)
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.HasLen, 2)
	c.Assert(pools[0].Name, check.Equals, defaultPool.Name)
	c.Assert(pools[0].Default, check.Equals, true)
	c.Assert(pools[0].Public, check.Equals, true)
	c.Assert(pools[0].Allowed, check.DeepEquals, map[pool.PoolConstraintType][]string{
		"plan":        {"default-plan"},
		"router":      {"fake", "fake-tls"},
		"service":     nil,
		"team":        {"tsuruteam"},
		"volume-plan": nil,
	})
	c.Assert(pools[0].Teams, check.DeepEquals, []string{"tsuruteam"})

	c.Assert(pools[1].Name, check.Equals, "pool1")
	c.Assert(pools[1].Allowed, check.DeepEquals, map[pool.PoolConstraintType][]string{
		"plan":        {"default-plan"},
		"router":      {"fake", "fake-tls"},
		"service":     nil,
		"team":        {"tsuruteam"},
		"volume-plan": nil,
	})
	c.Assert(pools[1].Public, check.Equals, true)
	c.Assert(pools[1].Teams, check.DeepEquals, []string{"tsuruteam"})
	c.Assert(pools[1].Default, check.Equals, false)
}

func (s *S) TestPoolListHandler(c *check.C) {
	teamName := "angra"
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, teamName),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, "foo_team"),
	})
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), p.Name, []string{teamName})
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "nopool"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	defaultPool, err := pool.GetDefaultPool(context.TODO())
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodGet, "/pools", nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = poolList(rec, req, token)
	c.Assert(err, check.IsNil)
	var pools []pool.PoolInfo
	err = json.NewDecoder(rec.Body).Decode(&pools)
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.HasLen, 2)
	c.Assert(pools[0].Name, check.DeepEquals, defaultPool.Name)
	c.Assert(pools[0].Default, check.Equals, true)
	c.Assert(pools[1].Name, check.DeepEquals, "pool1")
	c.Assert(pools[1].Default, check.Equals, false)
}

func (s *S) TestPoolListEmptyHandler(c *check.C) {
	poolCollection, err := storagev2.PoolCollection()
	c.Assert(err, check.IsNil)
	_, err = poolCollection.DeleteMany(context.TODO(), mongoBSON.M{})
	c.Assert(err, check.IsNil)
	u := auth.User{Email: "passing-by@angra.com", Password: "123456"}
	_, err = nativeScheme.Create(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(context.TODO(), map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodGet, "/pools", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "b "+token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestPoolListHandlerWithPermissionToDefault(c *check.C) {
	team := authTypes.Team{Name: "angra"}
	perms := []permission.Permission{
		{
			Scheme:  permission.PermAppCreate,
			Context: permission.Context(permTypes.CtxGlobal, ""),
		},
		{
			Scheme:  permission.PermPoolUpdate,
			Context: permission.Context(permTypes.CtxGlobal, ""),
		},
	}
	token := userWithPermission(c, perms...)
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name, Default: p.Default}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), p.Name, []string{team.Name})
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodGet, "/pools", nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = poolList(rec, req, token)
	c.Assert(err, check.IsNil)
	var pools []pool.PoolInfo
	err = json.NewDecoder(rec.Body).Decode(&pools)
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.HasLen, 2)
	c.Assert(pools[0].Name, check.Equals, "test1")
	c.Assert(pools[1].Name, check.Equals, "pool1")
}

func (s *S) TestPoolListHandlerWithGlobalContext(c *check.C) {
	perms := []permission.Permission{
		{
			Scheme:  permission.PermAll,
			Context: permission.Context(permTypes.CtxGlobal, ""),
		},
	}
	token := userWithPermission(c, perms...)
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name, Default: p.Default}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodGet, "/pools", nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = poolList(rec, req, token)
	c.Assert(err, check.IsNil)
	var pools []pool.PoolInfo
	err = json.NewDecoder(rec.Body).Decode(&pools)
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.HasLen, 2)
	c.Assert(pools[0].Name, check.Equals, "test1")
	c.Assert(pools[1].Name, check.Equals, "pool1")
}

func (s *S) TestPoolListHandlerWithPoolReadPermission(c *check.C) {
	perms := []permission.Permission{
		{
			Scheme:  permission.PermPoolRead,
			Context: permission.Context(permTypes.CtxPool, "pool1"),
		},
	}
	token := userWithPermission(c, perms...)
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	p = pool.Pool{Name: "pool2"}
	opts = pool.AddPoolOptions{Name: p.Name}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodGet, "/pools", nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = poolList(rec, req, token)
	c.Assert(err, check.IsNil)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	var pools []pool.Pool
	err = json.NewDecoder(rec.Body).Decode(&pools)
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.HasLen, 2)
	c.Assert(pools[0].Name, check.Equals, "test1")
	c.Assert(pools[1].Name, check.Equals, "pool1")
}

func (s *S) TestPoolUpdateToPublicHandler(c *check.C) {
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}}, nil
	}
	opts := pool.AddPoolOptions{Name: "pool1"}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{PoolExpr: "pool1", Field: pool.ConstraintTypeTeam, Values: []string{"*"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	p, err := pool.GetPoolByName(context.TODO(), "pool1")
	c.Assert(err, check.IsNil)
	_, err = p.GetTeams(context.TODO())
	c.Assert(err, check.NotNil)
	b := bytes.NewBufferString("public=true")
	req, err := http.NewRequest(http.MethodPut, "/pools/pool1", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	c.Assert(err, check.IsNil)
	teams, err := p.GetTeams(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypePool, Value: "pool1"},
		Owner:  s.token.GetUserName(),
		Kind:   "pool.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":name", "value": "pool1"},
			{"name": "public", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestPoolUpdateToDefaultPoolHandler(c *check.C) {
	pool.RemovePool(context.TODO(), "test1")
	opts := pool.AddPoolOptions{Name: "pool1"}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	b := bytes.NewBufferString("default=true")
	req, err := http.NewRequest(http.MethodPut, "/pools/pool1", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	c.Assert(err, check.IsNil)
	p, err := pool.GetPoolByName(context.TODO(), "pool1")
	c.Assert(err, check.IsNil)
	c.Assert(p.Default, check.Equals, true)
}

func (s *S) TestPoolUpdateOverwriteDefaultPoolHandler(c *check.C) {
	pool.RemovePool(context.TODO(), "test1")
	opts := pool.AddPoolOptions{Name: "pool1", Default: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "pool2"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	b := bytes.NewBufferString("default=true&force=true")
	req, err := http.NewRequest(http.MethodPut, "/pools/pool2", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	p, err := pool.GetPoolByName(context.TODO(), "pool2")
	c.Assert(err, check.IsNil)
	c.Assert(p.Default, check.Equals, true)
}

func (s *S) TestPoolUpdateNotOverwriteDefaultPoolHandler(c *check.C) {
	pool.RemovePool(context.TODO(), "test1")
	opts := pool.AddPoolOptions{Name: "pool1", Default: true}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	opts = pool.AddPoolOptions{Name: "pool2"}
	err = pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	b := bytes.NewBufferString("default=true")
	request, err := http.NewRequest(http.MethodPut, "/pools/pool2", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(recorder.Body.String(), check.Equals, pool.ErrDefaultPoolAlreadyExists.Error()+"\n")
}

func (s *S) TestPoolUpdateNotFound(c *check.C) {
	b := bytes.NewBufferString("public=true")
	request, err := http.NewRequest(http.MethodPut, "/pools/not-found", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestPoolConstraint(c *check.C) {
	err := pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{PoolExpr: "*", Field: pool.ConstraintTypeRouter, Values: []string{"*"}})
	c.Assert(err, check.IsNil)
	err = pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{PoolExpr: "dev", Field: pool.ConstraintTypeRouter, Values: []string{"dev"}})
	c.Assert(err, check.IsNil)
	err = pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{PoolExpr: "dev", Field: pool.ConstraintTypeVolumePlan, Values: []string{"faas"}})
	c.Assert(err, check.IsNil)
	expected := []pool.PoolConstraint{
		{PoolExpr: "test1", Field: pool.ConstraintTypeTeam, Values: []string{"*"}},
		{PoolExpr: "*", Field: pool.ConstraintTypeRouter, Values: []string{"*"}},
		{PoolExpr: "dev", Field: pool.ConstraintTypeRouter, Values: []string{"dev"}},
		{PoolExpr: "dev", Field: pool.ConstraintTypeVolumePlan, Values: []string{"faas"}},
	}
	request, err := http.NewRequest(http.MethodGet, "/constraints", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, request)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	var constraints []pool.PoolConstraint
	err = json.NewDecoder(rec.Body).Decode(&constraints)
	c.Assert(err, check.IsNil)
	c.Assert(constraints, check.DeepEquals, expected)
}

func (s *S) TestPoolConstraintListEmpty(c *check.C) {
	err := pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{PoolExpr: "test1", Field: pool.ConstraintTypeTeam, Values: []string{""}, Blacklist: true})
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest(http.MethodGet, "/1.3/constraints", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestPoolConstraintSet(c *check.C) {
	params := pool.PoolConstraint{
		PoolExpr:  "*",
		Blacklist: true,
		Field:     pool.ConstraintTypeRouter,
		Values:    []string{"routerA"},
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodPut, "/1.3/constraints", strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	expected := []*pool.PoolConstraint{
		{PoolExpr: "test1", Field: pool.ConstraintTypeTeam, Values: []string{"*"}},
		{PoolExpr: "*", Field: pool.ConstraintTypeRouter, Values: []string{"routerA"}, Blacklist: true},
	}
	constraints, err := pool.ListPoolsConstraints(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(constraints, check.DeepEquals, expected)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypePool, Value: "*"},
		Owner:  s.token.GetUserName(),
		Kind:   "pool.update.constraints.set",
		StartCustomData: []map[string]interface{}{
			{"name": "PoolExpr", "value": "*"},
			{"name": "Field", "value": "router"},
			{"name": "Values.0", "value": "routerA"},
			{"name": "Blacklist", "value": "true"},
			{"name": ":version", "value": "1.3"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestPoolConstraintSetAppend(c *check.C) {
	err := pool.SetPoolConstraint(context.TODO(), &pool.PoolConstraint{PoolExpr: "*", Field: pool.ConstraintTypeRouter, Values: []string{"routerA"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	params := pool.PoolConstraint{
		PoolExpr: "*",
		Field:    pool.ConstraintTypeRouter,
		Values:   []string{"routerB"},
	}
	v, err := form.EncodeToValues(&params)
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest(http.MethodPut, "/1.3/constraints?append=true", strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	expected := []*pool.PoolConstraint{
		{PoolExpr: "test1", Field: pool.ConstraintTypeTeam, Values: []string{"*"}},
		{PoolExpr: "*", Field: pool.ConstraintTypeRouter, Values: []string{"routerA", "routerB"}, Blacklist: true},
	}
	constraints, err := pool.ListPoolsConstraints(context.TODO(), nil)
	c.Assert(err, check.IsNil)
	c.Assert(constraints, check.DeepEquals, expected)
	c.Assert(eventtest.EventDesc{
		Target: eventTypes.Target{Type: eventTypes.TargetTypePool, Value: "*"},
		Owner:  s.token.GetUserName(),
		Kind:   "pool.update.constraints.set",
		StartCustomData: []map[string]interface{}{
			{"name": "PoolExpr", "value": "*"},
			{"name": "Field", "value": "router"},
			{"name": "Values.0", "value": "routerB"},
			{"name": "Blacklist", "value": ""},
			{"name": ":version", "value": "1.3"},
			{"name": "append", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestPoolConstraintSetRequiresPoolExpr(c *check.C) {
	req, err := http.NewRequest(http.MethodPut, "/constraints", bytes.NewBufferString(""))
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusBadRequest)
	c.Assert(rec.Body.String(), check.Equals, "You must provide a Pool Expression\n")
}

func (s *S) TestPoolGetHandler(c *check.C) {
	teamName := "angra"
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name}
	err := pool.AddPool(context.TODO(), opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool(context.TODO(), p.Name, []string{teamName})
	c.Assert(err, check.IsNil)
	expected := pool.Pool{
		Name:   "pool1",
		Labels: map[string]string{},
	}
	req, err := http.NewRequest(http.MethodGet, "/pools/pool1", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	var pool pool.Pool
	err = json.NewDecoder(rec.Body).Decode(&pool)
	c.Assert(err, check.IsNil)
	c.Assert(pool, check.DeepEquals, expected)
}
