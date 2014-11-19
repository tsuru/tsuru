// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"runtime/pprof"

	"github.com/tsuru/tsuru/auth"
)

func dumpGoroutines(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return pprof.Lookup("goroutine").WriteTo(w, 2)
}
