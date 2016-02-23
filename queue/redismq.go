// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"fmt"
	"sync"
	"time"

	"github.com/tsuru/tsuru/log"
	tsuruRedis "github.com/tsuru/tsuru/redis"
	"gopkg.in/redis.v3"
)

type redisPubSub struct {
	name    string
	prefix  string
	factory *redisPubSubFactory
	psc     *redis.PubSub
}

func (r *redisPubSub) Pub(msg []byte) error {
	conn, err := r.factory.getConn()
	if err != nil {
		return err
	}
	return conn.Publish(r.key(), string(msg)).Err()
}

func (r *redisPubSub) UnSub() error {
	err := r.psc.Close()
	if err != nil {
		return err
	}
	return nil
}

func (r *redisPubSub) Sub() (<-chan []byte, error) {
	conn, err := r.factory.getConn()
	if err != nil {
		return nil, err
	}
	r.psc, err = conn.Subscribe(r.key())
	if err != nil {
		return nil, err
	}
	msgChan := make(chan []byte)
	go func() {
		defer close(msgChan)
		for {
			msg, err := r.psc.ReceiveTimeout(r.factory.config.ReadTimeout)
			if err != nil {
				log.Errorf("Error receiving messages from channel %s: %s", r.key(), err)
				return
			}
			switch v := msg.(type) {
			case *redis.Message:
				msgChan <- []byte(v.Payload)
			case *redis.Subscription:
				if v.Count == 0 {
					return
				}
			}
		}
	}()
	return msgChan, nil
}

func (r *redisPubSub) key() string {
	return r.prefix + ":" + r.name
}

type redisPubSubFactory struct {
	sync.Mutex
	pool   tsuruRedis.PubSubClient
	config *tsuruRedis.CommonConfig
}

func (factory *redisPubSubFactory) Reset() {
}

func (factory *redisPubSubFactory) PubSub(name string) (PubSubQ, error) {
	return &redisPubSub{name: name, factory: factory}, nil
}

func (factory *redisPubSubFactory) getConn() (tsuruRedis.PubSubClient, error) {
	factory.Lock()
	defer factory.Unlock()
	if factory.pool != nil {
		return factory.pool, nil
	}
	factory.config = &tsuruRedis.CommonConfig{
		PoolSize:     1000,
		PoolTimeout:  2 * time.Second,
		IdleTimeout:  2 * time.Minute,
		MaxRetries:   1,
		DialTimeout:  100 * time.Millisecond,
		ReadTimeout:  30 * time.Minute,
		WriteTimeout: 500 * time.Millisecond,
		TryLegacy:    true,
	}
	client, err := tsuruRedis.NewRedisDefaultConfig("pubsub", factory.config)
	if err == tsuruRedis.ErrNoRedisConfig {
		factory.config.TryLocal = true
		client, err = tsuruRedis.NewRedisDefaultConfig("redis-queue", factory.config)
	}
	if err != nil {
		return nil, err
	}
	var ok bool
	factory.pool, ok = client.(tsuruRedis.PubSubClient)
	if !ok {
		return nil, fmt.Errorf("redis client is not a capable of pubsub: %#v", client)
	}
	return factory.pool, nil
}
