// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pool

import (
	"sort"
	"testing"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/service"
	"github.com/tsuru/tsuru/servicemanager"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type S struct {
	storage         *db.Storage
	teams           []authTypes.Team
	mockTeamService *authTypes.MockTeamService
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "pool_tests_s")
	var err error
	s.storage, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	s.storage.Apps().Database.DropDatabase()
	s.storage.Close()
}

func (s *S) SetUpTest(c *check.C) {
	provisiontest.ProvisionerInstance.Reset()
	err := dbtest.ClearAllCollections(s.storage.Apps().Database)
	c.Assert(err, check.IsNil)
	s.teams = []authTypes.Team{{Name: "ateam"}, {Name: "test"}, {Name: "pteam"}}
	s.mockTeamService = &authTypes.MockTeamService{
		OnList: func() ([]authTypes.Team, error) {
			return s.teams, nil
		},
		OnFindByName: func(name string) (*authTypes.Team, error) {
			for _, t := range s.teams {
				if t.Name == name {
					return &t, nil
				}
			}
			return nil, authTypes.ErrTeamNotFound
		},
	}
	servicemanager.Team = s.mockTeamService
}

func (s *S) TestValidateRouters(c *check.C) {
	config.Set("routers:router1:type", "hipache")
	config.Set("routers:router2:type", "hipache")
	defer config.Unset("routers")
	pool := Pool{Name: "pool1"}
	err := SetPoolConstraint(&PoolConstraint{PoolExpr: "pool*", Field: ConstraintTypeRouter, Values: []string{"router2"}, Blacklist: true})
	c.Assert(err, check.IsNil)

	err = pool.ValidateRouters([]appTypes.AppRouter{{Name: "router1"}})
	c.Assert(err, check.IsNil)
	err = pool.ValidateRouters([]appTypes.AppRouter{{Name: "router2"}})
	c.Assert(err, check.NotNil)
	err = pool.ValidateRouters([]appTypes.AppRouter{{Name: "unknown-router"}})
	c.Assert(err, check.NotNil)
	err = pool.ValidateRouters([]appTypes.AppRouter{{Name: "router1"}, {Name: "router2"}})
	c.Assert(err, check.NotNil)
}

func (s *S) TestAddPool(c *check.C) {
	msg := "Invalid pool name, pool name should have at most 63 " +
		"characters, containing only lower case letters, numbers or dashes, " +
		"starting with a letter."
	vErr := &tsuruErrors.ValidationError{Message: msg}
	tt := []struct {
		name        string
		expectedErr error
	}{
		{"pool1", nil},
		{"myPool", vErr},
		{"my pool", vErr},
		{"123mypool", vErr},
		{"", ErrPoolNameIsRequired},
		{"p", nil},
	}
	for _, t := range tt {
		err := AddPool(AddPoolOptions{Name: t.name})
		c.Assert(err, check.DeepEquals, t.expectedErr, check.Commentf("%s", t.name))
		if t.expectedErr == nil {
			pool, err := GetPoolByName(t.name)
			c.Assert(err, check.IsNil, check.Commentf("%s", t.name))
			c.Assert(pool, check.DeepEquals, &Pool{Name: t.name}, check.Commentf("%s", t.name))
		}
	}
}

func (s *S) TestAddNonPublicPool(c *check.C) {
	coll := s.storage.Pools()
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

func (s *S) TestAddPoolNameShouldBeUnique(c *check.C) {
	err := AddPool(AddPoolOptions{Name: "mypool"})
	c.Assert(err, check.IsNil)
	err = AddPool(AddPoolOptions{Name: "mypool"})
	c.Assert(err, check.DeepEquals, ErrPoolAlreadyExists)
}

func (s *S) TestForceAddDefaultPool(c *check.C) {
	coll := s.storage.Pools()
	opts := AddPoolOptions{
		Name:    "pool1",
		Public:  false,
		Default: true,
	}
	err := AddPool(opts)
	c.Assert(err, check.IsNil)
	opts = AddPoolOptions{
		Name:    "pool2",
		Public:  false,
		Default: true,
		Force:   true,
	}
	err = AddPool(opts)
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
	err = AddTeamsToPool("pool1", []string{"ateam", "test"})
	c.Assert(err, check.IsNil)
	var p Pool
	err = coll.FindId(pool.Name).One(&p)
	c.Assert(err, check.IsNil)
	teams, err := p.GetTeams()
	c.Assert(err, check.IsNil)
	sort.Strings(teams)
	c.Assert(teams, check.DeepEquals, []string{"ateam", "test"})
}

func (s *S) TestAddTeamToPoolWithTeams(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1"}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	err = AddTeamsToPool(pool.Name, []string{"test", "ateam"})
	c.Assert(err, check.IsNil)
	err = AddTeamsToPool(pool.Name, []string{"pteam"})
	c.Assert(err, check.IsNil)
	teams, err := pool.GetTeams()
	c.Assert(err, check.IsNil)
	sort.Strings(teams)
	c.Assert(teams, check.DeepEquals, []string{"ateam", "pteam", "test"})
}

func (s *S) TestAddTeamToPollShouldNotAcceptDuplicatedTeam(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1"}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	err = AddTeamsToPool(pool.Name, []string{"test", "ateam"})
	c.Assert(err, check.IsNil)
	err = AddTeamsToPool(pool.Name, []string{"ateam"})
	c.Assert(err, check.NotNil)
	teams, err := pool.GetTeams()
	c.Assert(err, check.IsNil)
	sort.Strings(teams)
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
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "pool1", Field: ConstraintTypeTeam, Values: []string{"myteam"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	err = AddTeamsToPool("pool1", []string{"otherteam"})
	c.Assert(err, check.NotNil)
	constraint, err := getExactConstraintForPool("pool1", "team")
	c.Assert(err, check.IsNil)
	c.Assert(constraint.Blacklist, check.Equals, true)
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
	err = AddTeamsToPool(pool.Name, []string{"test", "ateam"})
	c.Assert(err, check.IsNil)
	teams, err := pool.GetTeams()
	c.Assert(err, check.IsNil)
	sort.Strings(teams)
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
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "pool1", Field: ConstraintTypeTeam, Values: []string{"myteam"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	err = RemoveTeamsFromPool("pool1", []string{"myteam"})
	c.Assert(err, check.NotNil)
	constraint, err := getExactConstraintForPool("pool1", "team")
	c.Assert(err, check.IsNil)
	c.Assert(constraint.Blacklist, check.Equals, true)
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

func (s *S) TestListPool(c *check.C) {
	err := AddPool(AddPoolOptions{Name: "pool1"})
	c.Assert(err, check.IsNil)
	err = AddPool(AddPoolOptions{Name: "pool2", Default: true})
	c.Assert(err, check.IsNil)
	err = AddPool(AddPoolOptions{Name: "pool3"})
	c.Assert(err, check.IsNil)
	pools, err := ListPools("pool1", "pool3")
	c.Assert(err, check.IsNil)
	c.Assert(len(pools), check.Equals, 2)
	c.Assert(pools[0].Name, check.Equals, "pool1")
	c.Assert(pools[1].Name, check.Equals, "pool3")
}

func (s *S) TestListPoolsForTeam(c *check.C) {
	err := AddPool(AddPoolOptions{Name: "pool1"})
	c.Assert(err, check.IsNil)
	err = AddPool(AddPoolOptions{Name: "pool2"})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{
		PoolExpr: "pool1",
		Field:    ConstraintTypeTeam,
		Values:   []string{"team1"},
	})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{
		PoolExpr: "pool2",
		Field:    ConstraintTypeTeam,
		Values:   []string{"team2"},
	})
	c.Assert(err, check.IsNil)
	pools, err := ListPoolsForTeam("team1")
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.HasLen, 1)
}

func (s *S) TestListPossiblePoolsAll(c *check.C) {
	err := AddPool(AddPoolOptions{Name: "pool1", Default: true})
	c.Assert(err, check.IsNil)
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
	p, err := GetPoolByName(pool.Name)
	c.Assert(err, check.IsNil)
	c.Assert(p.Name, check.Equals, pool.Name)
	p, err = GetPoolByName("not found")
	c.Assert(p, check.IsNil)
	c.Assert(err, check.NotNil)
}

func (s *S) TestGetRouters(c *check.C) {
	config.Set("routers:router1:type", "hipache")
	config.Set("routers:router2:type", "hipache")
	defer config.Unset("routers")
	err := AddPool(AddPoolOptions{Name: "pool1"})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "pool*", Field: ConstraintTypeRouter, Values: []string{"router2"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	pool, err := GetPoolByName("pool1")
	c.Assert(err, check.IsNil)
	routers, err := pool.GetRouters()
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []string{"router1"})
	pool.Name = "other"
	routers, err = pool.GetRouters()
	c.Assert(err, check.IsNil)
	c.Assert(routers, check.DeepEquals, []string{"router1", "router2"})
}

func (s *S) TestGetServices(c *check.C) {
	s.mockTeamService.OnFindByNames = func(_ []string) ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: "ateam"}}, nil
	}
	serv := service.Service{Name: "demacia", Password: "pentakill", Endpoint: map[string]string{"production": "http://localhost:1234"}, OwnerTeams: []string{"ateam"}}
	err := serv.Create()
	c.Assert(err, check.IsNil)
	err = AddPool(AddPoolOptions{Name: "pool1"})
	c.Assert(err, check.IsNil)
	pool, err := GetPoolByName("pool1")
	c.Assert(err, check.IsNil)
	services, err := pool.GetServices()
	c.Assert(err, check.IsNil)
	c.Assert(services, check.DeepEquals, []string{"demacia"})
}

func (s *S) TestGetDefaultRouterFromConstraint(c *check.C) {
	config.Set("routers:router1:type", "hipache")
	config.Set("routers:router2:type", "hipache")
	defer config.Unset("routers")
	err := AddPool(AddPoolOptions{Name: "pool1"})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "pool*", Field: ConstraintTypeRouter, Values: []string{"router2"}, Blacklist: false})
	c.Assert(err, check.IsNil)
	pool, err := GetPoolByName("pool1")
	c.Assert(err, check.IsNil)
	r, err := pool.GetDefaultRouter()
	c.Assert(err, check.IsNil)
	c.Assert(r, check.Equals, "router2")
}

func (s *S) TestGetDefaultRouterNoDefault(c *check.C) {
	config.Set("routers:router1:type", "hipache")
	config.Set("routers:router2:type", "hipache")
	defer config.Unset("routers")
	err := AddPool(AddPoolOptions{Name: "pool1"})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "pool*", Field: ConstraintTypeRouter, Values: []string{"*"}, Blacklist: false})
	c.Assert(err, check.IsNil)
	pool, err := GetPoolByName("pool1")
	c.Assert(err, check.IsNil)
	r, err := pool.GetDefaultRouter()
	c.Assert(err, check.Equals, router.ErrDefaultRouterNotFound)
	c.Assert(r, check.Equals, "")
}

func (s *S) TestGetDefaultFallbackFromConfig(c *check.C) {
	config.Set("routers:router1:type", "hipache")
	config.Set("routers:router2:type", "hipache")
	config.Set("routers:router2:default", true)
	defer config.Unset("routers")
	err := AddPool(AddPoolOptions{Name: "pool1"})
	c.Assert(err, check.IsNil)
	pool, err := GetPoolByName("pool1")
	c.Assert(err, check.IsNil)
	r, err := pool.GetDefaultRouter()
	c.Assert(err, check.Equals, nil)
	c.Assert(r, check.Equals, "router2")
}

func (s *S) TestGetDefaultAllowAllSingleAllowedValue(c *check.C) {
	config.Set("routers:router2:type", "hipache")
	defer config.Unset("routers")
	err := AddPool(AddPoolOptions{Name: "pool1"})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "pool*", Field: ConstraintTypeRouter, Values: []string{"router*"}, Blacklist: false})
	c.Assert(err, check.IsNil)
	pool, err := GetPoolByName("pool1")
	c.Assert(err, check.IsNil)
	r, err := pool.GetDefaultRouter()
	c.Assert(err, check.IsNil)
	c.Assert(r, check.Equals, "router2")
}

func (s *S) TestGetDefaultBlacklistSingleAllowedValue(c *check.C) {
	config.Set("routers:router1:type", "hipache")
	config.Set("routers:router2:type", "hipache")
	defer config.Unset("routers")
	err := AddPool(AddPoolOptions{Name: "pool1"})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "pool*", Field: ConstraintTypeRouter, Values: []string{"router2"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	pool, err := GetPoolByName("pool1")
	c.Assert(err, check.IsNil)
	r, err := pool.GetDefaultRouter()
	c.Assert(err, check.IsNil)
	c.Assert(r, check.Equals, "router1")
}

func (s *S) TestPoolAllowedValues(c *check.C) {
	config.Set("routers:router:type", "hipache")
	config.Set("routers:router1:type", "hipache")
	config.Set("routers:router2:type", "hipache")
	defer config.Unset("routers")
	s.teams = append(s.teams, authTypes.Team{Name: "pubteam"}, authTypes.Team{Name: "team1"})
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1"}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "pool*", Field: ConstraintTypeTeam, Values: []string{"pubteam"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "pool*", Field: ConstraintTypeRouter, Values: []string{"router"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "pool1", Field: ConstraintTypeTeam, Values: []string{"team1"}})
	c.Assert(err, check.IsNil)
	constraints, err := pool.allowedValues()
	c.Assert(err, check.IsNil)
	c.Assert(constraints, check.DeepEquals, map[poolConstraintType][]string{
		ConstraintTypeTeam:    {"team1"},
		ConstraintTypeRouter:  {"router1", "router2"},
		ConstraintTypeService: nil,
	})
	pool.Name = "other"
	constraints, err = pool.allowedValues()
	c.Assert(err, check.IsNil)
	c.Assert(constraints, check.HasLen, 3)
	sort.Strings(constraints[ConstraintTypeTeam])
	c.Assert(constraints[ConstraintTypeTeam], check.DeepEquals, []string{
		"ateam", "pteam", "pubteam", "team1", "test",
	})
	sort.Strings(constraints[ConstraintTypeRouter])
	c.Assert(constraints[ConstraintTypeRouter], check.DeepEquals, []string{
		"router", "router1", "router2",
	})
}

func (s *S) TestRenamePoolTeam(c *check.C) {
	coll := s.storage.PoolsConstraints()
	constraints := []PoolConstraint{
		{PoolExpr: "e1", Field: ConstraintTypeRouter, Values: []string{"t1", "t2"}},
		{PoolExpr: "e2", Field: ConstraintTypeTeam, Values: []string{"t1", "t2"}},
		{PoolExpr: "e3", Field: ConstraintTypeTeam, Values: []string{"t2", "t3"}},
	}
	for _, constraint := range constraints {
		err := SetPoolConstraint(&constraint)
		c.Assert(err, check.IsNil)
	}
	err := RenamePoolTeam("t2", "t9000")
	c.Assert(err, check.IsNil)
	var cs []PoolConstraint
	err = coll.Find(nil).Sort("poolexpr").All(&cs)
	c.Assert(err, check.IsNil)
	c.Assert(cs, check.DeepEquals, []PoolConstraint{
		{PoolExpr: "e1", Field: ConstraintTypeRouter, Values: []string{"t1", "t2"}},
		{PoolExpr: "e2", Field: ConstraintTypeTeam, Values: []string{"t1", "t9000"}},
		{PoolExpr: "e3", Field: ConstraintTypeTeam, Values: []string{"t3", "t9000"}},
	})
}

func (s *S) TestGetProvisionerForPool(c *check.C) {
	coll := s.storage.Pools()
	pool := Pool{Name: "pool1", Default: true, Provisioner: "fake"}
	err := coll.Insert(pool)
	c.Assert(err, check.IsNil)
	prov, err := GetProvisionerForPool(pool.Name)
	c.Assert(err, check.IsNil)
	c.Assert(prov.GetName(), check.Equals, "fake")
	c.Assert(poolCache.Get("pool1"), check.Equals, provisiontest.ProvisionerInstance)
	prov, err = GetProvisionerForPool(pool.Name)
	c.Assert(err, check.IsNil)
	c.Assert(prov.GetName(), check.Equals, "fake")
	prov, err = GetProvisionerForPool("not found")
	c.Assert(prov, check.IsNil)
	c.Assert(err, check.Equals, ErrPoolNotFound)
}
