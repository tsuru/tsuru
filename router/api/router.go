// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"net/url"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/router"
)

const routerType = "api"

var errNotImplemented = errors.New("not implemented")

type apiRouter struct {
	routerName string
	endpoint   string
}

func init() {
	router.Register(routerType, createRouter)
}

func createRouter(routerName, configPrefix string) (router.Router, error) {
	endpoint, err := config.GetString(configPrefix + ":endpoint")
	if err != nil {
		return nil, err
	}
	return &apiRouter{
		routerName: routerName,
		endpoint:   endpoint,
	}, nil
}

func (r *apiRouter) AddBackend(name string) (err error) {
	return errNotImplemented
}

func (r *apiRouter) RemoveBackend(name string) (err error) {
	return errNotImplemented
}

func (r *apiRouter) AddRoute(name string, address *url.URL) (err error) {
	return errNotImplemented
}

func (r *apiRouter) AddRoutes(name string, addresses []*url.URL) (err error) {
	return errNotImplemented
}

func (r *apiRouter) Addr(name string) (addr string, err error) {
	return "", errNotImplemented
}

func (r *apiRouter) RemoveRoute(name string, address *url.URL) (err error) {
	return errNotImplemented
}

func (r *apiRouter) RemoveRoutes(name string, addresses []*url.URL) (err error) {
	return errNotImplemented
}

func (r *apiRouter) Routes(name string) (result []*url.URL, err error) {
	return nil, errNotImplemented
}

func (r *apiRouter) Swap(backend1 string, backend2 string, cnameOnly bool) (err error) {
	return errNotImplemented
}
