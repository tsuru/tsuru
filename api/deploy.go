// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"net/http"
)

func deploysList(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	deploys, err := app.ListDeploys()
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(deploys)
}
