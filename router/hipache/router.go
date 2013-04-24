// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hipache

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/globocom/config"
)

var conn redis.Conn

func connect() (redis.Conn, error) {
	if conn == nil {
		srv, err := config.GetString("hipache:redis-server")
		if err != nil {
			srv = "localhost:6379"
		}
		conn, err = redis.Dial("tcp", srv)
		if err != nil {
			return nil, err
		}
	}
	return conn, nil
}

type router struct{}

func (router) AddRoute(name, ip string) error {
	domain, err := config.GetString("hipache:domain")
	if err != nil {
		return &routeError{"add", err}
	}
	frontend := "frontend:" + name + "." + domain
	conn, err := connect()
	if err != nil {
		return &routeError{"add", err}
	}
	_, err = conn.Do("RPUSH", frontend, ip)
	if err != nil {
		return &routeError{"add", err}
	}
	return nil
}

func (router) RemoveRoute(name string) error {
	domain, err := config.GetString("hipache:domain")
	if err != nil {
		return &routeError{"remove", err}
	}
	frontend := "frontend:" + name + "." + domain
	conn, err := connect()
	if err != nil {
		return &routeError{"remove", err}
	}
	_, err = conn.Do("DEL", frontend)
	if err != nil {
		return &routeError{"remove", err}
	}
	return nil
}

type routeError struct {
	op  string
	err error
}

func (e *routeError) Error() string {
	return fmt.Sprintf("Could not %s route: %s", e.op, e.err)
}
