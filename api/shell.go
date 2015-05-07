// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"strconv"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
)

func remoteShellHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	unitID := r.URL.Query().Get("unit")
	width, _ := strconv.Atoi(r.URL.Query().Get("width"))
	height, _ := strconv.Atoi(r.URL.Query().Get("height"))
	term := r.URL.Query().Get("term")
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	app, err := getApp(appName, u)
	if err != nil {
		return err
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		return &errors.HTTP{
			Code:    http.StatusInternalServerError,
			Message: "cannot hijack connection",
		}
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		}
	}
	defer conn.Close()
	opts := provision.ShellOptions{
		Conn:   conn,
		Width:  width,
		Height: height,
		Unit:   unitID,
		Term:   term,
	}
	return app.Shell(opts)
}
