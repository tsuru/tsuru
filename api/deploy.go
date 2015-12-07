// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
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
	image := r.PostFormValue("image")
	if image == "" && version == "" && archiveURL == "" && file == nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "you must specify either the version, the archive-url, a image url or upload a file.",
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
	origin := r.URL.Query().Get("origin")
	if image != "" {
		origin = "image"
	}
	if origin != "" {
		if !validateOrigin(origin) {
			return &errors.HTTP{
				Code:    http.StatusBadRequest,
				Message: "Invalid deployment origin",
			}
		}
	}
	var userName string
	if t.IsAppToken() {
		if t.GetAppName() != appName && t.GetAppName() != app.InternalAppName {
			return &errors.HTTP{Code: http.StatusUnauthorized, Message: "invalid app token"}
		}
		userName = r.PostFormValue("user")
	} else {
		userName = t.GetUserName()
	}
	instance, err := app.GetByName(appName)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	if t.GetAppName() != app.InternalAppName {
		canDeploy := permission.Check(t, permission.PermAppDeploy,
			append(permission.Contexts(permission.CtxTeam, instance.Teams),
				permission.Context(permission.CtxApp, appName),
				permission.Context(permission.CtxPool, instance.Pool),
			)...,
		)
		if !canDeploy {
			return &errors.HTTP{Code: http.StatusForbidden, Message: "User does not have access to this app"}
		}
	}
	writer := io.NewKeepAliveWriter(w, 30*time.Second, "please wait...")
	defer writer.Stop()
	err = app.Deploy(app.DeployOptions{
		App:          instance,
		Version:      version,
		Commit:       commit,
		File:         file,
		ArchiveURL:   archiveURL,
		OutputStream: writer,
		User:         userName,
		Image:        image,
		Origin:       origin,
	})
	if err == nil {
		fmt.Fprintln(w, "\nOK")
	}
	return err
}

func validateOrigin(origin string) bool {
	originList := []string{"app-deploy", "git", "rollback", "drag-and-drop", "image"}
	for _, ol := range originList {
		if ol == origin {
			return true
		}
	}
	return false
}

func diffDeploy(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	writer := io.NewKeepAliveWriter(w, 30*time.Second, "")
	defer writer.Stop()
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Fprint(w, err.Error())
		return err
	}
	fmt.Fprint(w, "Saving the difference between the old and new code\n")
	appName := r.URL.Query().Get(":appname")
	val, err := url.ParseQuery(string(data))
	if err != nil {
		fmt.Fprint(w, err.Error())
		return err
	}
	diff := val.Get("customdata")
	instance, err := app.GetByName(appName)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	if t.GetAppName() != app.InternalAppName {
		canDiffDeploy := permission.Check(t, permission.PermAppReadDeploy,
			append(permission.Contexts(permission.CtxTeam, instance.Teams),
				permission.Context(permission.CtxApp, instance.Name),
				permission.Context(permission.CtxPool, instance.Pool),
			)...,
		)
		if !canDiffDeploy {
			return &errors.HTTP{Code: http.StatusForbidden, Message: permission.ErrUnauthorized.Error()}
		}
	}
	err = app.SaveDiffData(diff, instance.Name)
	if err != nil {
		fmt.Fprintln(w, err.Error())
		return err
	}
	return nil
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
	origin := r.URL.Query().Get("origin")
	if origin != "" {
		if !validateOrigin(origin) {
			return &errors.HTTP{
				Code:    http.StatusBadRequest,
				Message: "Invalid deployment origin",
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	keepAliveWriter := io.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &io.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	canRollback := permission.Check(t, permission.PermAppDeployRollback,
		append(permission.Contexts(permission.CtxTeam, instance.Teams),
			permission.Context(permission.CtxApp, instance.Name),
			permission.Context(permission.CtxPool, instance.Pool),
		)...,
	)
	if !canRollback {
		return &errors.HTTP{Code: http.StatusForbidden, Message: permission.ErrUnauthorized.Error()}
	}
	err = app.Rollback(app.DeployOptions{
		App:          instance,
		OutputStream: writer,
		Image:        image,
		User:         t.GetUserName(),
		Origin:       origin,
	})
	if err != nil {
		writer.Encode(io.SimpleJsonMessage{Error: err.Error()})
	}
	return nil
}

func deploysList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	contexts := permission.ContextsForPermission(t, permission.PermAppReadDeploy)
	if len(contexts) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	filter := appFilterByContext(contexts, nil)
	filter.Name = r.URL.Query().Get("app")
	skip := r.URL.Query().Get("skip")
	limit := r.URL.Query().Get("limit")
	skipInt, _ := strconv.Atoi(skip)
	limitInt, _ := strconv.Atoi(limit)
	deploys, err := app.ListDeploys(filter, skipInt, limitInt)
	if err != nil {
		return err
	}
	if len(deploys) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	return json.NewEncoder(w).Encode(deploys)
}

func deployInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	depId := r.URL.Query().Get(":deploy")
	deploy, err := app.GetDeploy(depId)
	if err != nil {
		return err
	}
	dbApp, err := app.GetByName(deploy.App)
	if err != nil {
		return err
	}
	canGet := permission.Check(t, permission.PermAppReadDeploy,
		append(permission.Contexts(permission.CtxTeam, dbApp.Teams),
			permission.Context(permission.CtxApp, dbApp.Name),
			permission.Context(permission.CtxPool, dbApp.Pool),
		)...,
	)
	if !canGet {
		return &errors.HTTP{Code: http.StatusNotFound, Message: "Deploy not found."}
	}
	var data interface{}
	data = deploy
	return json.NewEncoder(w).Encode(data)
}
