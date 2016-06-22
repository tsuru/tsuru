// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fusis

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"

	fusisApi "github.com/luizbafilho/fusis/api"
	fusisTypes "github.com/luizbafilho/fusis/api/types"
	"github.com/tsuru/config"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/router"
)

const routerType = "fusis"

var slugReplace = regexp.MustCompile(`[^\w\d]+`)

type fusisRouter struct {
	apiUrl    string
	proto     string
	port      uint16
	scheduler string
	mode      string
	client    *fusisApi.Client
}

func init() {
	router.Register(routerType, createRouter)
}

func createRouter(routerName, configPrefix string) (router.Router, error) {
	apiUrl, err := config.GetString(configPrefix + ":api-url")
	if err != nil {
		return nil, err
	}
	client := fusisApi.NewClient(apiUrl)
	client.HttpClient = tsuruNet.Dial5Full60ClientNoKeepAlive
	r := &fusisRouter{
		apiUrl:    apiUrl,
		client:    client,
		proto:     "tcp",
		port:      80,
		scheduler: "rr",
		mode:      "nat",
	}
	return r, nil
}

func (r *fusisRouter) AddBackend(name string) error {
	srv := fusisTypes.Service{
		Name:      name,
		Port:      r.port,
		Protocol:  r.proto,
		Scheduler: r.scheduler,
	}
	_, err := r.client.CreateService(srv)
	if err != nil {
		if err == fusisTypes.ErrServiceAlreadyExists {
			return router.ErrBackendExists
		}
		return err
	}
	return router.Store(name, name, routerType)
}

func (r *fusisRouter) RemoveBackend(name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	if backendName != name {
		return router.ErrBackendSwapped
	}
	err = r.client.DeleteService(backendName)
	if err == fusisTypes.ErrServiceNotFound {
		return router.ErrBackendNotFound
	}
	return err
}

func (r *fusisRouter) routeName(name string, address *url.URL) string {
	addr := slugReplace.ReplaceAllString(address.Host, "_")
	return fmt.Sprintf("%s_%s", name, addr)
}

func (r *fusisRouter) AddRoute(name string, address *url.URL) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	host, port, err := net.SplitHostPort(address.Host)
	if err != nil {
		host = address.Host
		port = "80"
	}
	portInt, _ := strconv.ParseUint(port, 10, 16)
	dst := fusisTypes.Destination{
		Name:      r.routeName(backendName, address),
		Host:      host,
		Port:      uint16(portInt),
		Mode:      r.mode,
		ServiceId: backendName,
	}
	_, err = r.client.AddDestination(dst)
	if err == fusisTypes.ErrDestinationAlreadyExists {
		return router.ErrRouteExists
	}
	return err
}

func (r *fusisRouter) AddRoutes(name string, addresses []*url.URL) error {
	added := make([]*url.URL, 0, len(addresses))
	var err error
	for _, addr := range addresses {
		err := r.AddRoute(name, addr)
		if err == router.ErrRouteExists {
			err = nil
			continue
		}
		if err != nil {
			break
		}
		added = append(added, addr)
	}
	if err != nil {
		for _, addr := range added {
			r.RemoveRoute(name, addr)
		}
		return err
	}
	return nil
}

func (r *fusisRouter) RemoveRoute(name string, address *url.URL) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	err = r.client.DeleteDestination(backendName, r.routeName(backendName, address))
	if err == fusisTypes.ErrDestinationNotFound {
		return router.ErrRouteNotFound
	}
	return err
}

func (r *fusisRouter) RemoveRoutes(name string, addresses []*url.URL) error {
	removed := make([]*url.URL, 0, len(addresses))
	var err error
	for _, addr := range addresses {
		err := r.RemoveRoute(name, addr)
		if err == router.ErrRouteNotFound {
			err = nil
			continue
		}
		if err != nil {
			break
		}
		removed = append(removed, addr)
	}
	if err != nil {
		for _, addr := range removed {
			r.AddRoute(name, addr)
		}
		return err
	}
	return nil
}

func (r *fusisRouter) findService(name string) (*fusisTypes.Service, error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return nil, err
	}
	srv, err := r.client.GetService(backendName)
	if err != nil {
		if err == fusisTypes.ErrServiceNotFound {
			return nil, router.ErrBackendNotFound
		}
		return nil, err
	}
	return srv, nil
}

func (r *fusisRouter) Addr(name string) (string, error) {
	srv, err := r.findService(name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", srv.Host, srv.Port), nil
}

func (r *fusisRouter) Swap(backend1 string, backend2 string, cnameOnly bool) error {
	return router.Swap(r, backend1, backend2, cnameOnly)
}

func (r *fusisRouter) Routes(name string) ([]*url.URL, error) {
	srv, err := r.findService(name)
	if err != nil {
		return nil, err
	}
	result := make([]*url.URL, len(srv.Destinations))
	for i, d := range srv.Destinations {
		var err error
		result[i] = &url.URL{
			Scheme: srv.Protocol,
			Host:   fmt.Sprintf("%s:%d", d.Host, d.Port),
		}
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
