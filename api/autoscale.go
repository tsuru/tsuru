// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
)

func autoScaleHistoryHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	appName := r.URL.Query().Get("app")
	history, err := app.ListAutoScaleHistory(appName)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(history)
}
