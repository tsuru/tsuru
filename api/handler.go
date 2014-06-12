// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/gorilla/mux"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"net/http"
	"net/url"
)

var (
	tokenRequiredErr = &errors.HTTP{
		Code:    http.StatusUnauthorized,
		Message: "You must provide the Authorization header",
	}
	adminRequiredErr = &errors.HTTP{
		Code:    http.StatusForbidden,
		Message: "You must be an admin",
	}
)

type Router struct {
	mux.Router
}

func registerVars(r *http.Request, vars map[string]string) {
	values := make(url.Values)
	for key, value := range vars {
		values[":"+key] = []string{value}
	}
	r.URL.RawQuery = url.Values(values).Encode() + "&" + r.URL.RawQuery
}

func (r *Router) Add(method string, path string, h http.Handler) *mux.Route {
	return r.Router.Handle(path, h).Methods(method)
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var match mux.RouteMatch
	if !r.Match(req, &match) {
		http.NotFound(w, req)
		return
	}
	registerVars(req, match.Vars)
	SetDelayedHandler(req, match.Handler)
}

type Handler func(http.ResponseWriter, *http.Request) error

func (fn Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	AddRequestError(r, fn(w, r))
}

type authorizationRequiredHandler func(http.ResponseWriter, *http.Request, auth.Token) error

func (fn authorizationRequiredHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t := GetAuthToken(r)
	if t == nil {
		AddRequestError(r, tokenRequiredErr)
	} else {
		AddRequestError(r, fn(w, r, t))
	}
}

type AdminRequiredHandler authorizationRequiredHandler

func (fn AdminRequiredHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t := GetAuthToken(r)
	if t == nil {
		AddRequestError(r, tokenRequiredErr)
	} else if user, err := t.User(); err != nil || !user.IsAdmin() {
		AddRequestError(r, adminRequiredErr)
	} else {
		AddRequestError(r, fn(w, r, t))
	}
}
