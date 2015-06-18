// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vulcand

import (
	"crypto/md5"
	"fmt"
	"net/url"

	"github.com/mailgun/vulcand/api"
	"github.com/mailgun/vulcand/engine"
	"github.com/mailgun/vulcand/plugin/registry"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/router"
)

const routerName = "vulcand"

func init() {
	router.Register(routerName, createRouter)
}

type vulcandRouter struct {
	client *api.Client
	prefix string
	domain string
}

func createRouter(prefix string) (router.Router, error) {
	vURL, err := config.GetString(prefix + ":api-url")
	if err != nil {
		return nil, err
	}
	domain, err := config.GetString(prefix + ":domain")
	if err != nil {
		return nil, err
	}
	client := api.NewClient(vURL, registry.GetRegistry())
	vRouter := &vulcandRouter{
		client: client,
		prefix: prefix,
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
		return err
	}
	err = r.client.UpsertBackend(*backend)
	if err != nil {
		return err
	}
	frontend, err := engine.NewHTTPFrontend(
		frontendName,
		backend.Id,
		fmt.Sprintf(`Host(%q) && Path("/")`, r.frontendHostname(name)),
		engine.HTTPFrontendSettings{},
	)
	if err != nil {
		return err
	}
	err = r.client.UpsertFrontend(*frontend, engine.NoTTL)
	if err != nil {
		r.client.DeleteBackend(backendKey)
		return err
	}
	return router.Store(name, name, routerName)
}

func (r *vulcandRouter) RemoveBackend(name string) error {
	frontendKey := engine.FrontendKey{Id: r.frontendName(r.frontendHostname(name))}
	err := r.client.DeleteFrontend(frontendKey)
	if err != nil {
		if _, ok := err.(*engine.NotFoundError); ok {
			return router.ErrBackendNotFound
		}
		return err
	}
	backendKey := engine.BackendKey{Id: r.backendName(name)}
	err = r.client.DeleteBackend(backendKey)
	if err != nil {
		return err
	}
	return router.Remove(name)
}

func (r *vulcandRouter) AddRoute(name string, address *url.URL) error {
	serverKey := engine.ServerKey{
		Id:         r.serverName(address.String()),
		BackendKey: engine.BackendKey{Id: r.backendName(name)},
	}
	if found, _ := r.client.GetServer(serverKey); found != nil {
		return router.ErrRouteExists
	}
	server, err := engine.NewServer(serverKey.Id, address.String())
	if err != nil {
		return err
	}
	return r.client.UpsertServer(serverKey.BackendKey, *server, engine.NoTTL)
}

func (r *vulcandRouter) RemoveRoute(name string, address *url.URL) error {
	serverKey := engine.ServerKey{
		Id:         r.serverName(address.String()),
		BackendKey: engine.BackendKey{Id: r.backendName(name)},
	}
	err := r.client.DeleteServer(serverKey)
	if err != nil {
		if _, ok := err.(*engine.NotFoundError); ok {
			return router.ErrRouteNotFound
		}
	}
	return err
}

func (r *vulcandRouter) SetCName(cname, name string) error {
	frontendName := r.frontendName(cname)
	if found, _ := r.client.GetFrontend(engine.FrontendKey{Id: frontendName}); found != nil {
		return router.ErrRouteExists
	}
	frontend, err := engine.NewHTTPFrontend(
		frontendName,
		r.backendName(name),
		fmt.Sprintf(`Host(%q) && Path("/")`, cname),
		engine.HTTPFrontendSettings{},
	)
	if err != nil {
		return err
	}
	return r.client.UpsertFrontend(*frontend, engine.NoTTL)
}

func (r *vulcandRouter) UnsetCName(cname, name string) error {
	frontendKey := engine.FrontendKey{Id: r.frontendName(cname)}
	err := r.client.DeleteFrontend(frontendKey)
	if err != nil {
		if _, ok := err.(*engine.NotFoundError); ok {
			return router.ErrRouteNotFound
		}
	}
	return err
}

func (r *vulcandRouter) Addr(name string) (string, error) {
	frontendHostname := r.frontendHostname(name)
	frontendKey := engine.FrontendKey{Id: r.frontendName(frontendHostname)}
	if found, _ := r.client.GetFrontend(frontendKey); found == nil {
		return "", router.ErrRouteNotFound
	}
	return frontendHostname, nil
}

func (r *vulcandRouter) Swap(backend1, backend2 string) error {
	return router.Swap(r, backend1, backend2)
}

func (r *vulcandRouter) Routes(name string) ([]*url.URL, error) {
	servers, err := r.client.GetServers(engine.BackendKey{
		Id: r.backendName(name),
	})
	if err != nil {
		return nil, err
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
