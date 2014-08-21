// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
)

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
