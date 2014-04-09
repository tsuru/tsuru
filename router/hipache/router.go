// Copyright 2014 tsuru authors. All rights reserved.
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
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/router"
	"strings"
)

var pool *redis.Pool

var errRouteNotFound = errors.New("Route not found")

func init() {
	router.Register("hipache", hipacheRouter{})
}

func connect() redis.Conn {
	if pool == nil {
		srv, err := config.GetString("hipache:redis-server")
		if err != nil {
			srv = "localhost:6379"
		}
		pool = redis.NewPool(func() (redis.Conn, error) {
			return redis.Dial("tcp", srv)
		}, 10)
	}
	pool.IdleTimeout = 180e9
	return pool.Get()
}

type hipacheRouter struct{}

func (hipacheRouter) AddBackend(name string) error {
	domain, err := config.GetString("hipache:domain")
	if err != nil {
		return &routeError{"add", err}
	}
	frontend := "frontend:" + name + "." + domain
	conn := connect()
	defer conn.Close()
	_, err = conn.Do("RPUSH", frontend, name)
	if err != nil {
		return &routeError{"add", err}
	}
	return router.Store(name, name)
}

func (r hipacheRouter) RemoveBackend(name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	domain, err := config.GetString("hipache:domain")
	if err != nil {
		return &routeError{"remove", err}
	}
	frontend := "frontend:" + backendName + "." + domain
	conn := connect()
	defer conn.Close()
	_, err = conn.Do("DEL", frontend)
	if err != nil {
		return &routeError{"remove", err}
	}
	err = router.Remove(backendName)
	if err != nil {
		return &routeError{"remove", err}
	}
	cname, err := r.getCName(backendName)
	if err != nil {
		return err
	}
	if cname == "" {
		return nil
	}
	_, err = conn.Do("DEL", "frontend:"+cname)
	if err != nil {
		return &routeError{"remove", err}
	}
	_, err = conn.Do("DEL", "cname:"+backendName)
	if err != nil {
		return &routeError{"remove", err}
	}
	return nil
}

func (r hipacheRouter) AddRoute(name, address string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	domain, err := config.GetString("hipache:domain")
	if err != nil {
		log.Errorf("error on getting hipache domain in add route for %s - %s", backendName, address)
		return &routeError{"add", err}
	}
	frontend := "frontend:" + backendName + "." + domain
	if err := r.addRoute(frontend, address); err != nil {
		log.Errorf("error on add route for %s - %s", backendName, address)
		return &routeError{"add", err}
	}
	cname, err := r.getCName(backendName)
	if err != nil {
		log.Errorf("error on get cname in add route for %s - %s", backendName, address)
		return err
	}
	if cname == "" {
		return nil
	}
	return r.addRoute("frontend:"+cname, address)
}

func (hipacheRouter) addRoute(name, address string) error {
	conn := connect()
	defer conn.Close()
	_, err := conn.Do("RPUSH", name, address)
	if err != nil {
		log.Errorf("error on store in redis in add route for %s - %s", name, address)
		return &routeError{"add", err}
	}
	return nil
}

func (r hipacheRouter) RemoveRoute(name, address string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	domain, err := config.GetString("hipache:domain")
	if err != nil {
		return &routeError{"remove", err}
	}
	frontend := "frontend:" + backendName + "." + domain
	if err := r.removeElement(frontend, address); err != nil {
		return err
	}
	cname, err := r.getCName(backendName)
	if err != nil {
		return &routeError{"remove", err}
	}
	if cname == "" {
		return nil
	}
	return r.removeElement("frontend:"+cname, address)
}

func (hipacheRouter) getCName(name string) (string, error) {
	conn := connect()
	defer conn.Close()
	cname, err := redis.String(conn.Do("GET", "cname:"+name))
	if err != nil && err != redis.ErrNil {
		return "", &routeError{"getCName", err}
	}
	return cname, nil
}

// validCName returns true if the cname is not a subdomain of
// hipache:domain conf, false otherwise
func (hipacheRouter) validCName(cname string) bool {
	domain, err := config.GetString("hipache:domain")
	if err != nil {
		return false
	}
	return !strings.Contains(cname, domain)
}

func (r hipacheRouter) SetCName(cname, name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	domain, err := config.GetString("hipache:domain")
	if err != nil {
		return &routeError{"setCName", err}
	}
	if !r.validCName(cname) {
		err := errors.New(fmt.Sprintf("Invalid CNAME %s. You can't use tsuru's application domain.", cname))
		return &routeError{"setCName", err}
	}
	frontend := "frontend:" + backendName + "." + domain
	conn := connect()
	defer conn.Close()
	routes, err := redis.Strings(conn.Do("LRANGE", frontend, 0, -1))
	if err != nil {
		return &routeError{"get", err}
	}
	if oldCName, err := redis.String(conn.Do("GET", "cname:"+backendName)); err == nil && oldCName != "" {
		err = r.UnsetCName(oldCName, backendName)
		if err != nil {
			return &routeError{"setCName", err}
		}
	}
	_, err = conn.Do("SET", "cname:"+backendName, cname)
	if err != nil {
		return &routeError{"set", err}
	}
	frontend = "frontend:" + cname
	for _, r := range routes {
		_, err := conn.Do("RPUSH", frontend, r)
		if err != nil {
			return &routeError{"setCName", err}
		}
	}
	return nil
}

func (r hipacheRouter) UnsetCName(cname, name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	conn := connect()
	defer conn.Close()
	_, err = conn.Do("DEL", "cname:"+backendName)
	if err != nil {
		return &routeError{"unsetCName", err}
	}
	_, err = conn.Do("DEL", "frontend:"+cname)
	if err != nil {
		return &routeError{"unsetCName", err}
	}
	return nil
}

func (hipacheRouter) Addr(name string) (string, error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return "", err
	}
	domain, err := config.GetString("hipache:domain")
	if err != nil {
		return "", &routeError{"get", err}
	}
	frontend := "frontend:" + backendName + "." + domain
	conn := connect()
	defer conn.Close()
	reply, err := conn.Do("LRANGE", frontend, 0, 0)
	if err != nil {
		return "", &routeError{"get", err}
	}
	backends := reply.([]interface{})
	if len(backends) < 1 {
		return "", errRouteNotFound
	}
	return fmt.Sprintf("%s.%s", backendName, domain), nil
}

func (hipacheRouter) Routes(name string) ([]string, error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return nil, err
	}
	domain, err := config.GetString("hipache:domain")
	if err != nil {
		return nil, &routeError{"routes", err}
	}
	frontend := "frontend:" + backendName + "." + domain
	conn := connect()
	defer conn.Close()
	routes, err := redis.Strings(conn.Do("LRANGE", frontend, 0, -1))
	if err != nil {
		return nil, &routeError{"routes", err}
	}
	return routes, nil
}

func (hipacheRouter) removeElement(name, address string) error {
	conn := connect()
	defer conn.Close()
	_, err := conn.Do("LREM", name, 0, address)
	if err != nil {
		return &routeError{"remove", err}
	}
	return nil
}

func (r hipacheRouter) Swap(backend1, backend2 string) error {
	return router.Swap(r, backend1, backend2)
}

type routeError struct {
	op  string
	err error
}

func (e *routeError) Error() string {
	return fmt.Sprintf("Could not %s route: %s", e.op, e.err)
}
