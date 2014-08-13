// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"net/http"
	"strconv"
)

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
