// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
)

func platformAdd(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	defer r.Body.Close()
	name := r.FormValue("name")
	file, _, _ := r.FormFile("dockerfile_content")
	if file != nil {
		defer file.Close()
	}
	args := make(map[string]string)
	for key, values := range r.Form {
		args[key] = values[0]
	}
	canCreatePlatform := permission.Check(t, permission.PermPlatformCreate)
	if !canCreatePlatform {
		return permission.ErrUnauthorized
	}
	w.Header().Set("Content-Type", "text")
	keepAliveWriter := io.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &io.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	err := app.PlatformAdd(provision.PlatformOptions{
		Name:   name,
		Args:   args,
		Input:  file,
		Output: writer,
	})
	if err != nil {
		writer.Encode(io.SimpleJsonMessage{Error: err.Error()})
		writer.Write([]byte("Failed to add platform!\n"))
		return nil
	}
	writer.Write([]byte("Platform successfully added!\n"))
	return nil
}

func platformUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	defer r.Body.Close()
	name := r.URL.Query().Get(":name")
	file, _, _ := r.FormFile("dockerfile_content")
	if file != nil {
		defer file.Close()
	}
	args := make(map[string]string)
	for key, values := range r.Form {
		args[key] = values[0]
	}
	canUpdatePlatform := permission.Check(t, permission.PermPlatformUpdate)
	if !canUpdatePlatform {
		return permission.ErrUnauthorized
	}
	w.Header().Set("Content-Type", "text")
	keepAliveWriter := io.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &io.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	err := app.PlatformUpdate(provision.PlatformOptions{
		Name:   name,
		Args:   args,
		Input:  file,
		Output: writer,
	})
	if err != nil {
		writer.Encode(io.SimpleJsonMessage{Error: err.Error()})
		writer.Write([]byte("Failed to update platform!\n"))
		return nil
	}
	writer.Write([]byte("Platform successfully updated!\n"))
	return nil
}

func platformRemove(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	canDeletePlatform := permission.Check(t, permission.PermPlatformDelete)
	if !canDeletePlatform {
		return permission.ErrUnauthorized
	}
	name := r.URL.Query().Get(":name")
	return app.PlatformRemove(name)
}
