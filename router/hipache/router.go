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
	"strconv"
	"sync"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/hc"
	"github.com/tsuru/tsuru/log"
	tsuruRedis "github.com/tsuru/tsuru/redis"
	"github.com/tsuru/tsuru/router"
	"gopkg.in/redis.v3"
)

const routerType = "hipache"

var (
	redisClients    = map[string]tsuruRedis.Client{}
	redisClientsMut sync.RWMutex
)

func init() {
	router.Register(routerType, createRouter)
	router.Register("planb", createRouter)
	hc.AddChecker("Router Hipache", router.BuildHealthCheck("hipache"))
	hc.AddChecker("Router Planb", router.BuildHealthCheck("planb"))
}

func createRouter(routerName, configPrefix string) (router.Router, error) {
	return &hipacheRouter{prefix: configPrefix}, nil
}

func (r *hipacheRouter) connect() (tsuruRedis.Client, error) {
	redisClientsMut.RLock()
	client := redisClients[r.prefix]
	if client == nil {
		redisClientsMut.RUnlock()
		redisClientsMut.Lock()
		defer redisClientsMut.Unlock()
		client = redisClients[r.prefix]
		if client == nil {
			var err error
			client, err = tsuruRedis.NewRedisDefaultConfig(r.prefix, &tsuruRedis.CommonConfig{
				PoolSize:     1000,
				PoolTimeout:  2 * time.Second,
				IdleTimeout:  2 * time.Minute,
				MaxRetries:   1,
				DialTimeout:  time.Second,
				ReadTimeout:  2 * time.Second,
				WriteTimeout: 2 * time.Second,
				TryLocal:     true,
			})
			if err != nil {
				return nil, err
			}
			redisClients[r.prefix] = client
		}
	} else {
		redisClientsMut.RUnlock()
	}
	return client, nil
}

type hipacheRouter struct {
	prefix string
}

func (r *hipacheRouter) AddBackend(name string) error {
	domain, err := config.GetString(r.prefix + ":domain")
	if err != nil {
		return &router.RouterError{Op: "add", Err: err}
	}
	frontend := "frontend:" + name + "." + domain
	conn, err := r.connect()
	if err != nil {
		return &router.RouterError{Op: "add", Err: err}
	}
	exists, err := conn.Exists(frontend).Result()
	if err != nil {
		return &router.RouterError{Op: "add", Err: err}
	}
	if exists {
		return router.ErrBackendExists
	}
	err = conn.RPush(frontend, name).Err()
	if err != nil {
		return &router.RouterError{Op: "add", Err: err}
	}
	return router.Store(name, name, routerType)
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
	conn, err := r.connect()
	if err != nil {
		return &router.RouterError{Op: "remove", Err: err}
	}
	err = conn.Del(frontend).Err()
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
		err = conn.Del("frontend:" + cname).Err()
		if err != nil {
			return &router.RouterError{Op: "remove", Err: err}
		}
	}
	err = conn.Del("cname:" + backendName).Err()
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

func (r *hipacheRouter) AddRoutes(name string, addresses []*url.URL) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	domain, err := config.GetString(r.prefix + ":domain")
	if err != nil {
		log.Errorf("error on getting hipache domain in add route for %s - %v", backendName, addresses)
		return &router.RouterError{Op: "add", Err: err}
	}
	routes, err := r.Routes(name)
	if err != nil {
		return err
	}
	toAdd := make([]string, 0, len(addresses))
addresses:
	for _, addr := range addresses {
		for _, r := range routes {
			if r.String() == addr.String() {
				continue addresses
			}
		}
		toAdd = append(toAdd, addr.String())
	}
	frontend := "frontend:" + backendName + "." + domain
	if err = r.addRoutes(frontend, toAdd); err != nil {
		return err
	}
	cnames, err := r.getCNames(backendName)
	if err != nil {
		log.Errorf("error on get cname in add route for %s - %v", backendName, addresses)
		return err
	}
	if cnames == nil {
		return nil
	}
	for _, cname := range cnames {
		err = r.addRoutes("frontend:"+cname, toAdd)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *hipacheRouter) addRoute(name, address string) error {
	conn, err := r.connect()
	if err != nil {
		return &router.RouterError{Op: "add", Err: err}
	}
	err = conn.RPush(name, address).Err()
	if err != nil {
		log.Errorf("error on store in redis in add route for %s - %s", name, address)
		return &router.RouterError{Op: "add", Err: err}
	}
	return nil
}

func (r *hipacheRouter) addRoutes(name string, addresses []string) error {
	conn, err := r.connect()
	if err != nil {
		return &router.RouterError{Op: "add", Err: err}
	}
	err = conn.RPush(name, addresses...).Err()
	if err != nil {
		log.Errorf("error on store in redis in add route for %s - %v", name, addresses)
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

func (r *hipacheRouter) RemoveRoutes(name string, addresses []*url.URL) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	domain, err := config.GetString(r.prefix + ":domain")
	if err != nil {
		return &router.RouterError{Op: "remove", Err: err}
	}
	toRemove := make([]string, len(addresses))
	for i := range addresses {
		toRemove[i] = addresses[i].String()
	}
	frontend := "frontend:" + backendName + "." + domain
	err = r.removeElements(frontend, toRemove)
	if err != nil {
		return err
	}
	cnames, err := r.getCNames(backendName)
	if err != nil {
		return &router.RouterError{Op: "remove", Err: err}
	}
	if cnames == nil {
		return nil
	}
	for _, cname := range cnames {
		err = r.removeElements("frontend:"+cname, toRemove)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *hipacheRouter) HealthCheck() error {
	conn, err := r.connect()
	if err != nil {
		return err
	}
	result, err := conn.Ping().Result()
	if err != nil {
		return err
	}
	if result != "PONG" {
		return fmt.Errorf("unexpected PING response from Redis server, want %q, got %q", "PONG", result)
	}
	return nil
}

func (r *hipacheRouter) getCNames(name string) ([]string, error) {
	conn, err := r.connect()
	if err != nil {
		return nil, &router.RouterError{Op: "getCName", Err: err}
	}
	cnames, err := conn.LRange("cname:"+name, 0, -1).Result()
	if err != nil && err != redis.Nil {
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
	conn, err := r.connect()
	if err != nil {
		return &router.RouterError{Op: "set", Err: err}
	}
	cnameExists := false
	currentCnames, err := conn.LRange("cname:"+backendName, 0, -1).Result()
	for _, n := range currentCnames {
		if n == cname {
			cnameExists = true
			break
		}
	}
	if !cnameExists {
		err = conn.RPush("cname:"+backendName, cname).Err()
		if err != nil {
			return &router.RouterError{Op: "set", Err: err}
		}
	}
	frontend := "frontend:" + backendName + "." + domain
	cnameFrontend := "frontend:" + cname
	wantedRoutes, err := conn.LRange(frontend, 1, -1).Result()
	if err != nil {
		return &router.RouterError{Op: "get", Err: err}
	}
	currentRoutes, err := conn.LRange(cnameFrontend, 0, -1).Result()
	if err != nil {
		return &router.RouterError{Op: "get", Err: err}
	}
	// Routes are always added again and duplicates removed this will ensure
	// that after a call to SetCName is made routes will be identical to the
	// original entry.
	if len(currentRoutes) == 0 {
		err = conn.RPush(cnameFrontend, backendName).Err()
		if err != nil {
			return &router.RouterError{Op: "setCName", Err: err}
		}
	} else {
		currentRoutes = currentRoutes[1:]
	}
	for _, r := range wantedRoutes {
		err = conn.RPush(cnameFrontend, r).Err()
		if err != nil {
			return &router.RouterError{Op: "setCName", Err: err}
		}
	}
	for _, r := range currentRoutes {
		err = conn.LRem(cnameFrontend, 1, r).Err()
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
	conn, err := r.connect()
	if err != nil {
		return &router.RouterError{Op: "unsetCName", Err: err}
	}
	currentCnames, err := conn.LRange("cname:"+backendName, 0, -1).Result()
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
	err = conn.LRem("cname:"+backendName, 0, cname).Err()
	if err != nil {
		return &router.RouterError{Op: "unsetCName", Err: err}
	}
	err = conn.Del("frontend:" + cname).Err()
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
	conn, err := r.connect()
	if err != nil {
		return "", &router.RouterError{Op: "get", Err: err}
	}
	backends, err := conn.LRange(frontend, 0, 0).Result()
	if err != nil {
		return "", &router.RouterError{Op: "get", Err: err}
	}
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
	conn, err := r.connect()
	if err != nil {
		return nil, &router.RouterError{Op: "routes", Err: err}
	}
	routes, err := conn.LRange(frontend, 0, -1).Result()
	if err != nil {
		return nil, &router.RouterError{Op: "routes", Err: err}
	}
	if len(routes) == 0 {
		return nil, router.ErrBackendNotFound
	}
	routes = routes[1:]
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
	conn, err := r.connect()
	if err != nil {
		return 0, &router.RouterError{Op: "remove", Err: err}
	}
	count, err := conn.LRem(name, 0, address).Result()
	if err != nil {
		return 0, &router.RouterError{Op: "remove", Err: err}
	}
	return int(count), nil
}

func (r *hipacheRouter) removeElements(name string, addresses []string) error {
	conn, err := r.connect()
	if err != nil {
		return &router.RouterError{Op: "remove", Err: err}
	}
	pipe := conn.Pipeline()
	defer pipe.Close()
	for _, addr := range addresses {
		pipe.LRem(name, 0, addr)
	}
	_, err = pipe.Exec()
	if err != nil {
		return &router.RouterError{Op: "remove", Err: err}
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
	return fmt.Sprintf("hipache router %q with redis at %q.", domain, "TODO"), nil
}

func (r *hipacheRouter) SetHealthcheck(name string, data router.HealthcheckData) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	domain, err := config.GetString(r.prefix + ":domain")
	if err != nil {
		return &router.RouterError{Op: "setHealthcheck", Err: err}
	}
	conn, err := r.connect()
	if err != nil {
		return &router.RouterError{Op: "setHealthcheck", Err: err}
	}
	healthcheck := "healthcheck:" + backendName + "." + domain
	err = conn.HMSetMap(healthcheck, map[string]string{
		"path":   data.Path,
		"body":   data.Body,
		"status": strconv.Itoa(data.Status),
	}).Err()
	if err != nil {
		return &router.RouterError{Op: "setHealthcheck", Err: err}
	}
	return nil
}
