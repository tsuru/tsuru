// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/event"
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
	canRead := permission.Check(t, permission.PermAppTokenRead,
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

// title: app token create
// path: /apps/{app}/tokens
// method: POST
// produce: application/json
// responses:
//   201: App token created
//   401: Unauthorized
//   409: App token already exists
func appTokenCreate(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	appName := r.URL.Query().Get(":app")
	app, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppTokenCreate,
		contextsForApp(&app)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}

	appToken := authTypes.NewAppToken(appName, t.GetUserName())
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppTokenCreate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&app)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()

	err = auth.AppTokenService().Insert(appToken)
	if err == authTypes.ErrAppTokenAlreadyExists {
		w.WriteHeader(http.StatusConflict)
	} else if err == nil {
		w.WriteHeader(http.StatusCreated)
	}
	return err
}

// title: app token delete
// path: /apps/{app}/tokens/{token}
// method: DELETE
// produce: application/json
// responses:
//   200: App token created
//   401: Unauthorized
//   404: App token not found
func appTokenDelete(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	appName := r.URL.Query().Get(":app")
	app, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppTokenDelete,
		contextsForApp(&app)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}

	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppTokenDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&app)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()

	token := r.URL.Query().Get(":token")
	appToken := authTypes.AppToken{Token: token}
	err = auth.AppTokenService().Delete(appToken)
	if err == authTypes.ErrAppTokenNotFound {
		w.WriteHeader(http.StatusNotFound)
	}
	return err
}
