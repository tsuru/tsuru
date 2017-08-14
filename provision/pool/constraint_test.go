package pool

import (
	"reflect"

	check "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestSetPoolConstraints(c *check.C) {
	coll := s.storage.PoolsConstraints()
	err := SetPoolConstraint(&PoolConstraint{PoolExpr: "*", Field: "router", Values: []string{"planb", "hipache"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "*", Field: "team", Values: []string{"user"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	var cs []*PoolConstraint
	err = coll.Find(bson.M{"poolexpr": "*"}).All(&cs)
	c.Assert(err, check.IsNil)
	c.Assert(cs, check.DeepEquals, []*PoolConstraint{
		{PoolExpr: "*", Field: "router", Values: []string{"planb", "hipache"}},
		{PoolExpr: "*", Field: "team", Values: []string{"user"}, Blacklist: true},
	})
}

func (s *S) TestSetPoolConstraintsWithServices(c *check.C) {
	coll := s.storage.PoolsConstraints()
	err := SetPoolConstraint(&PoolConstraint{
		PoolExpr: "prod",
		Field:    "service",
		Values:   []string{"lux"},
	})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{
		PoolExpr: "dev",
		Field:    "service",
		Values:   []string{"demacia"},
	})
	c.Assert(err, check.IsNil)
	var cs []*PoolConstraint
	err = coll.Find(bson.M{"field": "service"}).All(&cs)
	c.Assert(err, check.IsNil)
	c.Assert(cs, check.DeepEquals, []*PoolConstraint{
		{PoolExpr: "prod", Field: "service", Values: []string{"lux"}},
		{PoolExpr: "dev", Field: "service", Values: []string{"demacia"}},
	})
}

func (s *S) TestSetPoolConstraintsRemoveEmpty(c *check.C) {
	coll := s.storage.PoolsConstraints()
	err := SetPoolConstraint(&PoolConstraint{PoolExpr: "*", Field: "router", Values: []string{"planb", "hipache"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "*", Field: "team", Values: []string{"user"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "*", Field: "team", Values: []string{""}})
	c.Assert(err, check.IsNil)
	var cs []*PoolConstraint
	err = coll.Find(bson.M{"poolexpr": "*"}).All(&cs)
	c.Assert(err, check.IsNil)
	c.Assert(cs, check.DeepEquals, []*PoolConstraint{
		{PoolExpr: "*", Field: "router", Values: []string{"planb", "hipache"}},
	})
}

func (s *S) TestSetPoolConstraintInvalidConstraintType(c *check.C) {
	coll := s.storage.PoolsConstraints()
	err := SetPoolConstraint(&PoolConstraint{PoolExpr: "*", Field: "invalid", Values: []string{"abc"}, Blacklist: true})
	c.Assert(err, check.Equals, ErrInvalidConstraintType)
	var cs []*PoolConstraint
	err = coll.Find(bson.M{"poolexpr": "*"}).All(&cs)
	c.Assert(err, check.IsNil)
	c.Assert(len(cs), check.Equals, 0)
}

func (s *S) TestGetConstraintsForPool(c *check.C) {
	err := SetPoolConstraint(&PoolConstraint{PoolExpr: "*", Field: "router", Values: []string{"planb"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "pp", Field: "router", Values: []string{"galeb"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "*_dev", Field: "router", Values: []string{"planb_dev"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "*_dev", Field: "team", Values: []string{"team_pool1"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "pool1_dev", Field: "team", Values: []string{"team_pool1"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "pool1\\x", Field: "team", Values: []string{"team_pool1x"}})
	c.Assert(err, check.IsNil)
	err = SetPoolConstraint(&PoolConstraint{PoolExpr: "*\\xdev", Field: "team", Values: []string{"team_xdev"}})
	c.Assert(err, check.IsNil)
	tt := []struct {
		pool     string
		expected map[string]*PoolConstraint
	}{
		{pool: "prod", expected: map[string]*PoolConstraint{
			"router": {PoolExpr: "*", Field: "router", Values: []string{"planb"}},
		}},
		{pool: "pp", expected: map[string]*PoolConstraint{
			"router": {PoolExpr: "pp", Field: "router", Values: []string{"galeb"}},
		}},
		{pool: "pool1_dev", expected: map[string]*PoolConstraint{
			"router": {PoolExpr: "*_dev", Field: "router", Values: []string{"planb_dev"}},
			"team":   {PoolExpr: "pool1_dev", Field: "team", Values: []string{"team_pool1"}},
		}},
		{pool: "pool2_dev", expected: map[string]*PoolConstraint{
			"router": {PoolExpr: "*_dev", Field: "router", Values: []string{"planb_dev"}},
			"team":   {PoolExpr: "*_dev", Field: "team", Values: []string{"team_pool1"}, Blacklist: true},
		}},
		{pool: "pp2", expected: map[string]*PoolConstraint{
			"router": {PoolExpr: "*", Field: "router", Values: []string{"planb"}},
		}},
		{pool: "pool1\\x", expected: map[string]*PoolConstraint{
			"router": {PoolExpr: "*", Field: "router", Values: []string{"planb"}},
			"team":   {PoolExpr: "pool1\\x", Field: "team", Values: []string{"team_pool1x"}},
		}},
		{pool: "abc\\xdev", expected: map[string]*PoolConstraint{
			"router": {PoolExpr: "*", Field: "router", Values: []string{"planb"}},
			"team":   {PoolExpr: "*\\xdev", Field: "team", Values: []string{"team_xdev"}},
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
	err := SetPoolConstraint(&PoolConstraint{PoolExpr: "*", Field: "router", Values: []string{"planb"}, Blacklist: true})
	c.Assert(err, check.IsNil)
	err = AppendPoolConstraint(&PoolConstraint{PoolExpr: "*", Field: "router", Values: []string{"galeb"}})
	c.Assert(err, check.IsNil)
	err = AppendPoolConstraint(&PoolConstraint{PoolExpr: "*", Field: "service", Values: []string{"autoscale"}})
	c.Assert(err, check.IsNil)
	constraints, err := getConstraintsForPool("*")
	c.Assert(err, check.IsNil)
	c.Assert(constraints, check.DeepEquals, map[string]*PoolConstraint{
		"router":  {Field: "router", PoolExpr: "*", Values: []string{"planb", "galeb"}, Blacklist: true},
		"service": {Field: "service", PoolExpr: "*", Values: []string{"autoscale"}},
	})
}

func (s *S) TestAppendPoolConstraintNewConstraint(c *check.C) {
	err := AppendPoolConstraint(&PoolConstraint{PoolExpr: "myPool", Field: "router", Values: []string{"galeb"}})
	c.Assert(err, check.IsNil)
	constraints, err := getConstraintsForPool("myPool")
	c.Assert(err, check.IsNil)
	c.Assert(constraints, check.DeepEquals, map[string]*PoolConstraint{
		"router": {Field: "router", PoolExpr: "myPool", Values: []string{"galeb"}},
	})
}
