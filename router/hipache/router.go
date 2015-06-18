// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package hipache provides a router implementation that store routes in Redis,
// as specified by Hipache (https://github.com/dotcloud/hipache).
//
// It does not provide any exported type, in order to use the router, you must
// import this package and get the router instance using the function
// router.Get.
//
// In order to use this router, you need to define the "routers:<name>:type =
// hipache" in your config.
package hipache

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/garyburd/redigo/redis"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/hc"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/router"
)

const routerName = "hipache"

func init() {
	router.Register(routerName, createRouter)
	hc.AddChecker("Router Hipache", router.BuildHealthCheck("hipache"))
}

func createRouter(prefix string) (router.Router, error) {
	return &hipacheRouter{prefix: prefix}, nil
}

func (r *hipacheRouter) connect() redis.Conn {
	r.Lock()
	defer r.Unlock()
	if r.pool == nil {
		srv := r.redisServer()
		r.pool = &redis.Pool{
			Dial: func() (redis.Conn, error) {
				conn, err := redis.Dial("tcp", srv)
				if err != nil {
					return nil, err
				}
				password, _ := config.GetString(r.prefix + ":redis-password")
				if password != "" {
					_, err = conn.Do("AUTH", password)
					if err != nil {
						return nil, err
					}
				}
				return conn, nil
			},
			MaxIdle:     10,
			IdleTimeout: 180e9,
		}
	}
	return r.pool.Get()
}

func (r *hipacheRouter) redisServer() string {
	srv, err := config.GetString(r.prefix + ":redis-server")
	if err != nil {
		srv = "localhost:6379"
	}
	return srv
}

type hipacheRouter struct {
	sync.Mutex
	prefix string
	pool   *redis.Pool
}

func (r *hipacheRouter) AddBackend(name string) error {
	domain, err := config.GetString(r.prefix + ":domain")
	if err != nil {
		return &routeError{"add", err}
	}
	frontend := "frontend:" + name + "." + domain
	conn := r.connect()
	defer conn.Close()
	_, err = conn.Do("RPUSH", frontend, name)
	if err != nil {
		return &routeError{"add", err}
	}
	return router.Store(name, name, routerName)
}

func (r *hipacheRouter) RemoveBackend(name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	domain, err := config.GetString(r.prefix + ":domain")
	if err != nil {
		return &routeError{"remove", err}
	}
	frontend := "frontend:" + backendName + "." + domain
	conn := r.connect()
	defer conn.Close()
	_, err = conn.Do("DEL", frontend)
	if err != nil {
		return &routeError{"remove", err}
	}
	err = router.Remove(backendName)
	if err != nil {
		return &routeError{"remove", err}
	}
	cnames, err := r.getCNames(backendName)
	if err != nil {
		return err
	}
	if cnames == nil {
		return nil
	}
	for _, cname := range cnames {
		_, err = conn.Do("DEL", "frontend:"+cname)
		if err != nil {
			return &routeError{"remove", err}
		}
	}
	_, err = conn.Do("DEL", "cname:"+backendName)
	if err != nil {
		return &routeError{"remove", err}
	}
	return nil
}

func (r *hipacheRouter) AddRoute(name string, address *url.URL) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	domain, err := config.GetString(r.prefix + ":domain")
	if err != nil {
		log.Errorf("error on getting hipache domain in add route for %s - %s", backendName, address)
		return &routeError{"add", err}
	}
	frontend := "frontend:" + backendName + "." + domain
	if err := r.addRoute(frontend, address.String()); err != nil {
		log.Errorf("error on add route for %s - %s", backendName, address)
		return &routeError{"add", err}
	}
	cnames, err := r.getCNames(backendName)
	if err != nil {
		log.Errorf("error on get cname in add route for %s - %s", backendName, address)
		return err
	}
	if cnames == nil {
		return nil
	}
	for _, cname := range cnames {
		err = r.addRoute("frontend:"+cname, address.String())
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *hipacheRouter) addRoute(name, address string) error {
	conn := r.connect()
	defer conn.Close()
	_, err := conn.Do("RPUSH", name, address)
	if err != nil {
		log.Errorf("error on store in redis in add route for %s - %s", name, address)
		return &routeError{"add", err}
	}
	return nil
}

func (r *hipacheRouter) RemoveRoute(name string, address *url.URL) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	domain, err := config.GetString(r.prefix + ":domain")
	if err != nil {
		return &routeError{"remove", err}
	}
	frontend := "frontend:" + backendName + "." + domain
	if err := r.removeElement(frontend, address.String()); err != nil {
		return err
	}
	cnames, err := r.getCNames(backendName)
	if err != nil {
		return &routeError{"remove", err}
	}
	if cnames == nil {
		return nil
	}
	for _, cname := range cnames {
		err = r.removeElement("frontend:"+cname, address.String())
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *hipacheRouter) HealthCheck() error {
	conn := r.connect()
	defer conn.Close()
	result, err := redis.String(conn.Do("PING"))
	if err != nil {
		return err
	}
	if result != "PONG" {
		return fmt.Errorf("unexpected PING response from Redis server, want %q, got %q", "PONG", result)
	}
	return nil
}

func (r *hipacheRouter) getCNames(name string) ([]string, error) {
	conn := r.connect()
	defer conn.Close()
	cnames, err := redis.Strings(conn.Do("LRANGE", "cname:"+name, 0, -1))
	if err != nil && err != redis.ErrNil {
		return nil, &routeError{"getCName", err}
	}
	return cnames, nil
}

// validCName returns true if the cname is not a subdomain of
// hipache:domain conf, false otherwise
func (r *hipacheRouter) validCName(cname string) bool {
	domain, err := config.GetString(r.prefix + ":domain")
	if err != nil {
		return false
	}
	return !strings.HasSuffix(cname, domain)
}

func (r *hipacheRouter) SetCName(cname, name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	domain, err := config.GetString(r.prefix + ":domain")
	if err != nil {
		return &routeError{"setCName", err}
	}
	if !r.validCName(cname) {
		err := errors.New(fmt.Sprintf("Invalid CNAME %s. You can't use tsuru's application domain.", cname))
		return &routeError{"setCName", err}
	}
	frontend := "frontend:" + backendName + "." + domain
	conn := r.connect()
	defer conn.Close()
	routes, err := redis.Strings(conn.Do("LRANGE", frontend, 0, -1))
	if err != nil {
		return &routeError{"get", err}
	}
	_, err = conn.Do("RPUSH", "cname:"+backendName, cname)
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

func (r *hipacheRouter) UnsetCName(cname, name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	conn := r.connect()
	defer conn.Close()
	_, err = conn.Do("LREM", "cname:"+backendName, 1, cname)
	if err != nil {
		return &routeError{"unsetCName", err}
	}
	_, err = conn.Do("DEL", "frontend:"+cname)
	if err != nil {
		return &routeError{"unsetCName", err}
	}
	return nil
}

func (r *hipacheRouter) Addr(name string) (string, error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return "", err
	}
	domain, err := config.GetString(r.prefix + ":domain")
	if err != nil {
		return "", &routeError{"get", err}
	}
	frontend := "frontend:" + backendName + "." + domain
	conn := r.connect()
	defer conn.Close()
	reply, err := conn.Do("LRANGE", frontend, 0, 0)
	if err != nil {
		return "", &routeError{"get", err}
	}
	backends := reply.([]interface{})
	if len(backends) < 1 {
		return "", router.ErrRouteNotFound
	}
	return fmt.Sprintf("%s.%s", backendName, domain), nil
}

func (r *hipacheRouter) Routes(name string) ([]*url.URL, error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return nil, err
	}
	domain, err := config.GetString(r.prefix + ":domain")
	if err != nil {
		return nil, &routeError{"routes", err}
	}
	frontend := "frontend:" + backendName + "." + domain
	conn := r.connect()
	defer conn.Close()
	routes, err := redis.Strings(conn.Do("LRANGE", frontend, 0, -1))
	if err != nil {
		return nil, &routeError{"routes", err}
	}
	result := make([]*url.URL, len(routes))
	for i, route := range routes {
		result[i], err = url.Parse(route)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *hipacheRouter) removeElement(name, address string) error {
	conn := r.connect()
	defer conn.Close()
	_, err := conn.Do("LREM", name, 0, address)
	if err != nil {
		return &routeError{"remove", err}
	}
	return nil
}

func (r *hipacheRouter) Swap(backend1, backend2 string) error {
	return router.Swap(r, backend1, backend2)
}

func (r *hipacheRouter) StartupMessage() (string, error) {
	domain, err := config.GetString(r.prefix + ":domain")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("hipache router %q with redis at %q.", domain, r.redisServer()), nil
}

type routeError struct {
	op  string
	err error
}

func (e *routeError) Error() string {
	return fmt.Sprintf("Could not %s route: %s", e.op, e.err)
}
