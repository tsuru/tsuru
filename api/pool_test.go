// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestAddPoolNameIsRequired(c *check.C) {
	b := bytes.NewBufferString("name=")
	request, err := http.NewRequest("POST", "/pools", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, provision.ErrPoolNameIsRequired.Error()+"\n")
}

func (s *S) TestAddPoolDefaultPoolAlreadyExists(c *check.C) {
	b := bytes.NewBufferString("name=pool1&default=true")
	req, err := http.NewRequest("POST", "/pools", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	defer provision.RemovePool("pool1")
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusConflict)
	c.Assert(rec.Body.String(), check.Equals, provision.ErrDefaultPoolAlreadyExists.Error()+"\n")
}

func (s *S) TestAddPool(c *check.C) {
	b := bytes.NewBufferString("name=pool1")
	req, err := http.NewRequest("POST", "/pools", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	defer provision.RemovePool("pool1")
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
	c.Assert(err, check.IsNil)
	pools, err := provision.ListPools(bson.M{"_id": "pool1"})
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.HasLen, 1)
	b = bytes.NewBufferString("name=pool2&public=true")
	req, err = http.NewRequest("POST", "/pools", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	defer provision.RemovePool("pool2")
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusCreated)
	pools, err = provision.ListPools(bson.M{"_id": "pool2"})
	c.Assert(err, check.IsNil)
	c.Assert(pools[0].Public, check.Equals, true)
}

func (s *S) TestRemovePoolHandler(c *check.C) {
	opts := provision.AddPoolOptions{
		Name: "pool1",
	}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	req, err := http.NewRequest("DELETE", "/pools/pool1", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	err = removePoolHandler(rec, req, s.token)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	p, err := provision.ListPools(bson.M{"_id": "pool1"})
	c.Assert(err, check.IsNil)
	c.Assert(p, check.HasLen, 0)
}

func (s *S) TestAddTeamsToPool(c *check.C) {
	pool := provision.Pool{Name: "pool1"}
	opts := provision.AddPoolOptions{Name: pool.Name}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool(pool.Name)
	b := strings.NewReader("team=test")
	req, err := http.NewRequest("POST", "/pools/pool1/team", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	p, err := provision.ListPools(bson.M{"_id": "pool1"})
	c.Assert(err, check.IsNil)
	c.Assert(p[0].Teams, check.DeepEquals, []string{"test"})
}

func (s *S) TestRemoveTeamsToPoolWithoutTeam(c *check.C) {
	pool := provision.Pool{Name: "pool1", Teams: []string{"test"}}
	opts := provision.AddPoolOptions{Name: pool.Name}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool(pool.Name, pool.Teams)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool(pool.Name)
	req, err := http.NewRequest("DELETE", "/pools/pool1/team", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestRemoveTeamsToPoolHandler(c *check.C) {
	pool := provision.Pool{Name: "pool1", Teams: []string{"test"}}
	opts := provision.AddPoolOptions{Name: pool.Name}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool(pool.Name, pool.Teams)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool(pool.Name)
	req, err := http.NewRequest("DELETE", "/pools/pool1/team?team=test", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	var p provision.Pool
	err = s.conn.Pools().FindId(pool.Name).One(&p)
	c.Assert(err, check.IsNil)
	c.Assert(p.Teams, check.DeepEquals, []string{})
}

func (s *S) TestPoolListPublicPool(c *check.C) {
	pool := provision.Pool{Name: "pool1", Public: true}
	opts := provision.AddPoolOptions{Name: pool.Name, Public: pool.Public}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool(pool.Name)
	defaultPools, err := provision.ListPools(bson.M{"default": true})
	c.Assert(err, check.IsNil)
	expected := []provision.Pool{
		defaultPools[0],
		{Name: "pool1", Public: true, Teams: []string{}},
	}
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermTeamCreate,
		Context: permission.Context(permission.CtxGlobal, ""),
	})
	req, err := http.NewRequest("GET", "/pools", nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = poolList(rec, req, token)
	c.Assert(err, check.IsNil)
	var pools []provision.Pool
	err = json.NewDecoder(rec.Body).Decode(&pools)
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.DeepEquals, expected)
}

func (s *S) TestPoolListHandler(c *check.C) {
	team := auth.Team{Name: "angra"}
	err := s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permission.CtxTeam, "angra"),
	})
	pool := provision.Pool{Name: "pool1", Teams: []string{"angra"}}
	opts := provision.AddPoolOptions{Name: pool.Name}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool(pool.Name, pool.Teams)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool(pool.Name)
	opts = provision.AddPoolOptions{Name: "nopool"}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool("nopool")
	defaultPools, err := provision.ListPools(bson.M{"default": true})
	c.Assert(err, check.IsNil)
	expected := []provision.Pool{
		defaultPools[0],
		{Name: "pool1", Teams: []string{"angra"}},
	}
	req, err := http.NewRequest("GET", "/pools", nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = poolList(rec, req, token)
	c.Assert(err, check.IsNil)
	var pools []provision.Pool
	err = json.NewDecoder(rec.Body).Decode(&pools)
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.DeepEquals, expected)
}

func (s *S) TestPoolListEmptyHandler(c *check.C) {
	_, err := s.conn.Pools().RemoveAll(nil)
	c.Assert(err, check.IsNil)
	u := auth.User{Email: "passing-by@angra.com", Password: "123456"}
	_, err = nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	req, err := http.NewRequest("GET", "/pools", nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "b "+token.GetValue())
	rec := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestPoolListHandlerWithPermissionToDefault(c *check.C) {
	team := auth.Team{Name: "angra"}
	err := s.conn.Teams().Insert(team)
	c.Assert(err, check.IsNil)
	perms := []permission.Permission{
		{
			Scheme:  permission.PermAppCreate,
			Context: permission.Context(permission.CtxGlobal, ""),
		},
		{
			Scheme:  permission.PermPoolUpdate,
			Context: permission.Context(permission.CtxGlobal, ""),
		},
	}
	token := userWithPermission(c, perms...)
	pool := provision.Pool{Name: "pool1", Teams: []string{team.Name}}
	opts := provision.AddPoolOptions{Name: pool.Name, Default: pool.Default}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool(pool.Name, pool.Teams)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool(pool.Name)
	req, err := http.NewRequest("GET", "/pools", nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = poolList(rec, req, token)
	c.Assert(err, check.IsNil)
	var pools []provision.Pool
	err = json.NewDecoder(rec.Body).Decode(&pools)
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.HasLen, 2)
	c.Assert(pools[0].Name, check.Equals, "test1")
	c.Assert(pools[1].Name, check.Equals, "pool1")
}

func (s *S) TestPoolUpdateToPublicHandler(c *check.C) {
	opts := provision.AddPoolOptions{Name: "pool1"}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool("pool1")
	b := bytes.NewBufferString("public=true")
	req, err := http.NewRequest("PUT", "/pools/pool1", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	c.Assert(err, check.IsNil)
	p, err := provision.ListPools(bson.M{"_id": "pool1"})
	c.Assert(err, check.IsNil)
	c.Assert(p[0].Public, check.Equals, true)
}

func (s *S) TestPoolUpdateToDefaultPoolHandler(c *check.C) {
	provision.RemovePool("test1")
	opts := provision.AddPoolOptions{Name: "pool1"}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool("pool1")
	b := bytes.NewBufferString("default=true")
	req, err := http.NewRequest("PUT", "/pools/pool1", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	c.Assert(err, check.IsNil)
	p, err := provision.ListPools(bson.M{"_id": "pool1"})
	c.Assert(err, check.IsNil)
	c.Assert(p[0].Default, check.Equals, true)
}

func (s *S) TestPoolUpdateOverwriteDefaultPoolHandler(c *check.C) {
	provision.RemovePool("test1")
	opts := provision.AddPoolOptions{Name: "pool1", Default: true}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool("pool1")
	opts = provision.AddPoolOptions{Name: "pool2"}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool("pool2")
	b := bytes.NewBufferString("default=true&force=true")
	req, err := http.NewRequest("PUT", "/pools/pool2", b)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	rec := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
	p, err := provision.ListPools(bson.M{"_id": "pool2"})
	c.Assert(err, check.IsNil)
	c.Assert(p[0].Default, check.Equals, true)
}

func (s *S) TestPoolUpdateNotOverwriteDefaultPoolHandler(c *check.C) {
	provision.RemovePool("test1")
	opts := provision.AddPoolOptions{Name: "pool1", Default: true}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool("pool1")
	opts = provision.AddPoolOptions{Name: "pool2"}
	err = provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool("pool2")
	b := bytes.NewBufferString("default=true")
	request, err := http.NewRequest("PUT", "/pools/pool2", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(recorder.Body.String(), check.Equals, provision.ErrDefaultPoolAlreadyExists.Error()+"\n")
}
