// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hipache

import (
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

type Router struct{}

func (Router) AddRoute(name, ip string) error {
	domain, err := config.GetString("hipache:domain")
	if err != nil {
		return &routeError{err}
	}
	frontend := "frontend:" + name + "." + domain
	conn, err := connect()
	if err != nil {
		return &routeError{err}
	}
	_, err = conn.Do("RPUSH", frontend, ip)
	if err != nil {
		return &routeError{err}
	}
	return nil
}

type routeError struct {
	err error
}

func (e *routeError) Error() string {
	return "Could not add route: " + e.err.Error()
}
