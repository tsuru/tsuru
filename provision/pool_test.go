// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type S struct {
	storage *db.Storage
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "provision_tests_s")
	var err error
	s.storage, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	s.storage.Apps().Database.DropDatabase()
	s.storage.Close()
}

func (s *S) SetUpTest(c *check.C) {
	err := dbtest.ClearAllCollections(s.storage.Apps().Database)
	c.Assert(err, check.IsNil)
}

func (s *S) TestAddPool(c *check.C) {
	coll := s.storage.Pools()
	defer coll.RemoveId("pool1")
	opts := AddPoolOptions{
		Name:    "pool1",
		Public:  false,
		Default: false,
	}
	err := AddPool(opts)
	c.Assert(err, check.IsNil)
}

func (s *S) TestAddNonPublicPool(c *check.C) {
	coll := s.storage.Pools()
	defer coll.RemoveId("pool1")
	opts := AddPoolOptions{
		Name:    "pool1",
		Public:  false,
		Default: false,
	}
	err := AddPool(opts)
	c.Assert(err, check.IsNil)
	var p Pool
	err = coll.Find(bson.M{"_id": "pool1"}).One(&p)
	c.Assert(err, check.IsNil)
	c.Assert(p.Public, check.Equals, false)
}

func (s *S) TestAddPublicPool(c *check.C) {
	coll := s.storage.Pools()
	defer coll.RemoveId("pool1")
	opts := AddPoolOptions{
		Name:    "pool1",
		Public:  true,
		Default: false,
	}
	err := AddPool(opts)
	c.Assert(err, check.IsNil)
	var p Pool
	err = coll.Find(bson.M{"_id": "pool1"}).One(&p)
	c.Assert(err, check.IsNil)
	c.Assert(p.Public, check.Equals, true)
}

func (s *S) TestAddPoolWithoutNameShouldBreak(c *check.C) {
	opts := AddPoolOptions{
		Name:    "",
		Public:  false,
		Default: false,
	}
	err := AddPool(opts)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Pool name is required.")
}

func (s *S) TestAddDefaultPool(c *check.C) {
	opts := AddPoolOptions{
		Name:    "pool1",
		Public:  false,
		Default: true,
	}
	err := AddPool(opts)
	defer RemovePool("pool1")
	c.Assert(err, check.IsNil)
}

func (s *S) TestAddTeamToPoolNotFound(c *check.C) {
	err := AddTeamsToPool("notfound", []string{"ateam"})
	c.Assert(err, check.Equals, ErrPoolNotFound)
}

func (s *S) TestDefaultPoolCantHaveTeam(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "nonteams", Public: false, Default: true}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = AddTeamsToPool(pool.Name, []string{"ateam"})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, ErrPublicDefaultPollCantHaveTeams)
}

func (s *S) TestDefaultPoolShouldBeUnique(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "nonteams", Public: false, Default: true}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	opts := AddPoolOptions{
		Name:    "pool1",
		Public:  false,
		Default: true,
	}
	err = AddPool(opts)
	defer RemovePool("pool1")
	c.Assert(err, check.NotNil)
}

func (s *S) TestForceAddDefaultPool(c *check.C) {
	coll := s.storage.Pools()
	opts := AddPoolOptions{
		Name:    "pool1",
		Public:  false,
		Default: true,
	}
	err := AddPool(opts)
	defer RemovePool("pool1")
	c.Assert(err, check.IsNil)
	opts = AddPoolOptions{
		Name:    "pool2",
		Public:  false,
		Default: true,
		Force:   true,
	}
	err = AddPool(opts)
	defer RemovePool("pool2")
	c.Assert(err, check.IsNil)
	var p Pool
	err = coll.Find(bson.M{"_id": "pool1"}).One(&p)
	c.Assert(err, check.IsNil)
	c.Assert(p.Default, check.Equals, false)
	err = coll.Find(bson.M{"_id": "pool2"}).One(&p)
	c.Assert(err, check.IsNil)
	c.Assert(p.Default, check.Equals, true)
}

func (s *S) TestRemovePoolNotFound(c *check.C) {
	err := RemovePool("notfound")
	c.Assert(err, check.Equals, ErrPoolNotFound)
}

func (s *S) TestRemovePool(c *check.C) {
	coll := s.storage.Pools()
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
	coll := s.storage.Pools()
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
	coll := s.storage.Pools()
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
	coll := s.storage.Pools()
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

func (s *S) TestAddTeamsToAPublicPool(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "nonteams", Public: true}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = AddTeamsToPool(pool.Name, []string{"ateam"})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, ErrPublicDefaultPollCantHaveTeams)
}

func (s *S) TestRemoveTeamsFromPoolNotFound(c *check.C) {
	err := RemoveTeamsFromPool("notfound", []string{"test"})
	c.Assert(err, check.Equals, ErrPoolNotFound)
}

func (s *S) TestRemoveTeamsFromPool(c *check.C) {
	coll := s.storage.Pools()
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

func boolPtr(v bool) *bool {
	return &v
}

func (s *S) TestPoolUpdateNotFound(c *check.C) {
	err := PoolUpdate("notfound", UpdatePoolOptions{Public: boolPtr(true)})
	c.Assert(err, check.Equals, ErrPoolNotFound)
}

func (s *S) TestPoolUpdate(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1", Public: false}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = PoolUpdate("pool1", UpdatePoolOptions{Public: boolPtr(true)})
	c.Assert(err, check.IsNil)
	var p Pool
	err = coll.Find(bson.M{"_id": pool.Name}).One(&p)
	c.Assert(err, check.IsNil)
	c.Assert(p.Public, check.Equals, true)
}

func (s *S) TestPoolUpdateToDefault(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1", Public: false, Default: false}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = PoolUpdate("pool1", UpdatePoolOptions{Public: boolPtr(true), Default: boolPtr(true)})
	c.Assert(err, check.IsNil)
	var p Pool
	err = coll.Find(bson.M{"_id": pool.Name}).One(&p)
	c.Assert(err, check.IsNil)
	c.Assert(p.Default, check.Equals, true)
}

func (s *S) TestPoolUpdateForceToDefault(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1", Public: false, Default: true}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	pool = Pool{Name: "pool2", Public: false, Default: false}
	err = coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = PoolUpdate("pool2", UpdatePoolOptions{Public: boolPtr(true), Default: boolPtr(true), Force: true})
	c.Assert(err, check.IsNil)
	var p Pool
	err = coll.Find(bson.M{"_id": "pool2"}).One(&p)
	c.Assert(err, check.IsNil)
	c.Assert(p.Default, check.Equals, true)
}

func (s *S) TestPoolUpdateDefaultAttrFailIfDefaultPoolAlreadyExists(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1", Public: false, Default: true}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	pool = Pool{Name: "pool2", Public: false, Default: false}
	err = coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = PoolUpdate("pool2", UpdatePoolOptions{Public: boolPtr(true), Default: boolPtr(true)})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, ErrDefaultPoolAlreadyExists)
}

func (s *S) TestPoolUpdateDontHaveSideEffects(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1", Public: false, Default: true}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = PoolUpdate("pool1", UpdatePoolOptions{Public: boolPtr(true)})
	c.Assert(err, check.IsNil)
	var p Pool
	err = coll.Find(bson.M{"_id": pool.Name}).One(&p)
	c.Assert(err, check.IsNil)
	c.Assert(p.Public, check.Equals, true)
	c.Assert(p.Default, check.Equals, true)
}

func (s *S) TestListPoolAll(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1", Public: false, Default: true}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	pools, err := ListPossiblePools(nil)
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.HasLen, 1)
}

func (s *S) TestListPoolByQuery(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1", Public: false, Default: true}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	pool2 := Pool{Name: "pool2", Public: true, Default: true}
	err = coll.Insert(pool2)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	defer coll.RemoveId(pool2.Name)
	pools, err := listPools(bson.M{"public": true})
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.HasLen, 1)
	c.Assert(pools[0].Public, check.Equals, true)
}

func (s *S) TestListPoolEmpty(c *check.C) {
	pools, err := ListPossiblePools(nil)
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.HasLen, 0)
}

func (s *S) TestGetPoolByName(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1", Public: false, Default: true}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	p, err := GetPoolByName(pool.Name)
	c.Assert(err, check.IsNil)
	c.Assert(p.Name, check.Equals, pool.Name)
	p, err = GetPoolByName("not found")
	c.Assert(p, check.IsNil)
	c.Assert(err, check.NotNil)
}
