// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migrate

import (
	"testing"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/db/storage"
	check "gopkg.in/check.v1"
)

type S struct {
	coll *storage.Collection
}

func Test(t *testing.T) { check.TestingT(t) }

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_app_image_migrate_test")
	var err error
	s.coll, err = image.ImageCustomDataColl()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	s.coll.Database.DropDatabase()
	s.coll.Close()
}

func (s *S) SetUpTest(c *check.C) {
	dbtest.ClearAllCollections(s.coll.Database)
}

var _ = check.Suite(&S{})

func (s *S) TestMigrateExposedPorts(c *check.C) {
	err := s.coll.Insert(bson.M{"_id": "1-exposed-port", "exposedport": "8000/tcp"})
	c.Assert(err, check.IsNil)
	err = s.coll.Insert(bson.M{"_id": "2-exposed-port-and-ports", "exposedport": "8001/tcp", "exposedports": []string{"8002/tcp", "8003/tcp"}})
	c.Assert(err, check.IsNil)
	err = s.coll.Insert(bson.M{"_id": "3-exposed-ports", "exposedport": "", "exposedports": []string{"8004/tcp"}})
	c.Assert(err, check.IsNil)
	err = s.coll.Insert(bson.M{"_id": "4-no-port"})
	c.Assert(err, check.IsNil)

	err = MigrateExposedPorts()
	c.Assert(err, check.IsNil)

	var results []map[string]interface{}
	err = s.coll.Find(nil).All(&results)
	c.Assert(err, check.IsNil)
	c.Assert(results, check.DeepEquals, []map[string]interface{}{
		{"_id": "1-exposed-port", "exposedport": "8000/tcp", "exposedports": []interface{}{"8000/tcp"}},
		{"_id": "2-exposed-port-and-ports", "exposedport": "8001/tcp", "exposedports": []interface{}{"8002/tcp", "8003/tcp"}},
		{"_id": "3-exposed-ports", "exposedport": "", "exposedports": []interface{}{"8004/tcp"}},
		{"_id": "4-no-port"},
	})
}
