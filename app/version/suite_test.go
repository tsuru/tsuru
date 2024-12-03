// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package version

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/servicemanager"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "app_version_tests")
	config.Set("docker:collection", "docker")
	config.Set("docker:repository-namespace", "tsuru")

	storagev2.Reset()

	servicemanager.App = &appTypes.MockAppService{}
}

func (s *S) TearDownTest(c *check.C) {
	storagev2.ClearAllCollections(nil)
}

func (s *S) TearDownSuite(c *check.C) {
}
