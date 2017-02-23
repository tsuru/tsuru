// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestMigratePoolTeamsToPoolConstraints(c *check.C) {
	p := &Pool{Name: "pool1"}
	err := s.storage.Pools().Insert(p)
	c.Assert(err, check.IsNil)
	err = s.storage.Pools().Update(bson.M{"_id": "pool1"}, bson.M{"$set": bson.M{"teams": []string{"team1", "team2"}}})
	c.Assert(err, check.IsNil)
	err = MigratePoolTeamsToPoolConstraints()
	c.Assert(err, check.IsNil)
	constraint, err := getExactConstraintForPool("pool1", "team")
	c.Assert(err, check.IsNil)
	c.Assert(constraint, check.NotNil)
	c.Assert(constraint.Values, check.DeepEquals, []string{"team1", "team2"})
}
