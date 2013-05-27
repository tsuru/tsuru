// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package hipache provides a router implementation that store routes in Redis,
// as specified by Hipache (https://github.com/dotcloud/hipache).
//
// It does not provided any exported type, in order to use the router, you must
// import this package and get the router intance using the function
// router.Get.
//
// In order to use this router, you need to define the "hipache:domain"
// setting.
package hipache

import (
	"errors"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/router"
)

var conn redis.Conn

var errRouteNotFound = errors.New("Route not found")

func init() {
	router.Register("hipache", hipacheRouter{})
}

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

type hipacheRouter struct{}

func (hipacheRouter) AddBackend(name string) error {
	domain, err := config.GetString("hipache:domain")
	if err != nil {
		return &routeError{"add", err}
	}
	frontend := "frontend:" + name + "." + domain
	conn, err := connect()
	if err != nil {
		return &routeError{"add", err}
	}
	_, err = conn.Do("RPUSH", frontend, name)
	if err != nil {
		return &routeError{"add", err}
	}
	return nil
}

func (hipacheRouter) RemoveBackend(name string) error {
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

func (hipacheRouter) AddRoute(name, address string) error {
	domain, err := config.GetString("hipache:domain")
	if err != nil {
		return &routeError{"add", err}
	}
	frontend := "frontend:" + name + "." + domain
	conn, err := connect()
	if err != nil {
		return &routeError{"add", err}
	}
	_, err = conn.Do("RPUSH", frontend, address)
	if err != nil {
		return &routeError{"add", err}
	}
	return nil
}

func (r hipacheRouter) RemoveRoute(name, address string) error {
	domain, err := config.GetString("hipache:domain")
	if err != nil {
		return &routeError{"remove", err}
	}
	frontend := "frontend:" + name + "." + domain
	return r.removeElement(frontend, address)
}

func (hipacheRouter) AddCName(cname, name string) error {
	domain, err := config.GetString("hipache:domain")
	if err != nil {
		return &routeError{"addCName", err}
	}
	frontend := "frontend:" + name + "." + domain
	conn, err := connect()
	if err != nil {
		return &routeError{"addCName", err}
	}
	addresses, err := redis.Strings(conn.Do("LRANGE", frontend, 0, -1))
	if err != nil {
		return &routeError{"get", err}
	}
	frontend = "frontend:" + cname
	for _, r := range addresses {
		_, err := conn.Do("RPUSH", frontend, r)
		if err != nil {
			return &routeError{"addCName", err}
		}
	}
	return nil
}

func (r hipacheRouter) RemoveCName(cname, address string) error {
	return r.removeElement("frontend:"+cname, address)
}

func (hipacheRouter) Addr(name string) (string, error) {
	domain, err := config.GetString("hipache:domain")
	if err != nil {
		return "", &routeError{"get", err}
	}
	frontend := "frontend:" + name + "." + domain
	conn, err := connect()
	if err != nil {
		return "", &routeError{"get", err}
	}
	reply, err := conn.Do("LRANGE", frontend, 0, 0)
	if err != nil {
		return "", &routeError{"get", err}
	}
	backends := reply.([]interface{})
	if len(backends) < 1 {
		return "", errRouteNotFound
	}
	return fmt.Sprintf("%s.%s", name, domain), nil
}

func (hipacheRouter) removeElement(name, address string) error {
	conn, err := connect()
	if err != nil {
		return &routeError{"remove", err}
	}
	_, err = conn.Do("LREM", name, 0, address)
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
