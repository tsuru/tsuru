// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/service"
)

func deploy(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	var file multipart.File
	var err error
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		file, _, err = r.FormFile("file")
		if err != nil {
			return &errors.HTTP{
				Code:    http.StatusBadRequest,
				Message: err.Error(),
			}
		}
	}
	version := r.PostFormValue("version")
	archiveURL := r.PostFormValue("archive-url")
	if version == "" && archiveURL == "" && file == nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "you must specify either the version, the archive-url or upload a file",
		}
	}
	if version != "" && archiveURL != "" {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "you must specify either the version or the archive-url, but not both",
		}
	}
	commit := r.PostFormValue("commit")
	w.Header().Set("Content-Type", "text")
	appName := r.URL.Query().Get(":appname")
	var userName string
	var instance *app.App
	if t.IsAppToken() {
		if t.GetAppName() != appName && t.GetAppName() != app.InternalAppName {
			return &errors.HTTP{Code: http.StatusUnauthorized, Message: "invalid app token"}
		}
		instance, err = app.GetByName(appName)
		if err != nil {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		userName = r.PostFormValue("user")
	} else {
		user, err := t.User()
		if err != nil {
			return err
		}
		app, err := getApp(appName, user)
		if err != nil {
			return err
		}
		instance = &app
		userName = t.GetUserName()
	}
	writer := io.NewKeepAliveWriter(w, 30*time.Second, "please wait...")
	err = app.Deploy(app.DeployOptions{
		App:          instance,
		Version:      version,
		Commit:       commit,
		File:         file,
		ArchiveURL:   archiveURL,
		OutputStream: writer,
		User:         userName,
	})
	if err == nil {
		fmt.Fprintln(w, "\nOK")
	}
	return err
}

func deployRollback(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	appName := r.URL.Query().Get(":appname")
	instance, err := app.GetByName(appName)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf("App %s not found.", appName)}
	}
	image := r.PostFormValue("image")
	if image == "" {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "you cannot rollback without an image name",
		}
	}
	w.Header().Set("Content-Type", "application/json")
	writer := &io.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(w)}
	err = app.Deploy(app.DeployOptions{
		App:          instance,
		OutputStream: writer,
		Image:        image,
		User:         t.GetUserName(),
	})
	if err != nil {
		writer.Encode(io.SimpleJsonMessage{Error: err.Error()})
	}
	return nil
}

func deploysList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	var s *service.Service
	var a *app.App
	appName := r.URL.Query().Get("app")
	if appName != "" {
		a, err = app.GetByName(appName)
		if err != nil {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
	}
	serviceName := r.URL.Query().Get("service")
	if serviceName != "" {
		srv, err := getServiceOrError(serviceName, u)
		s = &srv
		if err != nil {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
	}
	skip := r.URL.Query().Get("skip")
	limit := r.URL.Query().Get("limit")
	skipInt, _ := strconv.Atoi(skip)
	limitInt, _ := strconv.Atoi(limit)
	deploys, err := app.ListDeploys(a, s, u, skipInt, limitInt)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(deploys)
}

func deployInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	depId := r.URL.Query().Get(":deploy")
	deploy, err := app.GetDeploy(depId, u)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	diff, err := app.GetDiffInDeploys(deploy)
	if err != nil {
		return err
	}
	data := map[string]interface{}{
		"Id":        deploy.ID.Hex(),
		"App":       deploy.App,
		"Timestamp": deploy.Timestamp.Format(time.RFC3339),
		"Duration":  deploy.Duration.Nanoseconds(),
		"Commit":    deploy.Commit,
		"Error":     deploy.Error,
		"Diff":      diff,
	}
	return json.NewEncoder(w).Encode(data)
}
