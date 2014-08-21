// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
)

func logRemove(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	appName := r.URL.Query().Get("app")
	if appName != "" {
		u, err := t.User()
		if err != nil {
			return err
		}
		a, err := getApp(r.URL.Query().Get("app"), u)
		if err != nil {
			return err
		}
		return app.LogRemove(&a)
	}
	return app.LogRemove(nil)
}
