// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"net/http"
	"strconv"
)

func getUserQuota(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	email := r.URL.Query().Get(":email")
	user, err := auth.GetUserByEmail(email)
	if err == auth.ErrUserNotFound {
		return &errors.HTTP{
			Code:    http.StatusNotFound,
			Message: err.Error(),
		}
	} else if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(user.Quota)
}

func changeUserQuota(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	limit, err := strconv.Atoi(r.FormValue("limit"))
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "Invalid limit",
		}
	}
	email := r.URL.Query().Get(":email")
	user, err := auth.GetUserByEmail(email)
	if err == auth.ErrUserNotFound {
		return &errors.HTTP{
			Code:    http.StatusNotFound,
			Message: err.Error(),
		}
	} else if err != nil {
		return err
	}
	return auth.ChangeQuota(user, limit)
}

func getAppQuota(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	a, err := app.GetByName(r.URL.Query().Get(":appname"))
	if err == app.ErrAppNotFound {
		return &errors.HTTP{
			Code:    http.StatusNotFound,
			Message: err.Error(),
		}
	} else if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(a.Quota)
}

func changeAppQuota(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	limit, err := strconv.Atoi(r.FormValue("limit"))
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "Invalid limit",
		}
	}
	appName := r.URL.Query().Get(":appname")
	a, err := app.GetByName(appName)
	if err != nil {
		return err
	}
	return app.ChangeQuota(a, limit)
}
