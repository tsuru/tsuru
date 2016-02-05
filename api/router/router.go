// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"net/url"

	"github.com/gorilla/mux"
	"github.com/tsuru/tsuru/api/context"
)

// versionMatcher defines a variable matcher to be parsed by the router
// when a request is about to be served.
const versionMatcher = "/{version:[0-9.]+}"

type Route struct {
	route   *mux.Route
	version string
}

func NewRouter() *DelayedRouter {
	return &DelayedRouter{
		mux:    mux.NewRouter(),
		routes: map[*mux.Route]*Route{},
	}
}

type DelayedRouter struct {
	mux    *mux.Router
	routes map[*mux.Route]*Route
}

func (r *DelayedRouter) registerVars(req *http.Request, vars map[string]string) {
	values := make(url.Values)
	for key, value := range vars {
		values[":"+key] = []string{value}
	}
	req.URL.RawQuery = url.Values(values).Encode() + "&" + req.URL.RawQuery
}

func (r *DelayedRouter) Add(version, method, path string, h http.Handler) *mux.Route {
	r.mux.NewRoute().PathPrefix(versionMatcher).Path(path).Handler(h).Methods(method)
	return r.mux.Handle(path, h).Methods(method)
}

// AddAll binds a path to GET, POST, PUT and DELETE methods.
func (r *DelayedRouter) AddAll(version, path string, h http.Handler) *mux.Route {
	return r.mux.Handle(path, h).Methods("GET", "POST", "PUT", "DELETE")
}

func (r *DelayedRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var match mux.RouteMatch
	if !r.mux.Match(req, &match) {
		http.NotFound(w, req)
		return
	}
	r.registerVars(req, match.Vars)
	context.SetDelayedHandler(req, match.Handler)
}
