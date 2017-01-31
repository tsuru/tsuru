// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"net/http/pprof"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/permission"
)

// title: profile index handler
// path: /debug/pprof
// method: GET
// responses:
//   200: Ok
//   401: Unauthorized
func indexHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermDebug) {
		return permission.ErrUnauthorized
	}
	pprof.Index(w, r)
	return nil
}

// title: profile cmdline handler
// path: /debug/pprof/cmdline
// method: GET
// responses:
//   200: Ok
//   401: Unauthorized
func cmdlineHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermDebug) {
		return permission.ErrUnauthorized
	}
	pprof.Cmdline(w, r)
	return nil
}

// title: profile handler
// path: /debug/pprof/profile
// method: GET
// responses:
//   200: Ok
//   401: Unauthorized
func profileHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermDebug) {
		return permission.ErrUnauthorized
	}
	pprof.Profile(w, r)
	return nil
}

// title: profile symbol handler
// path: /debug/pprof/symbol
// method: GET
// responses:
//   200: Ok
//   401: Unauthorized
func symbolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermDebug) {
		return permission.ErrUnauthorized
	}
	pprof.Symbol(w, r)
	return nil
}

// title: profile trace handler
// path: /debug/pprof/trace
// method: GET
// responses:
//   200: Ok
//   401: Unauthorized
func traceHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermDebug) {
		return permission.ErrUnauthorized
	}
	pprof.Trace(w, r)
	return nil
}
