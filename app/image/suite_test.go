// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image_test

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/servicemanager"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	mockService servicemock.MockService
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "app_image_tests")
	config.Set("docker:collection", "docker")
	config.Set("docker:repository-namespace", "tsuru")

	storagev2.Reset()
}

func (s *S) SetUpTest(c *check.C) {
	var err error
	servicemock.SetMockService(&s.mockService)
	servicemanager.AppVersion, err = version.AppVersionService()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownTest(c *check.C) {
	storagev2.ClearAllCollections(nil)
}

func (s *S) TearDownSuite(c *check.C) {
}
