// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue

import (
	"fmt"
	"sync"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/log"
)

type redismqQ struct {
	name    string
	prefix  string
	factory *redismqQFactory
	psc     *redis.PubSubConn
}

func (r *redismqQ) Pub(msg []byte) error {
	conn, err := r.factory.getConn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Do("PUBLISH", r.key(), msg)
	return err
}

func (r *redismqQ) UnSub() error {
	if r.psc == nil {
		return nil
	}
	err := r.psc.Unsubscribe()
	if err != nil {
		return err
	}
	err = r.psc.Close()
	if err != nil {
		return err
	}
	return nil
}

func (r *redismqQ) Sub() (chan []byte, error) {
	conn, err := r.factory.getConn(true)
	if err != nil {
		return nil, err
	}
	r.psc = &redis.PubSubConn{Conn: conn}
	msgChan := make(chan []byte)
	err = r.psc.Subscribe(r.key())
	if err != nil {
		return nil, err
	}
	go func() {
		defer close(msgChan)
		for {
			switch v := r.psc.Receive().(type) {
			case redis.Message:
				msgChan <- v.Data
			case redis.Subscription:
				if v.Count == 0 {
					return
				}
			case error:
				log.Errorf("Error receiving messages from channel %s: %s", r.key(), v.Error())
				return
			}
		}
	}()
	return msgChan, nil
}

func (r *redismqQ) key() string {
	return r.prefix + ":" + r.name
}

type redismqQFactory struct {
	sync.Mutex
	pool *redis.Pool
}

func (factory *redismqQFactory) Reset() {
}

func (factory *redismqQFactory) PubSub(name string) (PubSubQ, error) {
	return &redismqQ{name: name, factory: factory}, nil
}

func (factory *redismqQFactory) getConn(standAlone ...bool) (redis.Conn, error) {
	isStandAlone := len(standAlone) > 0 && standAlone[0]
	if isStandAlone {
		return factory.dial()
	}
	return factory.getPool().Get(), nil
}

func (factory *redismqQFactory) dial() (redis.Conn, error) {
	host, err := config.GetString("pubsub:redis-host")
	if err != nil {
		host, err = config.GetString("redis-queue:host")
		if err != nil {
			host = "localhost"
		}
	}
	port, err := config.Get("pubsub:redis-port")
	if err != nil {
		port, err = config.Get("redis-queue:port")
		if err != nil {
			port = "6379"
		}
	}
	port = fmt.Sprintf("%v", port)
	password, err := config.GetString("pubsub:redis-password")
	if err != nil {
		password, _ = config.GetString("redis-queue:password")
	}
	db, err := config.GetInt("pubsub:redis-db")
	if err != nil {
		db, err = config.GetInt("redis-queue:db")
		if err != nil {
			db = 3
		}
	}
	conn, err := redis.Dial("tcp", fmt.Sprintf("%s:%v", host, port))
	if err != nil {
		return nil, err
	}
	if password != "" {
		_, err = conn.Do("AUTH", password)
		if err != nil {
			return nil, err
		}
	}
	_, err = conn.Do("SELECT", db)
	return conn, err
}

func (factory *redismqQFactory) getPool() *redis.Pool {
	factory.Lock()
	defer factory.Unlock()
	if factory.pool != nil {
		return factory.pool
	}
	maxIdle, err := config.GetInt("pubsub:pool-max-idle-conn")
	if err != nil {
		maxIdle, err = config.GetInt("redis-queue:pool-max-idle-conn")
		if err != nil {
			maxIdle = 20
		}
	}
	idleTimeout, err := config.GetDuration("pubsub:pool-idle-timeout")
	if err != nil {
		idleTimeout, err = config.GetDuration("redis-queue:pool-idle-timeout")
		if err != nil {
			idleTimeout = 300
		}
	}
	idleTimeout = idleTimeout * time.Second
	factory.pool = &redis.Pool{
		MaxIdle:     maxIdle,
		IdleTimeout: idleTimeout,
		Dial:        factory.dial,
	}
	return factory.pool
}
