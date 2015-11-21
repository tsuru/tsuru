// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"net/http/pprof"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/permission"
)

func indexHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermDebug) {
		return permission.ErrUnauthorized
	}
	pprof.Index(w, r)
	return nil
}

func cmdlineHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermDebug) {
		return permission.ErrUnauthorized
	}
	pprof.Cmdline(w, r)
	return nil
}

func profileHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermDebug) {
		return permission.ErrUnauthorized
	}
	pprof.Profile(w, r)
	return nil
}

func symbolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermDebug) {
		return permission.ErrUnauthorized
	}
	pprof.Symbol(w, r)
	return nil
}
