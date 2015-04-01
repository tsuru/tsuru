// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"github.com/tsuru/tsuru/db"
	"gopkg.in/check.v1"
)

type S struct {
	storage *db.Storage
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	var err error
	s.storage, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) TestAddPool(c *check.C) {
	coll := s.storage.Collection(poolCollection)
	defer coll.RemoveId("pool1")
	err := AddPool("pool1")
	c.Assert(err, check.IsNil)
}

func (s *S) TestAddPoolWithoutNameShouldBreak(c *check.C) {
	err := AddPool("")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Pool name is required.")
}

func (s *S) TestRemovePool(c *check.C) {
	coll := s.storage.Collection(poolCollection)
	pool := Pool{Name: "pool1"}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	err = RemovePool("pool1")
	c.Assert(err, check.IsNil)
	p, err := coll.FindId("pool1").Count()
	c.Assert(err, check.IsNil)
	c.Assert(p, check.Equals, 0)
}

func (s *S) TestAddTeamToPool(c *check.C) {
	coll := s.storage.Collection(poolCollection)
	pool := Pool{Name: "pool1"}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = AddTeamsToPool("pool1", []string{"ateam", "test"})
	c.Assert(err, check.IsNil)
	var p Pool
	err = coll.FindId(pool.Name).One(&p)
	c.Assert(err, check.IsNil)
	c.Assert(p.Teams, check.DeepEquals, []string{"ateam", "test"})
}

func (s *S) TestAddTeamToPollWithTeams(c *check.C) {
	coll := s.storage.Collection(poolCollection)
	pool := Pool{Name: "pool1", Teams: []string{"test", "ateam"}}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = AddTeamsToPool(pool.Name, []string{"pteam"})
	c.Assert(err, check.IsNil)
	var p Pool
	err = coll.FindId(pool.Name).One(&p)
	c.Assert(err, check.IsNil)
	c.Assert(p.Teams, check.DeepEquals, []string{"test", "ateam", "pteam"})
}

func (s *S) TestAddTeamToPollShouldNotAcceptDuplicatedTeam(c *check.C) {
	coll := s.storage.Collection(poolCollection)
	pool := Pool{Name: "pool1", Teams: []string{"test", "ateam"}}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = AddTeamsToPool(pool.Name, []string{"ateam"})
	c.Assert(err, check.NotNil)
	var p Pool
	err = coll.FindId(pool.Name).One(&p)
	c.Assert(err, check.IsNil)
	c.Assert(p.Teams, check.DeepEquals, []string{"test", "ateam"})
}

func (s *S) TestRemoveTeamsFromPool(c *check.C) {
	coll := s.storage.Collection(poolCollection)
	pool := Pool{Name: "pool1", Teams: []string{"test", "ateam"}}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = RemoveTeamsFromPool(pool.Name, []string{"test"})
	c.Assert(err, check.IsNil)
	var p Pool
	err = coll.FindId(pool.Name).One(&p)
	c.Assert(err, check.IsNil)
	c.Assert(p.Teams, check.DeepEquals, []string{"ateam"})
}
