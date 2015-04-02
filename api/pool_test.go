// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
)

func (s *S) TestAddPoolHandler(c *check.C) {
	b := bytes.NewBufferString(`{"pool": "pool1"}`)
	req, err := http.NewRequest("POST", "/pool", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	defer provision.RemovePool("pool1")
	err = addPoolHandler(rec, req, nil)
	c.Assert(err, check.IsNil)
	pools, err := provision.ListPools(nil)
	c.Assert(err, check.IsNil)
	c.Assert(len(pools), check.Equals, 1)
}

func (s *S) TestRemovePoolHandler(c *check.C) {
	err := provision.AddPool("pool1")
	c.Assert(err, check.IsNil)
	b := bytes.NewBufferString(`{"pool": "pool1"}`)
	req, err := http.NewRequest("DELETE", "/pool", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = removePoolHandler(rec, req, nil)
	c.Assert(err, check.IsNil)
	p, err := provision.ListPools(nil)
	c.Assert(err, check.IsNil)
	c.Assert(len(p), check.Equals, 0)
}

func (s *S) TestListPoolsHandler(c *check.C) {
	pool := provision.Pool{Name: "pool1", Teams: []string{"tsuruteam", "ateam"}}
	err := provision.AddPool(pool.Name)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool(pool.Name, pool.Teams)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool(pool.Name)
	poolsExpected := []provision.Pool{pool}
	req, err := http.NewRequest("GET", "/pool", nil)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = listPoolHandler(rec, req, nil)
	c.Assert(err, check.IsNil)
	var pools []provision.Pool
	err = json.NewDecoder(rec.Body).Decode(&pools)
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.DeepEquals, poolsExpected)
}

func (s *S) TestAddTeamsToPoolHandler(c *check.C) {
	pool := provision.Pool{Name: "pool1"}
	err := provision.AddPool(pool.Name)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool(pool.Name)
	b := bytes.NewBufferString(`{"pool": "pool1", "teams": ["test"]}`)
	req, err := http.NewRequest("POST", "/pool/team", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = addTeamToPoolHandler(rec, req, nil)
	c.Assert(err, check.IsNil)
	p, err := provision.ListPools(nil)
	c.Assert(err, check.IsNil)
	c.Assert(p[0].Teams, check.DeepEquals, []string{"test"})
}

func (s *S) TestRemoveTeamsToPoolHandler(c *check.C) {
	pool := provision.Pool{Name: "pool1", Teams: []string{"test"}}
	err := provision.AddPool(pool.Name)
	c.Assert(err, check.IsNil)
	err = provision.AddTeamsToPool(pool.Name, pool.Teams)
	c.Assert(err, check.IsNil)
	defer provision.RemovePool(pool.Name)
	b := bytes.NewBufferString(`{"pool": "pool1", "teams": ["test"]}`)
	req, err := http.NewRequest("DELETE", "/pool/team", b)
	c.Assert(err, check.IsNil)
	rec := httptest.NewRecorder()
	err = removeTeamToPoolHandler(rec, req, nil)
	c.Assert(err, check.IsNil)
	p, err := provision.ListPools(nil)
	c.Assert(err, check.IsNil)
	c.Assert(p[0].Teams, check.DeepEquals, []string{})
}
