// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/io"
)

func platformAdd(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	name := r.FormValue("name")
	args := make(map[string]string)
	for key, values := range r.Form {
		args[key] = values[0]
	}
	w.Header().Set("Content-Type", "text")
	writer := io.NewKeepAliveWriter(w, 30*time.Second, "please wait...")
	err := app.PlatformAdd(name, args, writer)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "\nOK!")
	return nil
}

func platformUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	name := r.URL.Query().Get(":name")
	err := r.ParseForm()
	if err != nil {
		return err
	}
	args := make(map[string]string)
	for key, values := range r.Form {
		args[key] = values[0]
	}
	w.Header().Set("Content-Type", "text")
	writer := io.NewKeepAliveWriter(w, 30*time.Second, "please wait...")
	err = app.PlatformUpdate(name, args, writer)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "\nOK!")
	return nil
}

func platformRemove(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	name := r.URL.Query().Get(":name")
	return app.PlatformRemove(name)
}
