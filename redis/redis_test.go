// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redis

import (
	"fmt"
	"testing"
	"time"

	"github.com/tsuru/config"
	"gopkg.in/check.v1"
	"gopkg.in/redis.v3"
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
}

func (s *S) setConfig(key string, value interface{}) {
	key = fmt.Sprintf("%s:%s", s.prefix, key)
	config.Set(key, value)
	s.configsSet = append(s.configsSet, key)
}

func (s *S) TestNewRedisSimpleServer(c *check.C) {
	s.setConfig("redis-server", "localhost:6379")
	r, err := NewRedis(s.prefix)
	c.Assert(err, check.IsNil)
	c.Assert(r, check.NotNil)
}

func (s *S) TestNewRedisSimpleServerUsePassword(c *check.C) {
	s.setConfig("redis-server", "localhost:6379")
	s.setConfig("redis-password", "invalid-password")
	_, err := NewRedis(s.prefix)
	c.Assert(err, check.ErrorMatches, "ERR Client sent AUTH, but no password is set")
}

func (s *S) TestNewRedisSimpleServerUseDB(c *check.C) {
	s.setConfig("redis-server", "localhost:6379")
	s.setConfig("redis-db", "0")
	r, err := NewRedis(s.prefix)
	c.Assert(err, check.IsNil)
	err = r.Set("tsuru-test-key", "b", time.Minute).Err()
	c.Assert(err, check.IsNil)
	defer r.Del("tsuru-test-key")
	s.setConfig("redis-db", "1")
	r, err = NewRedis(s.prefix)
	c.Assert(err, check.IsNil)
	err = r.Get("tsuru-test-key").Err()
	c.Assert(err, check.Equals, redis.Nil)
}

func (s *S) TestNewRedisSentinel(c *check.C) {
	s.setConfig("redis-sentinel-addrs", "127.0.0.1:6380,127.0.0.1:6381")
	s.setConfig("redis-sentinel-master", "mymaster")
	_, err := NewRedis(s.prefix)
	c.Assert(err, check.ErrorMatches, "redis: all sentinels are unreachable")
}

func (s *S) TestNewRedisSentinelNoMaster(c *check.C) {
	s.setConfig("redis-sentinel-addrs", "127.0.0.1:6380,127.0.0.1:6381")
	_, err := NewRedis(s.prefix)
	c.Assert(err, check.ErrorMatches, "test-redis:redis-sentinel-master must be specified if using redis-sentinel")
}

func (s *S) TestNewRedisCluster(c *check.C) {
	s.setConfig("redis-cluster-addrs", "127.0.0.1:6380,127.0.0.1:6381")
	_, err := NewRedis(s.prefix)
	c.Assert(err, check.ErrorMatches, "dial tcp 127.0.0.1:\\d+: getsockopt: connection refused")
}

func (s *S) TestNewRedisClusterStripWhitespace(c *check.C) {
	s.setConfig("redis-cluster-addrs", "  127.0.0.1:6380 , 127.0.0.1:6381")
	_, err := NewRedis(s.prefix)
	c.Assert(err, check.ErrorMatches, "dial tcp 127.0.0.1:\\d+: getsockopt: connection refused")
}

func (s *S) TestNewRedisClusterWithMaxRetries(c *check.C) {
	s.setConfig("redis-cluster-addrs", "127.0.0.1:6380,127.0.0.1:6381")
	s.setConfig("redis-max-retries", 1)
	_, err := NewRedis(s.prefix)
	c.Assert(err, check.ErrorMatches, "could not initialize redis from \"test-redis\" config, using redis-cluster with max-retries > 0 is not supported")
}

func (s *S) TestNewRedisClusterWithDB(c *check.C) {
	s.setConfig("redis-cluster-addrs", "127.0.0.1:6380,127.0.0.1:6381")
	s.setConfig("redis-db", 1)
	_, err := NewRedis(s.prefix)
	c.Assert(err, check.ErrorMatches, "could not initialize redis from \"test-redis\" config, using redis-cluster with db != 0 is not supported")
}

func (s *S) TestNewRedisHostPort(c *check.C) {
	s.setConfig("redis-host", "localhost")
	s.setConfig("redis-port", 6379)
	r, err := NewRedis(s.prefix)
	c.Assert(err, check.IsNil)
	c.Assert(r, check.NotNil)
}

func (s *S) TestNewRedisHostPortLegacy(c *check.C) {
	s.setConfig("host", "localhost")
	s.setConfig("port", 6379)
	r, err := NewRedisDefaultConfig(s.prefix, &CommonConfig{TryLegacy: true})
	c.Assert(err, check.IsNil)
	c.Assert(r, check.NotNil)
}

func (s *S) TestNewRedisTryLocal(c *check.C) {
	r, err := NewRedisDefaultConfig(s.prefix, &CommonConfig{TryLocal: true})
	c.Assert(err, check.IsNil)
	c.Assert(r, check.NotNil)
}

func (s *S) TestNewRedisNotConfigured(c *check.C) {
	r, err := NewRedis(s.prefix)
	c.Assert(err, check.Equals, ErrNoRedisConfig)
	c.Assert(r, check.IsNil)
}
