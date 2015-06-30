// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"strconv"

	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"golang.org/x/net/websocket"
)

func remoteShellHandler(ws *websocket.Conn) {
	var httpErr *errors.HTTP
	defer func() {
		defer ws.Close()
		if httpErr != nil {
			var msg string
			switch httpErr.Code {
			case http.StatusUnauthorized:
				msg = "no token provided or session expired, please login again"
			default:
				msg = httpErr.Message
			}
			ws.Write([]byte("Error: " + msg))
		}
	}()
	r := ws.Request()
	token := context.GetAuthToken(r)
	if token == nil {
		httpErr = &errors.HTTP{
			Code:    http.StatusUnauthorized,
			Message: "no token provided",
		}
		return
	}
	user, err := token.User()
	if err != nil {
		httpErr = &errors.HTTP{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		}
		return
	}
	appName := r.URL.Query().Get(":appname")
	app, err := getApp(appName, user, r)
	if err != nil {
		if herr, ok := err.(*errors.HTTP); ok {
			httpErr = herr
		} else {
			httpErr = &errors.HTTP{
				Code:    http.StatusInternalServerError,
				Message: err.Error(),
			}
		}
		return
	}
	unitID := r.URL.Query().Get("unit")
	width, _ := strconv.Atoi(r.URL.Query().Get("width"))
	height, _ := strconv.Atoi(r.URL.Query().Get("height"))
	term := r.URL.Query().Get("term")
	opts := provision.ShellOptions{
		Conn:   ws,
		Width:  width,
		Height: height,
		Unit:   unitID,
		Term:   term,
	}
	err = app.Shell(opts)
	if err != nil {
		httpErr = &errors.HTTP{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		}
	}
}
