// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"runtime/pprof"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/permission"
)

func dumpGoroutines(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermDebug) {
		return permission.ErrUnauthorized
	}
	return pprof.Lookup("goroutine").WriteTo(w, 2)
}
