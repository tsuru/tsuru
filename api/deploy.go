// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/errors"
	"net/http"
)

func deploysList(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	appName := r.URL.Query().Get(":app")
	u, err := t.User()
	if err != nil {
		return err
	}
	a, err := getApp(appName, u)
	if err != nil {
		return err
	}
	deploys, err := a.ListDeploys()
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf("App %s not found.", appName)}
	}
	return json.NewEncoder(w).Encode(deploys)
}
