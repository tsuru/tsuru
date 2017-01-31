// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"net/url"
	"regexp"

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
	req.URL.RawQuery = values.Encode() + "&" + req.URL.RawQuery
}

func (r *DelayedRouter) addRoute(version, path string, h http.Handler, methods ...string) *mux.Route {
	muxRoute := r.mux.NewRoute().Handler(h).Methods(methods...)
	route := &Route{route: muxRoute, version: version}
	r.routes[muxRoute] = route
	versionRegexp := regexp.MustCompile("/(?P<version>[0-9.]+)/")
	muxRoute.MatcherFunc(func(httpRequest *http.Request, rm *mux.RouteMatch) bool {
		d := versionRegexp.FindStringSubmatch(httpRequest.URL.Path)
		return len(d) > 1 && r.routes[muxRoute].version == d[1]
	}).PathPrefix(versionMatcher).Path(path)
	r.mux.NewRoute().Path(path).Handler(h).Methods(methods...)
	return muxRoute
}

func (r *DelayedRouter) Add(version, method, path string, h http.Handler) *mux.Route {
	return r.addRoute(version, path, h, method)
}

// AddAll binds a path to GET, POST, PUT and DELETE methods.
func (r *DelayedRouter) AddAll(version, path string, h http.Handler) *mux.Route {
	return r.addRoute(version, path, h, "GET", "POST", "PUT", "DELETE")
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
