// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pool

import (
	"reflect"

	"github.com/tsuru/tsuru/provision/pool"
	check "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestSetPoolConstraints(c *check.C) {
	coll := s.storage.PoolsConstraints()
	err := SetPoolConstraint(&PoolConstraint{PoolExpr: "*", Type: ConstraintTypeRouter, Values: []string{"planb", "hipache"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "*", Type: ConstraintTypeTeam, Values: []string{"user"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	var cs []*PoolConstraint
	err = coll.Find(bson.M{"poolexpr": "*"}).All(&cs)
	c.Assert(err, check.IsNil)
	c.Assert(cs, check.DeepEquals, []*PoolConstraint{
		{PoolExpr: "*", Type: ConstraintTypeRouter, Values: []string{"planb", "hipache"}},
		{PoolExpr: "*", Type: ConstraintTypeTeam, Values: []string{"user"}, Blacklist: true},
	})
}

func (s *S) TestSetPoolConstraintsWithServices(c *check.C) {
	coll := s.storage.PoolsConstraints()
	err := SetPoolConstraint(&PoolConstraint{
		PoolExpr: "prod",
		Type:     pool.ConstraintTypeService,
		Values:   []string{"lux"},
	})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{
		PoolExpr: "dev",
		Type:     pool.ConstraintTypeService,
		Values:   []string{"demacia"},
	})
	c.Assert(err, check.IsNil)
	var cs []*PoolConstraint
	err = coll.Find(bson.M{"field": "service"}).All(&cs)
	c.Assert(err, check.IsNil)
	c.Assert(cs, check.DeepEquals, []*PoolConstraint{
		{PoolExpr: "prod", Type: ConstraintTypeService, Values: []string{"lux"}},
		{PoolExpr: "dev", Type: ConstraintTypeService, Values: []string{"demacia"}},
	})
}

func (s *S) TestSetPoolConstraintsRemoveEmpty(c *check.C) {
	coll := s.storage.PoolsConstraints()
	err := SetPoolConstraint(&PoolConstraint{PoolExpr: "*", Type: ConstraintTypeRouter, Values: []string{"planb", "hipache"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "*", Type: ConstraintTypeTeam, Values: []string{"user"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "*", Type: ConstraintTypeTeam, Values: []string{""}})
	c.Assert(err, check.IsNil)
	var cs []*PoolConstraint
	err = coll.Find(bson.M{"poolexpr": "*"}).All(&cs)
	c.Assert(err, check.IsNil)
	c.Assert(cs, check.DeepEquals, []*PoolConstraint{
		{PoolExpr: "*", Type: ConstraintTypeRouter, Values: []string{"planb", "hipache"}},
	})
}

func (s *S) TestSetPoolConstraintInvalidConstraintType(c *check.C) {
	coll := s.storage.PoolsConstraints()
	err := SetPoolConstraint(&PoolConstraint{PoolExpr: "*", Type: "invalid", Values: []string{"abc"}, Blacklist: true})
	c.Assert(err, check.Equals, ErrInvalidConstraintType)
	var cs []*PoolConstraint
	err = coll.Find(bson.M{"poolexpr": "*"}).All(&cs)
	c.Assert(err, check.IsNil)
	c.Assert(len(cs), check.Equals, 0)
}

func (s *S) TestGetConstraintsForPool(c *check.C) {
	err := SetPoolConstraint(&PoolConstraint{PoolExpr: "*", Type: ConstraintTypeRouter, Values: []string{"planb"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "pp", Type: ConstraintTypeRouter, Values: []string{"galeb"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "*_dev", Type: ConstraintTypeRouter, Values: []string{"planb_dev"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "*_dev", Type: ConstraintTypeTeam, Values: []string{"team_pool1"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "pool1_dev", Type: ConstraintTypeTeam, Values: []string{"team_pool1"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "pool1\\x", Type: ConstraintTypeTeam, Values: []string{"team_pool1x"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "*\\xdev", Type: ConstraintTypeTeam, Values: []string{"team_xdev"}})
	c.Assert(err, check.IsNil)
	tt := []struct {
		pool     string
		expected map[poolConstraintType]*PoolConstraint
	}{
		{pool: "prod", expected: map[poolConstraintType]*PoolConstraint{
			ConstraintTypeRouter: {PoolExpr: "*", Type: ConstraintTypeRouter, Values: []string{"planb"}},
		}},
		{pool: "pp", expected: map[poolConstraintType]*PoolConstraint{
			ConstraintTypeRouter: {PoolExpr: "pp", Type: ConstraintTypeRouter, Values: []string{"galeb"}},
		}},
		{pool: "pool1_dev", expected: map[poolConstraintType]*PoolConstraint{
			ConstraintTypeRouter: {PoolExpr: "*_dev", Type: ConstraintTypeRouter, Values: []string{"planb_dev"}},
			ConstraintTypeTeam:   {PoolExpr: "pool1_dev", Type: ConstraintTypeTeam, Values: []string{"team_pool1"}},
		}},
		{pool: "pool2_dev", expected: map[poolConstraintType]*PoolConstraint{
			ConstraintTypeRouter: {PoolExpr: "*_dev", Type: ConstraintTypeRouter, Values: []string{"planb_dev"}},
			ConstraintTypeTeam:   {PoolExpr: "*_dev", Type: ConstraintTypeTeam, Values: []string{"team_pool1"}, Blacklist: true},
		}},
		{pool: "pp2", expected: map[poolConstraintType]*PoolConstraint{
			ConstraintTypeRouter: {PoolExpr: "*", Type: ConstraintTypeRouter, Values: []string{"planb"}},
		}},
		{pool: "pool1\\x", expected: map[poolConstraintType]*PoolConstraint{
			ConstraintTypeRouter: {PoolExpr: "*", Type: ConstraintTypeRouter, Values: []string{"planb"}},
			ConstraintTypeTeam:   {PoolExpr: "pool1\\x", Type: ConstraintTypeTeam, Values: []string{"team_pool1x"}},
		}},
		{pool: "abc\\xdev", expected: map[poolConstraintType]*PoolConstraint{
			ConstraintTypeRouter: {PoolExpr: "*", Type: ConstraintTypeRouter, Values: []string{"planb"}},
			ConstraintTypeTeam:   {PoolExpr: "*\\xdev", Type: ConstraintTypeTeam, Values: []string{"team_xdev"}},
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
	err := SetPoolConstraint(&PoolConstraint{PoolExpr: "*", Type: ConstraintTypeRouter, Values: []string{"planb"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	err = AppendPoolConstraint(&PoolConstraint{PoolExpr: "*", Type: ConstraintTypeRouter, Values: []string{"galeb"}})
	c.Assert(err, check.IsNil)
	err = AppendPoolConstraint(&PoolConstraint{PoolExpr: "*", Type: ConstraintTypeService, Values: []string{"autoscale"}})
	c.Assert(err, check.IsNil)
	err = AppendPoolConstraint(&PoolConstraint{PoolExpr: "*", Type: "invalid", Values: []string{"val"}})
	c.Assert(err, check.Equals, ErrInvalidConstraintType)
	constraints, err := getConstraintsForPool("*")
	c.Assert(err, check.IsNil)
	c.Assert(constraints, check.DeepEquals, map[poolConstraintType]*PoolConstraint{
		ConstraintTypeRouter:  {Type: ConstraintTypeRouter, PoolExpr: "*", Values: []string{"planb", "galeb"}, Blacklist: true},
		ConstraintTypeService: {Type: ConstraintTypeService, PoolExpr: "*", Values: []string{"autoscale"}},
	})
}

func (s *S) TestAppendPoolConstraintNewConstraint(c *check.C) {
	err := AppendPoolConstraint(&PoolConstraint{PoolExpr: "myPool", Type: ConstraintTypeRouter, Values: []string{"galeb"}})
	c.Assert(err, check.IsNil)
	constraints, err := getConstraintsForPool("myPool")
	c.Assert(err, check.IsNil)
	c.Assert(constraints, check.DeepEquals, map[poolConstraintType]*PoolConstraint{
		ConstraintTypeRouter: {Type: ConstraintTypeRouter, PoolExpr: "myPool", Values: []string{"galeb"}},
	})
}
