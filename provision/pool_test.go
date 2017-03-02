// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"reflect"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
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
	err = auth.CreateTeam("ateam", &auth.User{})
	c.Assert(err, check.IsNil)
	err = auth.CreateTeam("test", &auth.User{})
	c.Assert(err, check.IsNil)
	err = auth.CreateTeam("pteam", &auth.User{})
	c.Assert(err, check.IsNil)
}

func (s *S) TestAddPool(c *check.C) {
	coll := s.storage.Pools()
	defer coll.RemoveId("pool1")
	opts := AddPoolOptions{
		Name:    "pool1",
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
	constraints, err := getConstraintsForPool("pool1", "team")
	c.Assert(err, check.IsNil)
	c.Assert(constraints["team"].AllowsAll(), check.Equals, false)
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
	constraints, err := getConstraintsForPool("pool1", "team")
	c.Assert(err, check.IsNil)
	c.Assert(constraints["team"].AllowsAll(), check.Equals, true)
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
	err := AddPool(AddPoolOptions{Name: "nonteams", Public: false, Default: true})
	c.Assert(err, check.IsNil)
	err = AddTeamsToPool("nonteams", []string{"ateam"})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, ErrPublicDefaultPoolCantHaveTeams)
}

func (s *S) TestDefaultPoolShouldBeUnique(c *check.C) {
	err := AddPool(AddPoolOptions{Name: "nonteams", Public: false, Default: true})
	c.Assert(err, check.IsNil)
	err = AddPool(AddPoolOptions{Name: "pool1", Public: false, Default: true})
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
	teams, err := p.GetTeams()
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.DeepEquals, []string{"ateam", "test"})
}

func (s *S) TestAddTeamToPoolWithTeams(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1"}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = AddTeamsToPool(pool.Name, []string{"test", "ateam"})
	c.Assert(err, check.IsNil)
	err = AddTeamsToPool(pool.Name, []string{"pteam"})
	c.Assert(err, check.IsNil)
	teams, err := pool.GetTeams()
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.DeepEquals, []string{"ateam", "test", "pteam"})
}

func (s *S) TestAddTeamToPollShouldNotAcceptDuplicatedTeam(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1"}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = AddTeamsToPool(pool.Name, []string{"test", "ateam"})
	c.Assert(err, check.IsNil)
	err = AddTeamsToPool(pool.Name, []string{"ateam"})
	c.Assert(err, check.NotNil)
	teams, err := pool.GetTeams()
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.DeepEquals, []string{"ateam", "test"})
}

func (s *S) TestAddTeamsToAPublicPool(c *check.C) {
	err := AddPool(AddPoolOptions{Name: "nonteams", Public: true})
	c.Assert(err, check.IsNil)
	err = AddTeamsToPool("nonteams", []string{"ateam"})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, ErrPublicDefaultPoolCantHaveTeams)
}

func (s *S) TestAddTeamsToPoolWithBlacklistShouldFail(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1"}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = SetPoolConstraints("pool1", "team!=myteam")
	c.Assert(err, check.IsNil)
	err = AddTeamsToPool("pool1", []string{"otherteam"})
	c.Assert(err, check.NotNil)
	constraint, err := getExactConstraintForPool("pool1", "team")
	c.Assert(err, check.IsNil)
	c.Assert(constraint.WhiteList, check.Equals, false)
	c.Assert(constraint.Values, check.DeepEquals, []string{"myteam"})
}

func (s *S) TestRemoveTeamsFromPoolNotFound(c *check.C) {
	err := RemoveTeamsFromPool("notfound", []string{"test"})
	c.Assert(err, check.Equals, ErrPoolNotFound)
}

func (s *S) TestRemoveTeamsFromPool(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1"}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = AddTeamsToPool(pool.Name, []string{"test", "ateam"})
	c.Assert(err, check.IsNil)
	teams, err := pool.GetTeams()
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.DeepEquals, []string{"ateam", "test"})
	err = RemoveTeamsFromPool(pool.Name, []string{"test"})
	c.Assert(err, check.IsNil)
	teams, err = pool.GetTeams()
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.DeepEquals, []string{"ateam"})
}

func (s *S) TestRemoveTeamsFromPoolWithBlacklistShouldFail(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1"}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	err = SetPoolConstraints("pool1", "team!=myteam")
	c.Assert(err, check.IsNil)
	err = RemoveTeamsFromPool("pool1", []string{"myteam"})
	c.Assert(err, check.NotNil)
	constraint, err := getExactConstraintForPool("pool1", "team")
	c.Assert(err, check.IsNil)
	c.Assert(constraint.WhiteList, check.Equals, false)
	c.Assert(constraint.Values, check.DeepEquals, []string{"myteam"})
}

func boolPtr(v bool) *bool {
	return &v
}

func (s *S) TestPoolUpdateNotFound(c *check.C) {
	err := PoolUpdate("notfound", UpdatePoolOptions{Public: boolPtr(true)})
	c.Assert(err, check.Equals, ErrPoolNotFound)
}

func (s *S) TestPoolUpdate(c *check.C) {
	opts := AddPoolOptions{
		Name:   "pool1",
		Public: false,
	}
	err := AddPool(opts)
	c.Assert(err, check.IsNil)
	err = PoolUpdate("pool1", UpdatePoolOptions{Public: boolPtr(true)})
	c.Assert(err, check.IsNil)
	constraint, err := getExactConstraintForPool("pool1", "team")
	c.Assert(err, check.IsNil)
	c.Assert(constraint.AllowsAll(), check.Equals, true)
}

func (s *S) TestPoolUpdateToDefault(c *check.C) {
	opts := AddPoolOptions{
		Name:    "pool1",
		Public:  false,
		Default: false,
	}
	err := AddPool(opts)
	c.Assert(err, check.IsNil)
	err = PoolUpdate("pool1", UpdatePoolOptions{Public: boolPtr(true), Default: boolPtr(true)})
	c.Assert(err, check.IsNil)
	p, err := GetPoolByName("pool1")
	c.Assert(err, check.IsNil)
	c.Assert(p.Default, check.Equals, true)
}

func (s *S) TestPoolUpdateForceToDefault(c *check.C) {
	err := AddPool(AddPoolOptions{Name: "pool1", Public: false, Default: true})
	c.Assert(err, check.IsNil)
	err = AddPool(AddPoolOptions{Name: "pool2", Public: false, Default: false})
	c.Assert(err, check.IsNil)
	err = PoolUpdate("pool2", UpdatePoolOptions{Public: boolPtr(true), Default: boolPtr(true), Force: true})
	c.Assert(err, check.IsNil)
	p, err := GetPoolByName("pool2")
	c.Assert(err, check.IsNil)
	c.Assert(p.Default, check.Equals, true)
}

func (s *S) TestPoolUpdateDefaultAttrFailIfDefaultPoolAlreadyExists(c *check.C) {
	err := AddPool(AddPoolOptions{Name: "pool1", Public: false, Default: true})
	c.Assert(err, check.IsNil)
	err = AddPool(AddPoolOptions{Name: "pool2", Public: false, Default: false})
	c.Assert(err, check.IsNil)
	err = PoolUpdate("pool2", UpdatePoolOptions{Public: boolPtr(true), Default: boolPtr(true)})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.Equals, ErrDefaultPoolAlreadyExists)
}

func (s *S) TestPoolUpdateDontHaveSideEffects(c *check.C) {
	err := AddPool(AddPoolOptions{Name: "pool1", Public: false, Default: true})
	c.Assert(err, check.IsNil)
	err = PoolUpdate("pool1", UpdatePoolOptions{Public: boolPtr(true)})
	c.Assert(err, check.IsNil)
	p, err := GetPoolByName("pool1")
	c.Assert(err, check.IsNil)
	c.Assert(p.Default, check.Equals, true)
	constraint, err := getExactConstraintForPool("pool1", "team")
	c.Assert(err, check.IsNil)
	c.Assert(constraint.AllowsAll(), check.Equals, true)
}

func (s *S) TestListPoolAll(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1", Default: true}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	pools, err := ListPossiblePools(nil)
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.HasLen, 1)
}

func (s *S) TestListPoolByQuery(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1", Default: true}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	pool2 := Pool{Name: "pool2", Default: true}
	err = coll.Insert(pool2)
	c.Assert(err, check.IsNil)
	defer coll.RemoveId(pool.Name)
	defer coll.RemoveId(pool2.Name)
	pools, err := listPools(bson.M{"_id": "pool2"})
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.HasLen, 1)
	c.Assert(pools[0].Name, check.Equals, "pool2")
}

func (s *S) TestListPoolEmpty(c *check.C) {
	pools, err := ListPossiblePools(nil)
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.HasLen, 0)
}

func (s *S) TestGetPoolByName(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1", Default: true}
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

func (s *S) TestSetPoolConstraints(c *check.C) {
	coll := s.storage.PoolsConstraints()
	err := SetPoolConstraints("*", "router=planb,hipache", "team!=user")
	c.Assert(err, check.IsNil)
	var cs []*PoolConstraint
	err = coll.Find(bson.M{"poolexpr": "*"}).All(&cs)
	c.Assert(err, check.IsNil)
	c.Assert(cs, check.DeepEquals, []*PoolConstraint{
		{PoolExpr: "*", Field: "router", Values: []string{"planb", "hipache"}, WhiteList: true},
		{PoolExpr: "*", Field: "team", Values: []string{"user"}, WhiteList: false},
	})
}

func (s *S) TestSetPoolConstraintsRemoveEmpty(c *check.C) {
	coll := s.storage.PoolsConstraints()
	err := SetPoolConstraints("*", "router=planb,hipache", "team!=user")
	c.Assert(err, check.IsNil)
	err = SetPoolConstraints("*", "team=")
	c.Assert(err, check.IsNil)
	var cs []*PoolConstraint
	err = coll.Find(bson.M{"poolexpr": "*"}).All(&cs)
	c.Assert(err, check.IsNil)
	c.Assert(cs, check.DeepEquals, []*PoolConstraint{
		{PoolExpr: "*", Field: "router", Values: []string{"planb", "hipache"}, WhiteList: true},
	})
}

func (s *S) TestGetConstraintsForPool(c *check.C) {
	err := SetPoolConstraints("*", "router=planb")
	c.Assert(err, check.IsNil)
	err = SetPoolConstraints("pp", "router=galeb")
	c.Assert(err, check.IsNil)
	err = SetPoolConstraints("*_dev", "router=planb_dev", "team!=team_pool1")
	c.Assert(err, check.IsNil)
	err = SetPoolConstraints("pool1_dev", "team=team_pool1")
	c.Assert(err, check.IsNil)
	tt := []struct {
		pool     string
		expected map[string]*PoolConstraint
	}{
		{pool: "prod", expected: map[string]*PoolConstraint{
			"router": &PoolConstraint{PoolExpr: "*", Field: "router", Values: []string{"planb"}, WhiteList: true},
		}},
		{pool: "pp", expected: map[string]*PoolConstraint{
			"router": &PoolConstraint{PoolExpr: "pp", Field: "router", Values: []string{"galeb"}, WhiteList: true},
		}},
		{pool: "pool1_dev", expected: map[string]*PoolConstraint{
			"router": &PoolConstraint{PoolExpr: "*_dev", Field: "router", Values: []string{"planb_dev"}, WhiteList: true},
			"team":   &PoolConstraint{PoolExpr: "pool1_dev", Field: "team", Values: []string{"team_pool1"}, WhiteList: true},
		}},
		{pool: "pool2_dev", expected: map[string]*PoolConstraint{
			"router": &PoolConstraint{PoolExpr: "*_dev", Field: "router", Values: []string{"planb_dev"}, WhiteList: true},
			"team":   &PoolConstraint{PoolExpr: "*_dev", Field: "team", Values: []string{"team_pool1"}, WhiteList: false},
		}},
	}
	for i, t := range tt {
		constraints, err := getConstraintsForPool(t.pool)
		c.Check(err, check.IsNil)
		if !reflect.DeepEqual(constraints, t.expected) {
			c.Fatalf("(%d) Expected %#+v for pool %q. Got %#+v.", i, t.expected, t.pool, constraints)
		}
	}
}

func (s *S) TestAppendPoolConstraint(c *check.C) {
	err := SetPoolConstraints("*", "router!=planb")
	c.Assert(err, check.IsNil)
	err = appendPoolConstraint("*", "router", "galeb")
	c.Assert(err, check.IsNil)
	constraints, err := getConstraintsForPool("*")
	c.Assert(err, check.IsNil)
	c.Assert(constraints, check.DeepEquals, map[string]*PoolConstraint{
		"router": &PoolConstraint{Field: "router", PoolExpr: "*", Values: []string{"planb", "galeb"}, WhiteList: false},
	})
}

func (s *S) TestAppendPoolConstraintNewConstraint(c *check.C) {
	err := appendPoolConstraint("myPool", "router", "galeb")
	c.Assert(err, check.IsNil)
	constraints, err := getConstraintsForPool("myPool")
	c.Assert(err, check.IsNil)
	c.Assert(constraints, check.DeepEquals, map[string]*PoolConstraint{
		"router": &PoolConstraint{Field: "router", PoolExpr: "myPool", Values: []string{"galeb"}, WhiteList: true},
	})
}

func (s *S) TestPoolAllowedValues(c *check.C) {
	config.Set("routers:router1:type", "hipache")
	config.Set("routers:router2:type", "hipache")
	config.Set("routers:router3:type", "hipache")
	defer config.Unset("routers")
	err := auth.CreateTeam("pubteam", &auth.User{})
	c.Assert(err, check.IsNil)
	err = auth.CreateTeam("team1", &auth.User{})
	c.Assert(err, check.IsNil)
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1"}
	err = coll.Insert(pool)
	c.Assert(err, check.IsNil)
	err = SetPoolConstraints("pool*", "team=pubteam", "router!=router2")
	c.Assert(err, check.IsNil)
	err = SetPoolConstraints("pool1", "team=team1")
	c.Assert(err, check.IsNil)
	constraints, err := pool.AllowedValues()
	c.Assert(err, check.IsNil)
	c.Assert(constraints, check.DeepEquals, map[string][]string{
		"team":   {"team1"},
		"router": {"router1", "router3"},
	})
}
