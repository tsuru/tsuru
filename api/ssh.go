// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"strconv"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
)

func sshHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	containerID := r.URL.Query().Get("container_id")
	width, _ := strconv.Atoi(r.URL.Query().Get("width"))
	height, _ := strconv.Atoi(r.URL.Query().Get("height"))
	hj, ok := w.(http.Hijacker)
	u, err := t.User()
	if err != nil {
		return err
	}
	appName := r.URL.Query().Get(":app")
	app, err := getApp(appName, u)
	if err != nil {
		return err
	}
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
	return app.Ssh(conn, width, height, containerID)
}
