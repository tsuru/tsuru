// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"net/http"
)

func deploysList(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	service := r.URL.Query().Get("service")
	if service != "" {
		s, err := getServiceOrError(service, u)
		if err != nil {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		deploys, err := app.ListDeploys(&s)
		if err != nil {
			return err
		}
		return json.NewEncoder(w).Encode(deploys)
	}
	deploys, err := app.ListDeploys(nil)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(deploys)
}
