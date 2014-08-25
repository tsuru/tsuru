// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/io"
)

func deploy(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	version := r.PostFormValue("version")
	archiveURL := r.PostFormValue("archive-url")
	if version == "" && archiveURL == "" {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "you must specify either the version or the archive-url",
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
	instance, err := app.GetByName(appName)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf("App %s not found.", appName)}
	}
	writer := io.NewKeepAliveWriter(w, 30*time.Second, "please wait...")
	err = app.Deploy(app.DeployOptions{
		App:          instance,
		Version:      version,
		Commit:       commit,
		ArchiveURL:   archiveURL,
		OutputStream: writer,
	})
	if err == nil {
		fmt.Fprintln(w, "\nOK")
	}
	return err

}

func deploysList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	service := r.URL.Query().Get("service")
	if service != "" {
		s, err := getServiceOrError(service, u)
		if err != nil {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		deploys, err := app.ListDeploys(&s)
		if err != nil {
			return err
		}
		return json.NewEncoder(w).Encode(deploys)
	}
	deploys, err := app.ListDeploys(nil)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(deploys)
}

func deployInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	depId := r.URL.Query().Get(":deploy")
	deploy, err := app.GetDeploy(depId)
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
		"Duration":  deploy.Duration.Seconds(),
		"Commit":    deploy.Commit,
		"Error":     deploy.Error,
		"Diff":      diff,
	}
	return json.NewEncoder(w).Encode(data)
}
