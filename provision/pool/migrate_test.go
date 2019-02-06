// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pool

import (
	"github.com/globalsign/mgo/bson"
	check "gopkg.in/check.v1"
)

func (s *S) TestMigratePoolTeamsToPoolConstraints(c *check.C) {
	err := s.storage.Pools().Insert(&Pool{Name: "pool1"})
	c.Assert(err, check.IsNil)
	err = s.storage.Pools().Insert(&bson.M{"_id": "publicPool", "public": true})
	c.Assert(err, check.IsNil)
	err = s.storage.Pools().Update(bson.M{"_id": "pool1"}, bson.M{"$set": bson.M{"teams": []string{"team1", "team2"}}})
	c.Assert(err, check.IsNil)
	err = MigratePoolTeamsToPoolConstraints()
	c.Assert(err, check.IsNil)
	constraint, err := getExactConstraintForPool("pool1", "team")
	c.Assert(err, check.IsNil)
	c.Assert(constraint, check.DeepEquals, &PoolConstraint{
		PoolExpr: "pool1",
		Values:   []string{"team1", "team2"},
		Field:    ConstraintTypeTeam,
	})
	constraint, err = getExactConstraintForPool("publicPool", "team")
	c.Assert(err, check.IsNil)
	c.Assert(constraint, check.DeepEquals, &PoolConstraint{
		PoolExpr: "publicPool",
		Values:   []string{"*"},
		Field:    ConstraintTypeTeam,
	})
}
