// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/tsuru/config"
	check "gopkg.in/check.v1"
)

type CheckerSuite struct{}

var _ = check.Suite(&CheckerSuite{})

var configFixture = `
listen: 0.0.0.0:8080
host: http://127.0.0.1:8080
debug: true

database:
  url: 127.0.0.1:3435
  name: tsuru

auth:
  user-registration: true
  scheme: native

provisioner: docker
docker:
  collection: docker_containers
  repository-namespace: tsuru
  cluster:
    mongo-url: mongodb://localhost:27017
    mongo-database: docker-cluster
`

func (s *CheckerSuite) SetUpTest(c *check.C) {
	err := config.ReadConfigBytes([]byte(configFixture))
	c.Assert(err, check.IsNil)
}

func (s *CheckerSuite) TestCheckDatabaseConfigDefault(c *check.C) {
	err := checkDatabase()
	c.Assert(err, check.IsNil)
}

func (s *CheckerSuite) TestCheckDatabaseConfigMongodb(c *check.C) {
	config.Set("database:driver", "mongodb")
	err := checkDatabase()
	c.Assert(err, check.IsNil)
}

func (s *CheckerSuite) TestCheckDatabaseConfigError(c *check.C) {
	config.Unset("database:url")
	err := checkDatabase()
	c.Assert(err, check.NotNil)
	config.Set("database:url", "/url")
	config.Unset("database:name")
	err = checkDatabase()
	c.Assert(err, check.NotNil)
}

func (s *CheckerSuite) TestCheckDatabaseConfigDriverError(c *check.C) {
	config.Set("database:driver", "postgres")
	err := checkDatabase()
	c.Assert(err, check.NotNil)
}

func (s *CheckerSuite) TestCheckDockerJustCheckIfProvisionerIsDocker(c *check.C) {
	config.Set("provisioner", "test")
	err := checkProvisioner()
	c.Assert(err, check.IsNil)
}

func (s *CheckerSuite) TestCheckDockerIsNotConfigured(c *check.C) {
	config.Unset("docker")
	err := checkDocker()
	c.Assert(err, check.NotNil)
}

func (s *CheckerSuite) TestCheckDockerBasicConfig(c *check.C) {
	err := checkDockerBasicConfig()
	c.Assert(err, check.IsNil)
}

func (s *CheckerSuite) TestCheckDockerBasicConfigError(c *check.C) {
	config.Unset("docker:collection")
	err := checkDockerBasicConfig()
	c.Assert(err, check.NotNil)
}

func (s *CheckerSuite) TestCheckClusterWithMongo(c *check.C) {
	err := checkCluster()
	c.Assert(err, check.IsNil)
	toFail := []string{"docker:cluster:mongo-url", "docker:cluster:mongo-database"}
	for _, prop := range toFail {
		val, _ := config.Get(prop)
		config.Unset(prop)
		err := checkCluster()
		c.Assert(err, check.NotNil)
		config.Set(prop, val)
	}
}

func (s *CheckerSuite) TestCheckClusterWithDeprecatedStorage(c *check.C) {
	config.Set("docker:cluster:storage", "redis")
	err := checkCluster()
	c.Assert(err, check.NotNil)
	config.Set("docker:cluster:storage", "something")
	err = checkCluster()
	c.Assert(err, check.NotNil)
	config.Unset("docker:cluster:storage")
}
