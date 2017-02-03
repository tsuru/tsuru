// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/router"
)

// title: router list
// path: /routers
// method: GET
// produce: application/json
// responses:
//   200: OK
//   204: No content
func listRouters(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowed := permission.Check(t, permission.PermAppCreate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	routers, err := router.List()
	if err != nil {
		return err
	}
	if len(routers) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(routers)
}
