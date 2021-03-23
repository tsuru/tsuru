// Copyright 2013 tsuru authors. All rights reserved.
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
// hipache" or "routers:<name>:type = planb" in your config.
package hipache

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/hc"
	"github.com/tsuru/tsuru/log"
	tsuruRedis "github.com/tsuru/tsuru/redis"
	"github.com/tsuru/tsuru/router"
	routerTypes "github.com/tsuru/tsuru/types/router"
	redis "gopkg.in/redis.v3"
)

const routerType = "hipache"

var (
	redisClients    = map[string]tsuruRedis.Client{}
	redisClientsMut sync.RWMutex
)

var (
	_ router.Router        = &hipacheRouter{}
	_ router.HealthChecker = &hipacheRouter{}
	_ router.CNameRouter   = &hipacheRouter{}
	_ router.TLSRouter     = &planbRouter{}
)

func init() {
	router.Register(routerType, createHipacheRouter)
	router.Register("planb", createPlanbRouter)
	hc.AddChecker("Router Hipache", router.BuildHealthCheck("hipache"))
	hc.AddChecker("Router Planb", router.BuildHealthCheck("planb"))
}

func createHipacheRouter(routerName string, config router.ConfigGetter) (router.Router, error) {
	return &hipacheRouter{config: config, routerName: routerName}, nil
}

func createPlanbRouter(routerName string, config router.ConfigGetter) (router.Router, error) {
	return &planbRouter{hipacheRouter{config: config, routerName: routerName}}, nil
}

func (r *hipacheRouter) connect() (tsuruRedis.Client, error) {
	key, err := r.config.Hash()
	if err != nil {
		return nil, err
	}
	redisClientsMut.RLock()
	client := redisClients[key]
	if client == nil {
		redisClientsMut.RUnlock()
		redisClientsMut.Lock()
		defer redisClientsMut.Unlock()
		client = redisClients[key]
		if client == nil {
			var err error
			client, err = tsuruRedis.NewRedisDefaultConfig(r.routerName, r.config, &tsuruRedis.CommonConfig{
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
			redisClients[key] = client
		}
	} else {
		redisClientsMut.RUnlock()
	}
	return client, nil
}

type hipacheRouter struct {
	routerName string
	config     router.ConfigGetter
}

func (r *hipacheRouter) GetName() string {
	return r.routerName
}

func (r *hipacheRouter) GetType() string {
	return routerType
}

func (r *hipacheRouter) AddBackend(ctx context.Context, app router.App) (err error) {
	name := app.GetName()
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	domain, err := r.config.GetString("domain")
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

func (r *hipacheRouter) RemoveBackend(ctx context.Context, app router.App) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return err
	}
	if backendName != app.GetName() {
		return router.ErrBackendSwapped
	}
	domain, err := r.config.GetString("domain")
	if err != nil {
		return &router.RouterError{Op: "remove", Err: err}
	}
	frontend := "frontend:" + backendName + "." + domain
	conn, err := r.connect()
	if err != nil {
		return &router.RouterError{Op: "remove", Err: err}
	}
	deleted, err := conn.Del(frontend).Result()
	if err != nil {
		return &router.RouterError{Op: "remove", Err: err}
	}
	if deleted == 0 {
		return router.ErrBackendNotFound
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
	healthcheck := "healthcheck:" + backendName + "." + domain
	err = conn.Del(healthcheck).Err()
	if err != nil {
		return &router.RouterError{Op: "remove", Err: err}
	}
	return nil
}

func (r *hipacheRouter) AddRoutes(ctx context.Context, app router.App, addresses []*url.URL) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return err
	}
	domain, err := r.config.GetString("domain")
	if err != nil {
		log.Errorf("error on getting hipache domain in add route for %s - %v", backendName, addresses)
		return &router.RouterError{Op: "add", Err: err}
	}
	routes, err := r.Routes(ctx, app)
	if err != nil {
		return err
	}
	toAdd := make([]string, 0, len(addresses))
addresses:
	for _, addr := range addresses {
		addr.Scheme = router.HttpScheme
		for _, r := range routes {
			if r.Host == addr.Host {
				continue addresses
			}
		}
		toAdd = append(toAdd, addr.String())
	}
	if len(toAdd) == 0 {
		log.Debugf("[add-routes] no new routes to add for %q", app.GetName())
		return nil
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

func (r *hipacheRouter) RemoveRoutes(ctx context.Context, app router.App, addresses []*url.URL) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return err
	}
	domain, err := r.config.GetString("domain")
	if err != nil {
		return &router.RouterError{Op: "remove", Err: err}
	}
	toRemove := make([]string, len(addresses))
	for i := range addresses {
		addresses[i].Scheme = router.HttpScheme
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

func (r *hipacheRouter) HealthCheck(ctx context.Context) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	conn, err := r.connect()
	if err != nil {
		return err
	}
	result, err := conn.Ping().Result()
	if err != nil {
		return err
	}
	if result != "PONG" {
		return errors.Errorf("unexpected PING response from Redis server, want %q, got %q", "PONG", result)
	}
	return nil
}

func (r *hipacheRouter) CNames(ctx context.Context, app router.App) (urls []*url.URL, err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	cnames, err := r.getCNames(app.GetName())
	if err != nil {
		return nil, err
	}
	urls = make([]*url.URL, len(cnames))
	for i, cname := range cnames {
		urls[i] = &url.URL{Host: cname}
	}
	return urls, nil
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

func (r *hipacheRouter) SetCName(ctx context.Context, cname string, app router.App) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return err
	}
	domain, err := r.config.GetString("domain")
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
	if err != nil {
		return &router.RouterError{Op: "set", Err: err}
	}
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

func (r *hipacheRouter) UnsetCName(ctx context.Context, cname string, app router.App) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return err
	}
	conn, err := r.connect()
	if err != nil {
		return &router.RouterError{Op: "unsetCName", Err: err}
	}
	currentCnames, err := conn.LRange("cname:"+backendName, 0, -1).Result()
	if err != nil {
		return &router.RouterError{Op: "unsetCName", Err: err}
	}
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

func (r *hipacheRouter) Addr(ctx context.Context, app router.App) (addr string, err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return "", err
	}
	domain, err := r.config.GetString("domain")
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

func (r *hipacheRouter) Routes(ctx context.Context, app router.App) (urls []*url.URL, err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return nil, err
	}
	domain, err := r.config.GetString("domain")
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
	urls = make([]*url.URL, len(routes))
	for i, route := range routes {
		urls[i], err = url.Parse(route)
		if err != nil {
			return nil, err
		}
	}
	return urls, nil
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

func (r *hipacheRouter) Swap(ctx context.Context, app1, app2 router.App, cnameOnly bool) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	return router.Swap(ctx, r, app1, app2, cnameOnly)
}

func (r *hipacheRouter) StartupMessage() (string, error) {
	domain, err := r.config.GetString("domain")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("hipache router %q with redis at %q.", domain, "TODO"), nil
}

func (r *hipacheRouter) SetHealthcheck(ctx context.Context, app router.App, data routerTypes.HealthcheckData) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	backendName, err := router.Retrieve(app.GetName())
	if err != nil {
		return err
	}
	domain, err := r.config.GetString("domain")
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

type planbRouter struct {
	hipacheRouter
}

func (r *planbRouter) AddCertificate(ctx context.Context, _ router.App, cname, cert, key string) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	conn, err := r.connect()
	if err != nil {
		return &router.RouterError{Op: "addCertificate", Err: err}
	}
	err = conn.HMSetMap("tls:"+cname, map[string]string{
		"certificate": cert,
		"key":         key,
	}).Err()
	if err != nil {
		return &router.RouterError{Op: "addCertificate", Err: err}
	}
	return nil
}

func (r *planbRouter) RemoveCertificate(ctx context.Context, _ router.App, cname string) (err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	conn, err := r.connect()
	if err != nil {
		return &router.RouterError{Op: "removeCertificate", Err: err}
	}
	err = conn.Del("tls:" + cname).Err()
	if err != nil {
		return &router.RouterError{Op: "removeCertificate", Err: err}
	}
	return nil
}

func (r *planbRouter) GetCertificate(ctx context.Context, _ router.App, cname string) (cert string, err error) {
	done := router.InstrumentRequest(r.routerName)
	defer func() {
		done(err)
	}()
	conn, err := r.connect()
	if err != nil {
		return "", &router.RouterError{Op: "getCertificate", Err: err}
	}
	result, err := conn.HMGet("tls:"+cname, "certificate").Result()
	if err != nil {
		return "", &router.RouterError{Op: "getCertificate", Err: err}
	}
	if len(result) == 0 || result[0] == nil {
		return "", router.ErrCertificateNotFound
	}
	return result[0].(string), nil
}
