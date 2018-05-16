// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
)

// title: user quota
// path: /users/{email}/quota
// method: GET
// produce: application/json
// responses:
//   200: OK
//   401: Unauthorized
//   404: User not found
func getUserQuota(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowed := permission.Check(t, permission.PermUserUpdateQuota)
	if !allowed {
		return permission.ErrUnauthorized
	}
	email := r.URL.Query().Get(":email")
	user, err := auth.GetUserByEmail(email)
	if err == authTypes.ErrUserNotFound {
		return &errors.HTTP{
			Code:    http.StatusNotFound,
			Message: err.Error(),
		}
	}
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(user.Quota)
}

// title: update user quota
// path: /users/{email}/quota
// method: PUT
// consume: application/x-www-form-urlencoded
// responses:
//   200: Quota updated
//   400: Invalid data
//   401: Unauthorized
//   403: Limit lower than allocated value
//   404: User not found
func changeUserQuota(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	email := r.URL.Query().Get(":email")
	allowed := permission.Check(t, permission.PermUserUpdateQuota, permission.Context(permission.CtxUser, email))
	if !allowed {
		return permission.ErrUnauthorized
	}
	user, err := auth.GetUserByEmail(email)
	if err == authTypes.ErrUserNotFound {
		return &errors.HTTP{
			Code:    http.StatusNotFound,
			Message: err.Error(),
		}
	} else if err != nil {
		return err
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeUser, Value: email},
		Kind:       permission.PermUserUpdateQuota,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermUserReadEvents, permission.Context(permission.CtxUser, email)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	limit, err := strconv.Atoi(r.FormValue("limit"))
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "Invalid limit",
		}
	}
	err = servicemanager.UserQuota.ChangeLimit(user.Email, limit)
	if err == authTypes.ErrLimitLowerThanAllocated {
		return &errors.HTTP{
			Code:    http.StatusForbidden,
			Message: err.Error(),
		}
	}
	return err
}

// title: application quota
// path: /apps/{appname}/quota
// method: GET
// produce: application/json
// responses:
//   200: OK
//   401: Unauthorized
//   404: Application not found
func getAppQuota(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	a, err := getAppFromContext(r.URL.Query().Get(":appname"), r)
	if err != nil {
		return err
	}
	canRead := permission.Check(t, permission.PermAppRead, contextsForApp(&a)...)
	if !canRead {
		return permission.ErrUnauthorized
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(a.Quota)
}

// title: update application quota
// path: /apps/{appname}/quota
// method: PUT
// consume: application/x-www-form-urlencoded
// responses:
//   200: Quota updated
//   400: Invalid data
//   401: Unauthorized
//   403: Limit lower than allocated
//   404: Application not found
func changeAppQuota(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	appName := r.URL.Query().Get(":appname")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(t, permission.PermAppAdminQuota, contextsForApp(&a)...)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeApp, Value: appName},
		Kind:       permission.PermAppAdminQuota,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	limit, err := strconv.Atoi(r.FormValue("limit"))
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "Invalid limit",
		}
	}
	err = a.SetQuotaLimit(limit)
	if err == appTypes.ErrLimitLowerThanAllocated {
		return &errors.HTTP{
			Code:    http.StatusForbidden,
			Message: err.Error(),
		}
	}
	return err
}
