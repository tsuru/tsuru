// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockercommon

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/servicemanager"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	check "gopkg.in/check.v1"
)

type S struct{}

var _ = check.Suite(&S{})

func (s *S) SetUpTest(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "provision_dockercommon_internal_tests_s")
	config.Set("docker:registry", "my.registry")
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = storagev2.ClearAllCollections(nil)
	c.Assert(err, check.IsNil)
	servicemanager.AppVersion, err = version.AppVersionService()
	c.Assert(err, check.IsNil)
}
