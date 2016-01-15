// Copyright 2016 tsuru authors. All rights reserved.
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
	"fmt"
	"net/url"
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

func createRouter(routerName, configPrefix string) (router.Router, error) {
	return &hipacheRouter{prefix: configPrefix}, nil
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
				db, err := config.GetInt(r.prefix + ":redis-db")
				if err == nil {
					_, err = conn.Do("SELECT", db)
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
		return &router.RouterError{Op: "add", Err: err}
	}
	frontend := "frontend:" + name + "." + domain
	conn := r.connect()
	defer conn.Close()
	exists, err := redis.Bool(conn.Do("EXISTS", frontend))
	if err != nil {
		return &router.RouterError{Op: "add", Err: err}
	}
	if exists {
		return router.ErrBackendExists
	}
	_, err = conn.Do("RPUSH", frontend, name)
	if err != nil {
		return &router.RouterError{Op: "add", Err: err}
	}
	return router.Store(name, name, routerName)
}

func (r *hipacheRouter) RemoveBackend(name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	if backendName != name {
		return router.ErrBackendSwapped
	}
	domain, err := config.GetString(r.prefix + ":domain")
	if err != nil {
		return &router.RouterError{Op: "remove", Err: err}
	}
	frontend := "frontend:" + backendName + "." + domain
	conn := r.connect()
	defer conn.Close()
	_, err = conn.Do("DEL", frontend)
	if err != nil {
		return &router.RouterError{Op: "remove", Err: err}
	}
	err = router.Remove(backendName)
	if err != nil {
		return &router.RouterError{Op: "remove", Err: err}
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
			return &router.RouterError{Op: "remove", Err: err}
		}
	}
	_, err = conn.Do("DEL", "cname:"+backendName)
	if err != nil {
		return &router.RouterError{Op: "remove", Err: err}
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
		return &router.RouterError{Op: "add", Err: err}
	}
	routes, err := r.Routes(name)
	if err != nil {
		return err
	}
	for _, r := range routes {
		if r.String() == address.String() {
			return router.ErrRouteExists
		}
	}
	frontend := "frontend:" + backendName + "." + domain
	if err = r.addRoute(frontend, address.String()); err != nil {
		log.Errorf("error on add route for %s - %s", backendName, address)
		return &router.RouterError{Op: "add", Err: err}
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
		return &router.RouterError{Op: "add", Err: err}
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
		return &router.RouterError{Op: "remove", Err: err}
	}
	frontend := "frontend:" + backendName + "." + domain
	count, err := r.removeElement(frontend, address.String())
	if err != nil {
		return err
	}
	if count == 0 {
		return router.ErrRouteNotFound
	}
	cnames, err := r.getCNames(backendName)
	if err != nil {
		return &router.RouterError{Op: "remove", Err: err}
	}
	if cnames == nil {
		return nil
	}
	for _, cname := range cnames {
		_, err = r.removeElement("frontend:"+cname, address.String())
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
		return nil, &router.RouterError{Op: "getCName", Err: err}
	}
	return cnames, nil
}

func (r *hipacheRouter) SetCName(cname, name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	domain, err := config.GetString(r.prefix + ":domain")
	if err != nil {
		return &router.RouterError{Op: "setCName", Err: err}
	}
	if !router.ValidCName(cname, domain) {
		return router.ErrCNameNotAllowed
	}
	conn := r.connect()
	defer conn.Close()
	cnameExists := false
	currentCnames, err := redis.Strings(conn.Do("LRANGE", "cname:"+backendName, 0, -1))
	for _, n := range currentCnames {
		if n == cname {
			cnameExists = true
			break
		}
	}
	if !cnameExists {
		_, err = conn.Do("RPUSH", "cname:"+backendName, cname)
		if err != nil {
			return &router.RouterError{Op: "set", Err: err}
		}
	}
	frontend := "frontend:" + backendName + "." + domain
	cnameFrontend := "frontend:" + cname
	wantedRoutes, err := redis.Strings(conn.Do("LRANGE", frontend, 1, -1))
	if err != nil {
		return &router.RouterError{Op: "get", Err: err}
	}
	currentRoutes, err := redis.Strings(conn.Do("LRANGE", cnameFrontend, 0, -1))
	if err != nil {
		return &router.RouterError{Op: "get", Err: err}
	}
	// Routes are always added again and duplicates removed this will ensure
	// that after a call to SetCName is made routes will be identical to the
	// original entry.
	if len(currentRoutes) == 0 {
		_, err := conn.Do("RPUSH", cnameFrontend, backendName)
		if err != nil {
			return &router.RouterError{Op: "setCName", Err: err}
		}
	} else {
		currentRoutes = currentRoutes[1:]
	}
	for _, r := range wantedRoutes {
		_, err := conn.Do("RPUSH", cnameFrontend, r)
		if err != nil {
			return &router.RouterError{Op: "setCName", Err: err}
		}
	}
	for _, r := range currentRoutes {
		_, err := conn.Do("LREM", cnameFrontend, "1", r)
		if err != nil {
			return &router.RouterError{Op: "setCName", Err: err}
		}
	}
	if cnameExists {
		return router.ErrCNameExists
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
	currentCnames, err := redis.Strings(conn.Do("LRANGE", "cname:"+backendName, 0, -1))
	found := false
	for _, n := range currentCnames {
		if n == cname {
			found = true
			break
		}
	}
	if !found {
		return router.ErrCNameNotFound
	}
	_, err = conn.Do("LREM", "cname:"+backendName, 0, cname)
	if err != nil {
		return &router.RouterError{Op: "unsetCName", Err: err}
	}
	_, err = conn.Do("DEL", "frontend:"+cname)
	if err != nil {
		return &router.RouterError{Op: "unsetCName", Err: err}
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
		return "", &router.RouterError{Op: "get", Err: err}
	}
	frontend := "frontend:" + backendName + "." + domain
	conn := r.connect()
	defer conn.Close()
	reply, err := conn.Do("LRANGE", frontend, 0, 0)
	if err != nil {
		return "", &router.RouterError{Op: "get", Err: err}
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
		return nil, &router.RouterError{Op: "routes", Err: err}
	}
	frontend := "frontend:" + backendName + "." + domain
	conn := r.connect()
	defer conn.Close()
	routes, err := redis.Strings(conn.Do("LRANGE", frontend, 1, -1))
	if err != nil {
		return nil, &router.RouterError{Op: "routes", Err: err}
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

func (r *hipacheRouter) removeElement(name, address string) (int, error) {
	conn := r.connect()
	defer conn.Close()
	count, err := redis.Int(conn.Do("LREM", name, 0, address))
	if err != nil {
		return 0, &router.RouterError{Op: "remove", Err: err}
	}
	return count, nil
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
