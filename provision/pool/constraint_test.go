// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pool

import (
	"context"
	"reflect"

	"github.com/tsuru/tsuru/db/storagev2"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	check "gopkg.in/check.v1"
)

func (s *S) TestSetPoolConstraints(c *check.C) {
	colection, err := storagev2.PoolConstraintsCollection()
	c.Assert(err, check.IsNil)

	err = SetPoolConstraint(context.TODO(), &PoolConstraint{PoolExpr: "*", Field: ConstraintTypeRouter, Values: []string{"router1", "router2"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(context.TODO(), &PoolConstraint{PoolExpr: "*", Field: ConstraintTypeTeam, Values: []string{"user"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	var cs []*PoolConstraint
	cursor, err := colection.Find(context.TODO(), mongoBSON.M{"poolexpr": "*"})
	c.Assert(err, check.IsNil)
	cursor.All(context.TODO(), &cs)
	c.Assert(err, check.IsNil)
	c.Assert(err, check.IsNil)
	c.Assert(cs, check.DeepEquals, []*PoolConstraint{
		{PoolExpr: "*", Field: ConstraintTypeRouter, Values: []string{"router1", "router2"}},
		{PoolExpr: "*", Field: ConstraintTypeTeam, Values: []string{"user"}, Blacklist: true},
	})
}

func (s *S) TestSetPoolConstraintsWithServices(c *check.C) {
	err := SetPoolConstraint(context.TODO(), &PoolConstraint{
		PoolExpr: "prod",
		Field:    ConstraintTypeService,
		Values:   []string{"lux"},
	})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(context.TODO(), &PoolConstraint{
		PoolExpr: "dev",
		Field:    ConstraintTypeService,
		Values:   []string{"demacia"},
	})
	c.Assert(err, check.IsNil)
	var cs []*PoolConstraint

	colection, err := storagev2.PoolConstraintsCollection()
	c.Assert(err, check.IsNil)

	cursor, err := colection.Find(context.TODO(), mongoBSON.M{"field": "service"})
	c.Assert(err, check.IsNil)

	err = cursor.All(context.TODO(), &cs)
	c.Assert(err, check.IsNil)

	c.Assert(err, check.IsNil)
	c.Assert(cs, check.DeepEquals, []*PoolConstraint{
		{PoolExpr: "prod", Field: ConstraintTypeService, Values: []string{"lux"}},
		{PoolExpr: "dev", Field: ConstraintTypeService, Values: []string{"demacia"}},
	})
}

func (s *S) TestSetPoolConstraintsRemoveEmpty(c *check.C) {
	err := SetPoolConstraint(context.TODO(), &PoolConstraint{PoolExpr: "*", Field: ConstraintTypeRouter, Values: []string{"router1", "router2"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(context.TODO(), &PoolConstraint{PoolExpr: "*", Field: ConstraintTypeTeam, Values: []string{"user"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(context.TODO(), &PoolConstraint{PoolExpr: "*", Field: ConstraintTypeTeam, Values: []string{""}})
	c.Assert(err, check.IsNil)
	var cs []*PoolConstraint
	colection, err := storagev2.PoolConstraintsCollection()
	c.Assert(err, check.IsNil)

	cursor, err := colection.Find(context.TODO(), mongoBSON.M{"poolexpr": "*"})
	c.Assert(err, check.IsNil)

	err = cursor.All(context.TODO(), &cs)
	c.Assert(err, check.IsNil)

	c.Assert(cs, check.DeepEquals, []*PoolConstraint{
		{PoolExpr: "*", Field: ConstraintTypeRouter, Values: []string{"router1", "router2"}},
	})
}

func (s *S) TestSetPoolConstraintInvalidConstraintType(c *check.C) {
	err := SetPoolConstraint(context.TODO(), &PoolConstraint{PoolExpr: "*", Field: "invalid", Values: []string{"abc"}, Blacklist: true})
	c.Assert(err, check.Equals, ErrInvalidConstraintType)
	var cs []*PoolConstraint

	colection, err := storagev2.PoolConstraintsCollection()
	c.Assert(err, check.IsNil)

	cursor, err := colection.Find(context.TODO(), mongoBSON.M{"poolexpr": "*"})
	c.Assert(err, check.IsNil)

	err = cursor.All(context.TODO(), &cs)
	c.Assert(err, check.IsNil)

	c.Assert(len(cs), check.Equals, 0)
}

func (s *S) TestGetConstraintsForPool(c *check.C) {
	err := SetPoolConstraint(context.TODO(), &PoolConstraint{PoolExpr: "*", Field: ConstraintTypeRouter, Values: []string{"router1"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(context.TODO(), &PoolConstraint{PoolExpr: "pp", Field: ConstraintTypeRouter, Values: []string{"router2"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(context.TODO(), &PoolConstraint{PoolExpr: "*_dev", Field: ConstraintTypeRouter, Values: []string{"router1_dev"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(context.TODO(), &PoolConstraint{PoolExpr: "*_dev", Field: ConstraintTypeTeam, Values: []string{"team_pool1"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(context.TODO(), &PoolConstraint{PoolExpr: "pool1_dev", Field: ConstraintTypeTeam, Values: []string{"team_pool1"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(context.TODO(), &PoolConstraint{PoolExpr: "pool1\\x", Field: ConstraintTypeTeam, Values: []string{"team_pool1x"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(context.TODO(), &PoolConstraint{PoolExpr: "*\\xdev", Field: ConstraintTypeTeam, Values: []string{"team_xdev"}})
	c.Assert(err, check.IsNil)
	tt := []struct {
		pool     string
		expected map[PoolConstraintType]*PoolConstraint
	}{
		{pool: "prod", expected: map[PoolConstraintType]*PoolConstraint{
			ConstraintTypeRouter: {PoolExpr: "*", Field: ConstraintTypeRouter, Values: []string{"router1"}},
		}},
		{pool: "pp", expected: map[PoolConstraintType]*PoolConstraint{
			ConstraintTypeRouter: {PoolExpr: "pp", Field: ConstraintTypeRouter, Values: []string{"router2"}},
		}},
		{pool: "pool1_dev", expected: map[PoolConstraintType]*PoolConstraint{
			ConstraintTypeRouter: {PoolExpr: "*_dev", Field: ConstraintTypeRouter, Values: []string{"router1_dev"}},
			ConstraintTypeTeam:   {PoolExpr: "pool1_dev", Field: ConstraintTypeTeam, Values: []string{"team_pool1"}},
		}},
		{pool: "pool2_dev", expected: map[PoolConstraintType]*PoolConstraint{
			ConstraintTypeRouter: {PoolExpr: "*_dev", Field: ConstraintTypeRouter, Values: []string{"router1_dev"}},
			ConstraintTypeTeam:   {PoolExpr: "*_dev", Field: ConstraintTypeTeam, Values: []string{"team_pool1"}, Blacklist: true},
		}},
		{pool: "pp2", expected: map[PoolConstraintType]*PoolConstraint{
			ConstraintTypeRouter: {PoolExpr: "*", Field: ConstraintTypeRouter, Values: []string{"router1"}},
		}},
		{pool: "pool1\\x", expected: map[PoolConstraintType]*PoolConstraint{
			ConstraintTypeRouter: {PoolExpr: "*", Field: ConstraintTypeRouter, Values: []string{"router1"}},
			ConstraintTypeTeam:   {PoolExpr: "pool1\\x", Field: ConstraintTypeTeam, Values: []string{"team_pool1x"}},
		}},
		{pool: "abc\\xdev", expected: map[PoolConstraintType]*PoolConstraint{
			ConstraintTypeRouter: {PoolExpr: "*", Field: ConstraintTypeRouter, Values: []string{"router1"}},
			ConstraintTypeTeam:   {PoolExpr: "*\\xdev", Field: ConstraintTypeTeam, Values: []string{"team_xdev"}},
		}},
	}
	for i, t := range tt {
		constraints, err := getConstraintsForPool(context.TODO(), t.pool)
		c.Check(err, check.IsNil)
		if !reflect.DeepEqual(constraints, t.expected) {
			c.Fatalf("(%d) Expected %#+v for pool %q. Got %#+v.", i, t.expected, t.pool, constraints)
		}
	}
}

func (s *S) TestAppendPoolConstraint(c *check.C) {
	err := SetPoolConstraint(context.TODO(), &PoolConstraint{PoolExpr: "*", Field: ConstraintTypeRouter, Values: []string{"router1"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	err = AppendPoolConstraint(context.TODO(), &PoolConstraint{PoolExpr: "*", Field: ConstraintTypeRouter, Values: []string{"router2"}})
	c.Assert(err, check.IsNil)
	err = AppendPoolConstraint(context.TODO(), &PoolConstraint{PoolExpr: "*", Field: ConstraintTypeService, Values: []string{"autoscale"}})
	c.Assert(err, check.IsNil)
	err = AppendPoolConstraint(context.TODO(), &PoolConstraint{PoolExpr: "*", Field: "invalid", Values: []string{"val"}})
	c.Assert(err, check.Equals, ErrInvalidConstraintType)
	constraints, err := getConstraintsForPool(context.TODO(), "*")
	c.Assert(err, check.IsNil)
	c.Assert(constraints, check.DeepEquals, map[PoolConstraintType]*PoolConstraint{
		ConstraintTypeRouter:  {Field: ConstraintTypeRouter, PoolExpr: "*", Values: []string{"router1", "router2"}, Blacklist: true},
		ConstraintTypeService: {Field: ConstraintTypeService, PoolExpr: "*", Values: []string{"autoscale"}},
	})
}

func (s *S) TestAppendPoolConstraintNewConstraint(c *check.C) {
	err := AppendPoolConstraint(context.TODO(), &PoolConstraint{PoolExpr: "myPool", Field: ConstraintTypeRouter, Values: []string{"router2"}})
	c.Assert(err, check.IsNil)
	constraints, err := getConstraintsForPool(context.TODO(), "myPool")
	c.Assert(err, check.IsNil)
	c.Assert(constraints, check.DeepEquals, map[PoolConstraintType]*PoolConstraint{
		ConstraintTypeRouter: {Field: ConstraintTypeRouter, PoolExpr: "myPool", Values: []string{"router2"}},
	})
}

func (s *S) TestToConstraintType(c *check.C) {
	_, err := ToConstraintType("")
	c.Assert(err, check.Equals, ErrInvalidConstraintType)
	_, err = ToConstraintType("x")
	c.Assert(err, check.Equals, ErrInvalidConstraintType)
	ct, err := ToConstraintType("team")
	c.Assert(err, check.IsNil)
	c.Assert(ct, check.Equals, ConstraintTypeTeam)
}
