// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/permission"
	authTypes "github.com/tsuru/tsuru/types/auth"
)

// title: app token list
// path: /apps/{app}/tokens
// method: GET
// produce: application/json
// responses:
//   200: List app tokens
//   204: No content
//   401: Unauthorized
func appTokenList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	app, err := getAppFromContext(r.URL.Query().Get(":app"), r)
	if err != nil {
		return err
	}
	canRead := permission.Check(t, permission.PermAppRead,
		contextsForApp(&app)...,
	)
	if !canRead {
		return permission.ErrUnauthorized
	}

	appTokens, err := auth.AppTokenService().FindByAppName(app.Name)
	if err != nil {
		return err
	}
	if len(appTokens) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(appTokens)
}
