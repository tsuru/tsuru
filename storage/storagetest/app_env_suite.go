// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"github.com/tsuru/tsuru/db"
	apptypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type AppEnvVarSuite struct {
	SuiteHooks
	AppEnvVarStorage apptypes.AppEnvVarStorage
}

func (s *AppEnvVarSuite) TestEnvList(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)

	conn.Apps().Insert(
		bson.M{"_id": "app-1", "provisioner": "docker", "default": true},
	)
}
