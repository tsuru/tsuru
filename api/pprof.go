// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/permission"
)

func debugHandler(h http.HandlerFunc) func(http.ResponseWriter, *http.Request, auth.Token) error {
	return debugHandlerInt(h)
}

func debugHandlerInt(h http.Handler) func(http.ResponseWriter, *http.Request, auth.Token) error {
	return func(w http.ResponseWriter, r *http.Request, t auth.Token) error {
		if !permission.Check(t, permission.PermDebug) {
			return permission.ErrUnauthorized
		}
		h.ServeHTTP(w, r)
		return nil
	}
}
