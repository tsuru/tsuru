// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !windows

package vulcand

import (
	"crypto/md5"
	"fmt"
	"net/url"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/hc"
	"github.com/tsuru/tsuru/router"
	"github.com/vulcand/route"
	"github.com/vulcand/vulcand/api"
	"github.com/vulcand/vulcand/engine"
	"github.com/vulcand/vulcand/plugin/registry"
)

const routerName = "vulcand"

func init() {
	router.Register(routerName, createRouter)
	hc.AddChecker("Router vulcand", router.BuildHealthCheck("vulcand"))
}

type vulcandRouter struct {
	client *api.Client
	prefix string
	domain string
}

func createRouter(routerName, configPrefix string) (router.Router, error) {
	vURL, err := config.GetString(configPrefix + ":api-url")
	if err != nil {
		return nil, err
	}
	domain, err := config.GetString(configPrefix + ":domain")
	if err != nil {
		return nil, err
	}
	client := api.NewClient(vURL, registry.GetRegistry())
	vRouter := &vulcandRouter{
		client: client,
		prefix: configPrefix,
		domain: domain,
	}
	return vRouter, nil
}

func (r *vulcandRouter) frontendHostname(app string) string {
	return fmt.Sprintf("%s.%s", app, r.domain)
}

func (r *vulcandRouter) frontendName(hostname string) string {
	return fmt.Sprintf("tsuru_%s", hostname)
}

func (r *vulcandRouter) backendName(app string) string {
	return fmt.Sprintf("tsuru_%s", app)
}

func (r *vulcandRouter) serverName(address string) string {
	return fmt.Sprintf("tsuru_%x", md5.Sum([]byte(address)))
}

func (r *vulcandRouter) AddBackend(name string) error {
	backendName := r.backendName(name)
	frontendName := r.frontendName(r.frontendHostname(name))
	backendKey := engine.BackendKey{Id: backendName}
	frontendKey := engine.FrontendKey{Id: frontendName}
	if found, _ := r.client.GetBackend(backendKey); found != nil {
		return router.ErrBackendExists
	}
	if found, _ := r.client.GetFrontend(frontendKey); found != nil {
		return router.ErrBackendExists
	}
	backend, err := engine.NewHTTPBackend(
		backendName,
		engine.HTTPBackendSettings{},
	)
	if err != nil {
		return &router.RouterError{Err: err, Op: "add-backend"}
	}
	err = r.client.UpsertBackend(*backend)
	if err != nil {
		return err
	}
	frontend, err := engine.NewHTTPFrontend(
		route.NewMux(),
		frontendName,
		backend.Id,
		fmt.Sprintf(`Host(%q)`, r.frontendHostname(name)),
		engine.HTTPFrontendSettings{},
	)
	if err != nil {
		return &router.RouterError{Err: err, Op: "add-backend"}
	}
	err = r.client.UpsertFrontend(*frontend, engine.NoTTL)
	if err != nil {
		r.client.DeleteBackend(backendKey)
		return &router.RouterError{Err: err, Op: "add-backend"}
	}
	return router.Store(name, name, routerName)
}

func (r *vulcandRouter) RemoveBackend(name string) error {
	usedName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	if usedName != name {
		return router.ErrBackendSwapped
	}
	backendKey := engine.BackendKey{Id: r.backendName(usedName)}
	frontends, err := r.client.GetFrontends()
	if err != nil {
		return &router.RouterError{Err: err, Op: "remove-backend"}
	}
	toRemove := []engine.FrontendKey{}
	for _, f := range frontends {
		if f.BackendId == backendKey.Id {
			toRemove = append(toRemove, engine.FrontendKey{Id: f.GetId()})
		}
	}
	for _, fk := range toRemove {
		err = r.client.DeleteFrontend(fk)
		if err != nil {
			if _, ok := err.(*engine.NotFoundError); ok {
				return router.ErrBackendNotFound
			}
			return &router.RouterError{Err: err, Op: "remove-backend"}
		}
	}
	routes, err := r.Routes(name)
	if err != nil {
		return err
	}
	for _, route := range routes {
		err = r.RemoveRoute(name, route)
		if err != nil {
			return err
		}
	}
	err = r.client.DeleteBackend(backendKey)
	if err != nil {
		if _, ok := err.(*engine.NotFoundError); ok {
			return router.ErrBackendNotFound
		}
		return &router.RouterError{Err: err, Op: "remove-backend"}
	}
	return router.Remove(usedName)
}

func (r *vulcandRouter) AddRoute(name string, address *url.URL) error {
	usedName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	serverKey := engine.ServerKey{
		Id:         r.serverName(address.Host),
		BackendKey: engine.BackendKey{Id: r.backendName(usedName)},
	}
	if found, _ := r.client.GetServer(serverKey); found != nil {
		return router.ErrRouteExists
	}
	server, err := engine.NewServer(serverKey.Id, address.String())
	if err != nil {
		return &router.RouterError{Err: err, Op: "add-route"}
	}
	err = r.client.UpsertServer(serverKey.BackendKey, *server, engine.NoTTL)
	if err != nil {
		return &router.RouterError{Err: err, Op: "add-route"}
	}
	return nil
}

func (r *vulcandRouter) AddRoutes(name string, addresses []*url.URL) error {
	usedName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	for _, addr := range addresses {
		serverKey := engine.ServerKey{
			Id:         r.serverName(addr.Host),
			BackendKey: engine.BackendKey{Id: r.backendName(usedName)},
		}
		server, err := engine.NewServer(serverKey.Id, addr.String())
		if err != nil {
			return &router.RouterError{Err: err, Op: "add-route"}
		}
		err = r.client.UpsertServer(serverKey.BackendKey, *server, engine.NoTTL)
		if err != nil {
			return &router.RouterError{Err: err, Op: "add-route"}
		}
	}
	return nil
}

func (r *vulcandRouter) RemoveRoute(name string, address *url.URL) error {
	usedName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	serverKey := engine.ServerKey{
		Id:         r.serverName(address.Host),
		BackendKey: engine.BackendKey{Id: r.backendName(usedName)},
	}
	err = r.client.DeleteServer(serverKey)
	if err != nil {
		if _, ok := err.(*engine.NotFoundError); ok {
			return router.ErrRouteNotFound
		}
		return &router.RouterError{Err: err, Op: "remove-route"}
	}
	return nil
}

func (r *vulcandRouter) RemoveRoutes(name string, addresses []*url.URL) error {
	usedName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	for _, addr := range addresses {
		serverKey := engine.ServerKey{
			Id:         r.serverName(addr.Host),
			BackendKey: engine.BackendKey{Id: r.backendName(usedName)},
		}
		err = r.client.DeleteServer(serverKey)
		if err != nil {
			if _, ok := err.(*engine.NotFoundError); ok {
				continue
			}
			return &router.RouterError{Err: err, Op: "remove-route"}
		}
	}
	return nil
}

func (r *vulcandRouter) CNames(name string) ([]*url.URL, error) {
	fes, err := r.client.GetFrontends()
	if err != nil {
		return nil, err
	}
	backendName := r.backendName(name)
	address, err := r.Addr(name)
	if err != nil {
		return nil, err
	}
	address = r.backendName(address)
	urls := []*url.URL{}
	for _, f := range fes {
		host := strings.Replace(f.Id, "tsuru_", "", 1)
		if f.BackendId == backendName && f.Id != address {
			urls = append(urls, &url.URL{Host: host})
		}
	}
	return urls, nil
}

func (r *vulcandRouter) SetCName(cname, name string) error {
	usedName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	if !router.ValidCName(cname, r.domain) {
		return router.ErrCNameNotAllowed
	}
	frontendName := r.frontendName(cname)
	if found, _ := r.client.GetFrontend(engine.FrontendKey{Id: frontendName}); found != nil {
		return router.ErrCNameExists
	}
	frontend, err := engine.NewHTTPFrontend(
		route.NewMux(),
		frontendName,
		r.backendName(usedName),
		fmt.Sprintf(`Host(%q)`, cname),
		engine.HTTPFrontendSettings{},
	)
	if err != nil {
		return &router.RouterError{Err: err, Op: "set-cname"}
	}
	err = r.client.UpsertFrontend(*frontend, engine.NoTTL)
	if err != nil {
		return &router.RouterError{Err: err, Op: "set-cname"}
	}
	return nil
}

func (r *vulcandRouter) UnsetCName(cname, _ string) error {
	frontendKey := engine.FrontendKey{Id: r.frontendName(cname)}
	err := r.client.DeleteFrontend(frontendKey)
	if err != nil {
		if _, ok := err.(*engine.NotFoundError); ok {
			return router.ErrCNameNotFound
		}
		return &router.RouterError{Err: err, Op: "unset-cname"}
	}
	return nil
}

func (r *vulcandRouter) Addr(name string) (string, error) {
	usedName, err := router.Retrieve(name)
	if err != nil {
		return "", err
	}
	frontendHostname := r.frontendHostname(usedName)
	frontendKey := engine.FrontendKey{Id: r.frontendName(frontendHostname)}
	if found, _ := r.client.GetFrontend(frontendKey); found == nil {
		return "", router.ErrRouteNotFound
	}
	return frontendHostname, nil
}

func (r *vulcandRouter) Swap(backend1, backend2 string, cnameOnly bool) error {
	return router.Swap(r, backend1, backend2, cnameOnly)
}

func (r *vulcandRouter) Routes(name string) ([]*url.URL, error) {
	usedName, err := router.Retrieve(name)
	if err != nil {
		return nil, err
	}
	servers, err := r.client.GetServers(engine.BackendKey{
		Id: r.backendName(usedName),
	})
	if err != nil {
		return nil, &router.RouterError{Err: err, Op: "routes"}
	}
	routes := make([]*url.URL, len(servers))
	for i, server := range servers {
		parsedUrl, _ := url.Parse(server.URL)
		routes[i] = parsedUrl
	}
	return routes, nil
}

func (r *vulcandRouter) StartupMessage() (string, error) {
	message := fmt.Sprintf("vulcand router %q with API at %q", r.domain, r.client.Addr)
	return message, nil
}

func (r *vulcandRouter) HealthCheck() error {
	return r.client.GetStatus()
}
