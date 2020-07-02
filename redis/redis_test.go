// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redis

import (
	"fmt"
	"testing"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/router"
	check "gopkg.in/check.v1"
	redis "gopkg.in/redis.v3"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type S struct {
	prefix     string
	configsSet []string
}

var _ = check.Suite(&S{})

func (s *S) SetUpTest(c *check.C) {
	s.prefix = "test-redis"
	for _, entry := range s.configsSet {
		config.Unset(entry)
	}
	s.configsSet = nil
	collector.clients = map[string]poolStatsClient{}
}

func (s *S) setConfig(key string, value interface{}) {
	key = fmt.Sprintf("%s:%s", s.prefix, key)
	config.Set(key, value)
	s.configsSet = append(s.configsSet, key)
}

func (s *S) TestNewRedisSimpleServer(c *check.C) {
	s.setConfig("redis-server", "localhost:6379")
	r, err := NewRedisDefaultConfig("mine", &router.StaticConfigGetter{Prefix: s.prefix}, nil)
	c.Assert(err, check.IsNil)
	c.Assert(r, check.NotNil)
	c.Assert(collector.clients["mine"], check.NotNil)
}

func (s *S) TestNewRedisSimpleServerUsePassword(c *check.C) {
	s.setConfig("redis-server", "localhost:6379")
	s.setConfig("redis-password", "invalid-password")
	cli, err := NewRedisDefaultConfig("mine", &router.StaticConfigGetter{Prefix: s.prefix}, nil)
	c.Assert(err, check.ErrorMatches, "ERR Client sent AUTH, but no password is set")
	c.Assert(cli, check.IsNil)
	c.Assert(collector.clients["mine"], check.IsNil)
}

func (s *S) TestNewRedisSimpleServerUseDB(c *check.C) {
	s.setConfig("redis-server", "localhost:6379")
	s.setConfig("redis-db", "0")
	r, err := NewRedisDefaultConfig("mine", &router.StaticConfigGetter{Prefix: s.prefix}, nil)
	c.Assert(err, check.IsNil)
	err = r.Set("tsuru-test-key", "b", time.Minute).Err()
	c.Assert(err, check.IsNil)
	defer r.Del("tsuru-test-key")
	s.setConfig("redis-db", "1")
	r, err = NewRedisDefaultConfig("mine", &router.StaticConfigGetter{Prefix: s.prefix}, nil)
	c.Assert(err, check.IsNil)
	err = r.Get("tsuru-test-key").Err()
	c.Assert(err, check.Equals, redis.Nil)
	c.Assert(collector.clients["mine"], check.NotNil)
}

func (s *S) TestNewRedisSentinel(c *check.C) {
	s.setConfig("redis-sentinel-addrs", "127.0.0.1:6380,127.0.0.1:6381")
	s.setConfig("redis-sentinel-master", "mymaster")
	cli, err := NewRedisDefaultConfig("mine", &router.StaticConfigGetter{Prefix: s.prefix}, nil)
	c.Assert(err, check.ErrorMatches, "redis: all sentinels are unreachable")
	c.Assert(cli, check.IsNil)
	c.Assert(collector.clients["mine"], check.IsNil)
}

func (s *S) TestNewRedisSentinelNoMaster(c *check.C) {
	s.setConfig("redis-sentinel-addrs", "127.0.0.1:6380,127.0.0.1:6381")
	cli, err := NewRedisDefaultConfig("mine", &router.StaticConfigGetter{Prefix: s.prefix}, nil)
	c.Assert(err, check.ErrorMatches, "mine:redis-sentinel-master must be specified if using redis-sentinel")
	c.Assert(cli, check.IsNil)
	c.Assert(collector.clients["mine"], check.IsNil)
}

func (s *S) TestNewRedisCluster(c *check.C) {
	s.setConfig("redis-cluster-addrs", "127.0.0.1:6380,127.0.0.1:6381")
	cli, err := NewRedisDefaultConfig("mine", &router.StaticConfigGetter{Prefix: s.prefix}, nil)
	c.Assert(err, check.ErrorMatches, "dial tcp 127.0.0.1:\\d+:.*")
	c.Assert(cli, check.IsNil)
	c.Assert(collector.clients["mine"], check.IsNil)
}

func (s *S) TestNewRedisClusterStripWhitespace(c *check.C) {
	s.setConfig("redis-cluster-addrs", "  127.0.0.1:6380 , 127.0.0.1:6381")
	cli, err := NewRedisDefaultConfig("mine", &router.StaticConfigGetter{Prefix: s.prefix}, nil)
	c.Assert(err, check.ErrorMatches, "dial tcp 127.0.0.1:\\d+:.*?")
	c.Assert(cli, check.IsNil)
	c.Assert(collector.clients["mine"], check.IsNil)
}

func (s *S) TestNewRedisClusterWithMaxRetries(c *check.C) {
	s.setConfig("redis-cluster-addrs", "127.0.0.1:6380,127.0.0.1:6381")
	s.setConfig("redis-max-retries", 1)
	cli, err := NewRedisDefaultConfig("mine", &router.StaticConfigGetter{Prefix: s.prefix}, nil)
	c.Assert(err, check.ErrorMatches, "could not initialize redis from \"mine\" config, using redis-cluster with max-retries > 0 is not supported")
	c.Assert(cli, check.IsNil)
	c.Assert(collector.clients["mine"], check.IsNil)
}

func (s *S) TestNewRedisClusterWithDB(c *check.C) {
	s.setConfig("redis-cluster-addrs", "127.0.0.1:6380,127.0.0.1:6381")
	s.setConfig("redis-db", 1)
	cli, err := NewRedisDefaultConfig("mine", &router.StaticConfigGetter{Prefix: s.prefix}, nil)
	c.Assert(err, check.ErrorMatches, "could not initialize redis from \"mine\" config, using redis-cluster with db != 0 is not supported")
	c.Assert(cli, check.IsNil)
	c.Assert(collector.clients["mine"], check.IsNil)
}

func (s *S) TestNewRedisHostPort(c *check.C) {
	s.setConfig("redis-host", "localhost")
	s.setConfig("redis-port", 6379)
	r, err := NewRedisDefaultConfig("mine", &router.StaticConfigGetter{Prefix: s.prefix}, nil)
	c.Assert(err, check.IsNil)
	c.Assert(r, check.NotNil)
	c.Assert(collector.clients["mine"], check.NotNil)
}

func (s *S) TestNewRedisHostPortLegacy(c *check.C) {
	s.setConfig("host", "localhost")
	s.setConfig("port", 6379)
	r, err := NewRedisDefaultConfig("mine", &router.StaticConfigGetter{Prefix: s.prefix}, &CommonConfig{TryLegacy: true})
	c.Assert(err, check.IsNil)
	c.Assert(r, check.NotNil)
	c.Assert(collector.clients["mine"], check.NotNil)
}

func (s *S) TestNewRedisTryLocal(c *check.C) {
	r, err := NewRedisDefaultConfig("mine", &router.StaticConfigGetter{Prefix: s.prefix}, &CommonConfig{TryLocal: true})
	c.Assert(err, check.IsNil)
	c.Assert(r, check.NotNil)
	c.Assert(collector.clients["mine"], check.NotNil)
}

func (s *S) TestNewRedisNotConfigured(c *check.C) {
	r, err := NewRedisDefaultConfig("mine", &router.StaticConfigGetter{Prefix: s.prefix}, nil)
	c.Assert(err, check.Equals, ErrNoRedisConfig)
	c.Assert(r, check.IsNil)
	c.Assert(collector.clients["mine"], check.IsNil)
}
