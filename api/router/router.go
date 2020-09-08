// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/gorilla/mux"
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/api/observability"
)

const (
	// versionMatcher defines a variable matcher to be parsed by the router
	// when a request is about to be served.
	versionMatcher = "/{version:[0-9.]+}"

	routeNameVariable    = ":mux-route-name"
	pathTemplateVariable = ":mux-path-template"
)

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

func (r *DelayedRouter) registerMatch(req *http.Request, match mux.RouteMatch) {
	values := make(url.Values)
	routeName := match.Route.GetName()
	if routeName != "" {
		values.Set(routeNameVariable, routeName)
	}
	pathTemplate, _ := match.Route.GetPathTemplate()
	if pathTemplate != "" {
		values.Set(pathTemplateVariable, strings.TrimPrefix(pathTemplate, versionMatcher))
	}

	for key, value := range match.Vars {
		values[":"+key] = []string{value}
	}
	req.URL.RawQuery = values.Encode() + "&" + req.URL.RawQuery
}

func (r *DelayedRouter) addRoute(name, version, path string, h http.Handler, methods ...string) *mux.Route {
	muxRoute := r.mux.NewRoute().Handler(h).Methods(methods...)
	route := &Route{route: muxRoute, version: version}
	r.routes[muxRoute] = route
	versionRegexp := regexp.MustCompile("/(?P<version>[0-9.]+)/")
	versionedRoute := muxRoute.MatcherFunc(func(httpRequest *http.Request, rm *mux.RouteMatch) bool {
		d := versionRegexp.FindStringSubmatch(httpRequest.URL.Path)
		return len(d) > 1 && r.routes[muxRoute].version == d[1]
	}).PathPrefix(versionMatcher).Path(path)
	plainRoute := r.mux.NewRoute().Path(path).Handler(h).Methods(methods...)
	if name != "" {
		plainRoute.Name(name)
		versionedRoute.Name(name)
	}
	for _, method := range methods {
		observability.PrePopulateMetrics(method, path)
	}
	return muxRoute
}

func (r *DelayedRouter) AddNamed(name, version, method, path string, h http.Handler) *mux.Route {
	return r.addRoute(name, version, path, h, method)
}

func (r *DelayedRouter) Add(version, method, path string, h http.Handler) *mux.Route {
	return r.addRoute("", version, path, h, method)
}

// AddAll binds a path to GET, POST, PUT and DELETE methods.
func (r *DelayedRouter) AddAll(version, path string, h http.Handler) *mux.Route {
	return r.addRoute("", version, path, h, "GET", "POST", "PUT", "DELETE")
}

func (r *DelayedRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var match mux.RouteMatch
	if !r.mux.Match(req, &match) {
		http.NotFound(w, req)
		return
	}

	r.registerMatch(req, match)
	observability.StartSpan(req)
	context.SetDelayedHandler(req, match.Handler)
}
