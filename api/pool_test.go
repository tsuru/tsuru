// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestAddPoolHandler(c *check.C) {
	b := bytes.NewBufferString(`{"name": "pool1"}`)
	req, err := http.NewRequest("POST", "/pool", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	defer provision.RemovePool("pool1")
	err = addPoolHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
	pools, err := provision.ListPools(bson.M{"_id": "pool1"})
	c.Assert(err, check.IsNil)
	c.Assert(len(pools), check.Equals, 1)
	b = bytes.NewBufferString(`{"name": "pool2", "public": true}`)
	req, err = http.NewRequest("POST", "/pool", b)
	c.Assert(err, check.IsNil)
	rec = httptest.NewRecorder()
	defer provision.RemovePool("pool2")
	err = addPoolHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
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
	b := bytes.NewBufferString(`{"pool": "pool1"}`)
	req, err := http.NewRequest("DELETE", "/pool", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = removePoolHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
	p, err := provision.ListPools(bson.M{"_id": "pool1"})
	c.Assert(err, check.IsNil)
	c.Assert(len(p), check.Equals, 0)
}

func (s *S) TestAddTeamsToPoolHandler(c *check.C) {
	pool := provision.Pool{Name: "pool1"}
	opts := provision.AddPoolOptions{Name: pool.Name}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool(pool.Name)
	b := bytes.NewBufferString(`{"pool": "pool1", "teams": ["test"]}`)
	req, err := http.NewRequest("POST", "/pool/pool1/team?:name=pool1", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = addTeamToPoolHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
	p, err := provision.ListPools(bson.M{"_id": "pool1"})
	c.Assert(err, check.IsNil)
	c.Assert(p[0].Teams, check.DeepEquals, []string{"test"})
}

func (s *S) TestRemoveTeamsToPoolHandler(c *check.C) {
	pool := provision.Pool{Name: "pool1", Teams: []string{"test"}}
	opts := provision.AddPoolOptions{Name: pool.Name}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool(pool.Name, pool.Teams)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool(pool.Name)
	b := bytes.NewBufferString(`{"pool": "pool1", "teams": ["test"]}`)
	req, err := http.NewRequest("DELETE", "/pool/pool1/team?:name=pool1", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = removeTeamToPoolHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
	p, err := provision.ListPools(nil)
	c.Assert(err, check.IsNil)
	c.Assert(p[0].Teams, check.DeepEquals, []string{})
}

func (s *S) TestListPoolsToUserHandler(c *check.C) {
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
	poolsExpected := map[string]interface{}{
		"pools_by_team": []interface{}{map[string]interface{}{"Team": "angra", "Pools": []interface{}{"pool1"}}},
		"public_pools":  []interface{}{},
	}
	req, err := http.NewRequest("GET", "/pool", nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = listPoolsToUser(rec, req, token)
	c.Assert(err, check.IsNil)
	var pools map[string]interface{}
	err = json.NewDecoder(rec.Body).Decode(&pools)
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.DeepEquals, poolsExpected)
}

func (s *S) TestListPoolsToUserEmptyHandler(c *check.C) {
	u := auth.User{Email: "passing-by@angra.com", Password: "123456"}
	_, err := nativeScheme.Create(&u)
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	token, err := nativeScheme.Login(map[string]string{"email": u.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	defer s.conn.Tokens().Remove(bson.M{"token": token.GetValue()})
	poolsExpected := map[string]interface{}{
		"pools_by_team": []interface{}{},
		"public_pools":  []interface{}{},
	}
	req, err := http.NewRequest("GET", "/pool", nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = listPoolsToUser(rec, req, token)
	c.Assert(err, check.IsNil)
	var pools map[string]interface{}
	err = json.NewDecoder(rec.Body).Decode(&pools)
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.DeepEquals, poolsExpected)
}

func (s *S) TestPoolUpdateToPublicHandler(c *check.C) {
	opts := provision.AddPoolOptions{Name: "pool1"}
	err := provision.AddPool(opts)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool("pool1")
	b := bytes.NewBufferString(`{"public": true}`)
	req, err := http.NewRequest("POST", "/pool/pool1?:name=pool1", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = poolUpdateHandler(rec, req, s.token)
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
	b := bytes.NewBufferString(`{"default": true}`)
	req, err := http.NewRequest("POST", "/pool/pool1?:name=pool1", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = poolUpdateHandler(rec, req, s.token)
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
	b := bytes.NewBufferString(`{"default": true}`)
	req, err := http.NewRequest("POST", "/pool/pool1?:name=pool2&force=true", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = poolUpdateHandler(rec, req, s.token)
	c.Assert(err, check.IsNil)
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
	b := bytes.NewBufferString(`{"default": true}`)
	req, err := http.NewRequest("POST", "/pool/pool2?:name=pool2", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = poolUpdateHandler(rec, req, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusPreconditionFailed)
	c.Assert(e.Message, check.Equals, "Default pool already exists.")
}
