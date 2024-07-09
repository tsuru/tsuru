// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"context"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/types/provision"
	"gopkg.in/check.v1"

	mongoBSON "go.mongodb.org/mongo-driver/bson"
)

type PoolSuite struct {
	SuiteHooks
	PoolStorage provision.PoolStorage
}

func (s *PoolSuite) TestFindAll(c *check.C) {

	collection, err := storagev2.PoolCollection()
	c.Assert(err, check.IsNil)

	_, err = collection.InsertMany(context.TODO(), []interface{}{
		mongoBSON.M{"_id": "pool-A", "provisioner": "docker", "default": true},
		mongoBSON.M{"_id": "pool-B", "provisioner": "kubernetes"},
	},
	)
	c.Assert(err, check.IsNil)
	pools, err := s.PoolStorage.FindAll(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(pools, check.DeepEquals, []provision.Pool{
		{Name: "pool-A", Provisioner: "docker", Default: true},
		{Name: "pool-B", Provisioner: "kubernetes"},
	})
}

func (s *PoolSuite) TestFindByName(c *check.C) {
	collection, err := storagev2.PoolCollection()
	c.Assert(err, check.IsNil)
	_, err = collection.InsertMany(context.TODO(), []interface{}{
		mongoBSON.M{"_id": "pool-A", "provisioner": "docker", "default": true},
		mongoBSON.M{"_id": "pool-B", "provisioner": "kubernetes"},
	})
	c.Assert(err, check.IsNil)
	pool, err := s.PoolStorage.FindByName(context.TODO(), "pool-B")
	c.Assert(err, check.IsNil)
	c.Assert(pool, check.DeepEquals, &provision.Pool{Name: "pool-B", Provisioner: "kubernetes"})

}

func (s *PoolSuite) TestFindByName_PoolNotFound(c *check.C) {
	_, err := s.PoolStorage.FindByName(context.TODO(), "pool-not-found")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.DeepEquals, provision.ErrPoolNotFound)
}
