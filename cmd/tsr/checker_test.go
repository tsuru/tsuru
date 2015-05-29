// Copyright 2015 Globo.com. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

type CheckerSuite struct{}

var _ = check.Suite(&CheckerSuite{})

var configFixture = `
listen: 0.0.0.0:8080
host: http://127.0.0.1:8080
debug: true
admin-team: admin

database:
  url: 127.0.0.1:3435
  name: tsuru

git:
  unit-repo: /home/application/current
  api-server: http://127.0.0.1:8000

auth:
  user-registration: true
  scheme: native

provisioner: docker
routers:
  hipache:
    type: hipache
    domain: tsuru-sample.com
pubsub:
  redis-host: localhost
  redis-port: 6379
queue:
  mongo-url: localhost
  mongo-database: queuedb
docker:
  collection: docker_containers
  repository-namespace: tsuru
  router: hipache
  deploy-cmd: /var/lib/tsuru/deploy
  cluster:
    mongo-url: mongodb://localhost:27017
    mongo-database: docker-cluster
  run-cmd:
    bin: /var/lib/tsuru/start
    port: 8888
  ssh:
    user: ubuntu
`

func (s *CheckerSuite) SetUpTest(c *check.C) {
	err := config.ReadConfigBytes([]byte(configFixture))
	c.Assert(err, check.IsNil)
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

func (s *CheckerSuite) TestCheckGandalfErrorRepoManagerDefined(c *check.C) {
	config.Set("repo-manager", "gandalf")
	config.Unset("git:api-server")
	err := checkGandalf()
	c.Assert(err, check.NotNil)
}

func (s *CheckerSuite) TestCheckGandalfErrorRepoManagerUndefined(c *check.C) {
	config.Unset("repo-manager")
	config.Unset("git:api-server")
	err := checkGandalf()
	c.Assert(err, check.NotNil)
}

func (s *CheckerSuite) TestCheckGandalfSuccessRepoManagerUndefined(c *check.C) {
	config.Unset("repo-manager")
	config.Set("git:api-server", "http://gandalf.com")
	err := checkGandalf()
	c.Assert(err, check.IsNil)
}

func (s *CheckerSuite) TestCheckGandalfSuccessRepoManagerDefined(c *check.C) {
	config.Set("repo-manager", "gandalf")
	config.Set("git:api-server", "http://gandalf.com")
	err := checkGandalf()
	c.Assert(err, check.IsNil)
}

func (s *CheckerSuite) TestCheckSchedulerConfig(c *check.C) {
	err := checkScheduler()
	c.Assert(err, check.IsNil)
}

func (s *CheckerSuite) TestCheckDockerServersError(c *check.C) {
	config.Set("docker:servers", []string{"srv1", "srv2"})
	err := checkScheduler()
	c.Assert(err, check.ErrorMatches, `Using docker:servers is deprecated, please remove it your config and use "tsuru-admin docker-node-add" do add docker nodes.`)
}

func (s *CheckerSuite) TestCheckSchedulerConfigSegregate(c *check.C) {
	config.Set("docker:segregate", true)
	err := checkScheduler()
	baseWarn := config.NewWarning("")
	c.Assert(err, check.FitsTypeOf, baseWarn)
	c.Assert(err, check.ErrorMatches, `Setting "docker:segregate" is not necessary anymore, this is the default behavior from now on.`)
	config.Set("docker:segregate", false)
	err = checkScheduler()
	c.Assert(err, check.Not(check.FitsTypeOf), baseWarn)
	c.Assert(err, check.ErrorMatches, `You must remove "docker:segregate" from your config.`)
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

func (s *CheckerSuite) TestCheckRouter(c *check.C) {
	err := checkRouter()
	c.Assert(err, check.IsNil)
}

func (s *CheckerSuite) TestCheckRouterHipacheShouldHaveHipacheConf(c *check.C) {
	config.Unset("routers:hipache")
	err := checkRouter()
	c.Assert(err, check.ErrorMatches, ".*default router \"hipache\" in \"routers:hipache\".*")
}

func (s *CheckerSuite) TestCheckRouterHipacheCanHaveHipacheInRoutersConf(c *check.C) {
	config.Unset("routers:hipache")
	config.Set("hipache:domain", "something")
	err := checkRouter()
	c.Assert(err, check.FitsTypeOf, config.NewWarning(""))
	c.Assert(err, check.ErrorMatches, ".*Setting \"hipache:\\*\" config entries is deprecated.*")
}

func (s *CheckerSuite) TestCheckRouterValidatesDefaultRouterNotExisting(c *check.C) {
	config.Unset("routers:hipache")
	config.Set("docker:router", "myrouter")
	err := checkRouter()
	c.Assert(err, check.ErrorMatches, ".*default router \"myrouter\" in \"routers:myrouter\".*")
}

func (s *CheckerSuite) TestCheckRouterValidatesDefaultRouter(c *check.C) {
	config.Unset("routers:hipache")
	config.Set("docker:router", "myrouter")
	config.Set("routers:myrouter:planet", "giediprime")
	config.Set("routers:myrouter:type", "something")
	err := checkRouter()
	c.Assert(err, check.IsNil)
}

func (s *CheckerSuite) TestCheckRouterValidatesDefaultRouterPresence(c *check.C) {
	config.Unset("routers:hipache")
	config.Unset("docker:router")
	err := checkRouter()
	c.Assert(err, check.ErrorMatches, ".*You must configure a default router in \"docker:router\".*")
}

func (s *CheckerSuite) TestCheckRouterValidatesDefaultRouterType(c *check.C) {
	config.Unset("routers:hipache:type")
	err := checkRouter()
	c.Assert(err, check.ErrorMatches, ".*You must configure your default router type in \"routers:hipache:type\".*")
}

func (s *CheckerSuite) TestCheckBeanstalkdRedisQueue(c *check.C) {
	err := checkBeanstalkd()
	c.Assert(err, check.IsNil)
}

func (s *CheckerSuite) TestCheckBeanstalkdNoQueueConfigured(c *check.C) {
	old, _ := config.Get("queue")
	defer config.Set("queue", old)
	config.Unset("queue")
	err := checkBeanstalkd()
	c.Assert(err, check.IsNil)
}

func (s *CheckerSuite) TestCheckBeanstalkdDefinedInQueue(c *check.C) {
	old, _ := config.Get("queue")
	defer config.Set("queue", old)
	config.Set("queue", "beanstalkd")
	err := checkBeanstalkd()
	c.Assert(err.Error(), check.Equals, "beanstalkd is no longer supported, please use redis instead")
}

func (w *CheckerSuite) TestCheckBeanstalkdQueueServerDefined(c *check.C) {
	config.Set("queue-server", "127.0.0.1:11300")
	defer config.Unset("queue-server")
	err := checkBeanstalkd()
	c.Assert(err.Error(), check.Equals, `beanstalkd is no longer supported, please remove the "queue-server" setting from your config file`)
}

func (w *CheckerSuite) TestCheckPubSub(c *check.C) {
	err := checkPubSub()
	c.Assert(err, check.IsNil)
}

func (w *CheckerSuite) TestCheckPubSubOld(c *check.C) {
	config.Unset("pubsub:redis-host")
	config.Set("redis-queue:host", "localhost")
	err := checkPubSub()
	c.Assert(err, check.FitsTypeOf, config.NewWarning(""))
	c.Assert(err, check.ErrorMatches, ".*Using \"redis-queue:\\*\" is deprecated.*")
}

func (w *CheckerSuite) TestCheckPubSubMissing(c *check.C) {
	config.Unset("pubsub:redis-host")
	err := checkPubSub()
	c.Assert(err, check.FitsTypeOf, config.NewWarning(""))
	c.Assert(err, check.ErrorMatches, ".*Config entry \"pubsub:redis-host\" is not set.*")
}

func (w *CheckerSuite) TestCheckQueue(c *check.C) {
	err := checkQueue()
	c.Assert(err, check.IsNil)
}

func (w *CheckerSuite) TestCheckQueueNotSet(c *check.C) {
	config.Unset("queue:mongo-url")
	err := checkQueue()
	c.Assert(err, check.FitsTypeOf, config.NewWarning(""))
	c.Assert(err, check.ErrorMatches, ".*Config entry \"queue:mongo-url\" is not set.*")
}
