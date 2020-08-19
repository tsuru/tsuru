// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redis

import (
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/log"
	routerTypes "github.com/tsuru/tsuru/types/router"
	redis "gopkg.in/redis.v3"
)

var (
	ErrNoRedisConfig = errors.New("no redis configuration found with config prefix")
)

type baseClient interface {
	Exists(key string) *redis.BoolCmd
	RPush(key string, values ...string) *redis.IntCmd
	Set(key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Get(key string) *redis.StringCmd
	Del(keys ...string) *redis.IntCmd
	Ping() *redis.StatusCmd
	LRange(key string, start, stop int64) *redis.StringSliceCmd
	LRem(key string, count int64, value interface{}) *redis.IntCmd
	Auth(password string) *redis.StatusCmd
	Select(index int64) *redis.StatusCmd
	Keys(pattern string) *redis.StringSliceCmd
	LLen(key string) *redis.IntCmd
	HMGet(key string, fields ...string) *redis.SliceCmd
	HMSetMap(key string, fields map[string]string) *redis.StatusCmd
	HLen(key string) *redis.IntCmd
	Close() error
}

type poolStatsClient interface {
	PoolStats() *redis.PoolStats
}

type Client interface {
	baseClient
	poolStatsClient
	Pipeline() Pipeline
}

type Pipeline interface {
	baseClient
	Exec() ([]redis.Cmder, error)
}

type ClientWrapper struct {
	*redis.Client
}

type ClusterClientWrapper struct {
	*redis.ClusterClient
}

func (c *ClientWrapper) Pipeline() Pipeline {
	return c.Client.Pipeline()
}

func (c *ClusterClientWrapper) Pipeline() Pipeline {
	return c.ClusterClient.Pipeline()
}

type CommonConfig struct {
	DB           int64
	Password     string
	MaxRetries   int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	PoolSize     int
	PoolTimeout  time.Duration
	IdleTimeout  time.Duration
	TryLegacy    bool
	TryLocal     bool
}

func newRedisSentinel(addrs []string, master string, redisConfig *CommonConfig) (Client, error) {
	client := redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:    master,
		SentinelAddrs: addrs,
		DB:            redisConfig.DB,
		Password:      redisConfig.Password,
		MaxRetries:    redisConfig.MaxRetries,
		DialTimeout:   redisConfig.DialTimeout,
		ReadTimeout:   redisConfig.ReadTimeout,
		WriteTimeout:  redisConfig.WriteTimeout,
		PoolSize:      redisConfig.PoolSize,
		PoolTimeout:   redisConfig.PoolTimeout,
		IdleTimeout:   redisConfig.IdleTimeout,
	})
	err := client.Ping().Err()
	if err != nil {
		client.Close()
		return nil, err
	}
	return &ClientWrapper{Client: client}, nil
}

func redisCluster(addrs []string, redisConfig *CommonConfig) (Client, error) {
	client := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:        addrs,
		Password:     redisConfig.Password,
		DialTimeout:  redisConfig.DialTimeout,
		ReadTimeout:  redisConfig.ReadTimeout,
		WriteTimeout: redisConfig.WriteTimeout,
		PoolSize:     redisConfig.PoolSize,
		PoolTimeout:  redisConfig.PoolTimeout,
		IdleTimeout:  redisConfig.IdleTimeout,
	})
	err := client.Ping().Err()
	if err != nil {
		client.Close()
		return nil, err
	}
	return &ClusterClientWrapper{ClusterClient: client}, nil
}

func redisServer(addr string, redisConfig *CommonConfig) (Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		DB:           redisConfig.DB,
		Password:     redisConfig.Password,
		MaxRetries:   redisConfig.MaxRetries,
		DialTimeout:  redisConfig.DialTimeout,
		ReadTimeout:  redisConfig.ReadTimeout,
		WriteTimeout: redisConfig.WriteTimeout,
		PoolSize:     redisConfig.PoolSize,
		PoolTimeout:  redisConfig.PoolTimeout,
		IdleTimeout:  redisConfig.IdleTimeout,
	})
	err := client.Ping().Err()
	if err != nil {
		client.Close()
		return nil, err
	}
	return &ClientWrapper{Client: client}, nil
}

func createServerList(addrs string) []string {
	parts := strings.Split(addrs, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func NewRedisDefaultConfig(name string, config routerTypes.ConfigGetter, defaultConfig *CommonConfig) (client Client, err error) {
	defer func() {
		if client != nil {
			collector.Add(name, client)
		}
	}()
	if defaultConfig == nil {
		defaultConfig = &CommonConfig{
			PoolSize:    1000,
			PoolTimeout: time.Second,
			IdleTimeout: 2 * time.Minute,
		}
	}
	db, err := config.GetInt("redis-db")
	if err != nil && defaultConfig.TryLegacy {
		db, err = config.GetInt("db")
	}
	if err == nil {
		defaultConfig.DB = int64(db)
	}
	password, err := config.GetString("redis-password")
	if err != nil && defaultConfig.TryLegacy {
		password, err = config.GetString("password")
	}
	if err == nil {
		defaultConfig.Password = password
	}
	poolSize, err := config.GetInt("redis-pool-size")
	if err == nil {
		defaultConfig.PoolSize = poolSize
	}
	maxRetries, err := config.GetInt("redis-max-retries")
	if err == nil {
		defaultConfig.MaxRetries = maxRetries
	}
	poolTimeout, err := config.GetFloat("redis-pool-timeout")
	if err == nil {
		defaultConfig.PoolTimeout = time.Duration(poolTimeout * float64(time.Second))
	}
	idleTimeout, err := config.GetFloat("redis-pool-idle-timeout")
	if err == nil {
		defaultConfig.IdleTimeout = time.Duration(idleTimeout * float64(time.Second))
	}
	dialTimeout, err := config.GetFloat("redis-dial-timeout")
	if err == nil {
		defaultConfig.DialTimeout = time.Duration(dialTimeout * float64(time.Second))
	}
	readTimeout, err := config.GetFloat("redis-read-timeout")
	if err == nil {
		defaultConfig.ReadTimeout = time.Duration(readTimeout * float64(time.Second))
	}
	writeTimeout, err := config.GetFloat("redis-write-timeout")
	if err == nil {
		defaultConfig.WriteTimeout = time.Duration(writeTimeout * float64(time.Second))
	}
	sentinels, err := config.GetString("redis-sentinel-addrs")
	if err == nil {
		masterName, _ := config.GetString("redis-sentinel-master")
		if masterName == "" {
			return nil, errors.Errorf("%s:redis-sentinel-master must be specified if using redis-sentinel", name)
		}
		log.Debugf("Connecting to redis sentinel from %q config prefix. Addrs: %s. Master: %s. DB: %d.", name, sentinels, masterName, db)
		return newRedisSentinel(createServerList(sentinels), masterName, defaultConfig)
	}
	cluster, err := config.GetString("redis-cluster-addrs")
	if err == nil {
		if defaultConfig.DB != 0 {
			return nil, errors.Errorf("could not initialize redis from %q config, using redis-cluster with db != 0 is not supported", name)
		}
		if defaultConfig.MaxRetries != 0 {
			return nil, errors.Errorf("could not initialize redis from %q config, using redis-cluster with max-retries > 0 is not supported", name)
		}
		log.Debugf("Connecting to redis cluster from %q config prefix. Addrs: %s. DB: %d.", name, cluster, db)
		return redisCluster(createServerList(cluster), defaultConfig)
	}
	server, err := config.GetString("redis-server")
	if err == nil {
		log.Debugf("Connecting to redis server from %q config prefix. Addr: %s. DB: %d.", name, server, db)
		return redisServer(server, defaultConfig)
	}
	host, err := config.GetString("redis-host")
	if err != nil && defaultConfig.TryLegacy {
		host, err = config.GetString("host")
	}
	if err == nil {
		portStr := "6379"
		port, err := config.Get("redis-port")
		if err != nil && defaultConfig.TryLegacy {
			port, err = config.Get("port")
		}
		if err == nil {
			portStr = fmt.Sprintf("%v", port)
		}
		addr := fmt.Sprintf("%s:%s", host, portStr)
		log.Debugf("Connecting to redis host/port from %q config prefix. Addr: %s. DB: %d.", name, addr, db)
		return redisServer(addr, defaultConfig)
	}
	if defaultConfig.TryLocal {
		addr := "localhost:6379"
		log.Debugf("Connecting to redis on localhost from %q config prefix. Addr: %s. DB: %d.", name, addr, db)
		return redisServer(addr, defaultConfig)
	}
	return nil, ErrNoRedisConfig
}
