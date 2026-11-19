// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pool

import (
	"context"
	"fmt"
	"reflect"
	"testing"

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

func (s *S) TestCompileConstraintPatterns(c *check.C) {
	tests := []struct {
		expression string
		matches    []string
		rejects    []string
	}{
		{"", []string{""}, []string{"pool"}},
		{"*", []string{"", "pool"}, nil},
		{"pool**east", []string{"pooleast", "pool-prod-east"}, []string{"pool-west", "other-pool-east"}},
		{`pool.[1](x)+\path`, []string{`pool.[1](x)+\path`}, []string{"pool1x", `pool.[1](x)+\paths`}},
		{"пул*", []string{"пул", "пул-east"}, []string{"pool"}},
	}
	constraints := make([]*PoolConstraint, len(tests))
	for i, test := range tests {
		constraints[i] = &PoolConstraint{PoolExpr: test.expression}
	}
	patterns, err := compileConstraintPatterns(constraints)
	c.Assert(err, check.IsNil)
	c.Assert(patterns, check.HasLen, len(tests))
	for i, test := range tests {
		for _, value := range test.matches {
			c.Assert(patterns[i].MatchString(value), check.Equals, true, check.Commentf("expression %q, value %q", test.expression, value))
		}
		for _, value := range test.rejects {
			c.Assert(patterns[i].MatchString(value), check.Equals, false, check.Commentf("expression %q, value %q", test.expression, value))
		}
	}
	patterns, err = compileConstraintPatterns(nil)
	c.Assert(err, check.IsNil)
	c.Assert(patterns, check.HasLen, 0)
	patterns, err = compileConstraintPatterns([]*PoolConstraint{{PoolExpr: "\xff"}})
	c.Assert(err, check.NotNil)
	c.Assert(patterns, check.IsNil)
}

func (s *S) TestResolveConstraintsForPool(c *check.C) {
	constraints := []*PoolConstraint{
		{PoolExpr: "dev*", Field: ConstraintTypeRouter, Values: []string{"default-router"}},
		{PoolExpr: "prod*", Field: ConstraintTypeRouter, Values: []string{"prod-router"}, Blacklist: true},
		{PoolExpr: "prod-east", Field: ConstraintTypeRouter, Values: []string{"east-router"}},
		{PoolExpr: "prod-east", Field: ConstraintTypeTeam, Values: []string{"east-team"}},
		{PoolExpr: `pool\x`, Field: ConstraintTypeTeam, Values: []string{"escaped-team"}},
		{PoolExpr: "pro*", Field: ConstraintTypeRouter, Values: []string{"fallback-router"}},
	}
	tests := []struct {
		name     string
		pool     string
		expected map[PoolConstraintType]*PoolConstraint
	}{
		{
			name: "exact expression takes precedence",
			pool: "prod-east",
			expected: map[PoolConstraintType]*PoolConstraint{
				ConstraintTypeRouter: constraints[2],
				ConstraintTypeTeam:   constraints[3],
			},
		},
		{
			name: "more specific wildcard and blacklist are preserved",
			pool: "prod-west",
			expected: map[PoolConstraintType]*PoolConstraint{
				ConstraintTypeRouter: constraints[1],
			},
		},
		{
			name: "another wildcard is selected",
			pool: "development",
			expected: map[PoolConstraintType]*PoolConstraint{
				ConstraintTypeRouter: constraints[0],
			},
		},
		{
			name: "escaped expression is matched literally",
			pool: `pool\x`,
			expected: map[PoolConstraintType]*PoolConstraint{
				ConstraintTypeTeam: constraints[4],
			},
		},
		{
			name:     "no matching constraint",
			pool:     "qa",
			expected: map[PoolConstraintType]*PoolConstraint{},
		},
	}

	patterns, err := compileConstraintPatterns(constraints)
	c.Assert(err, check.IsNil)
	for _, test := range tests {
		resolved := resolveConstraintsForPool(test.pool, constraints, patterns)
		c.Assert(resolved, check.DeepEquals, test.expected, check.Commentf("%s", test.name))
	}
}

func BenchmarkResolveConstraintsForPoolProductionScale(b *testing.B) {
	const (
		poolCount       = 2786
		constraintCount = 2933
	)
	pools := make([]string, poolCount)
	constraints := make([]*PoolConstraint, constraintCount)
	for i := range constraints {
		poolName := fmt.Sprintf("pool-%04d", i)
		constraints[i] = &PoolConstraint{
			PoolExpr: poolName,
			Field:    ConstraintTypeTeam,
			Values:   []string{"admin"},
		}
		if i < len(pools) {
			pools[i] = poolName
		}
	}

	var resolved map[PoolConstraintType]*PoolConstraint
	b.ReportAllocs()
	for b.Loop() {
		patterns, err := compileConstraintPatterns(constraints)
		if err != nil {
			b.Fatal(err)
		}
		for _, poolName := range pools {
			resolved = resolveConstraintsForPool(poolName, constraints, patterns)
		}
	}
	if resolved[ConstraintTypeTeam] == nil {
		b.Fatal("expected a team constraint for the last pool")
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
