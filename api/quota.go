// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/quota"
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
	email := r.URL.Query().Get(":email")
	allowed := permission.Check(t, permission.PermUserReadQuota, permission.Context(permTypes.CtxUser, email))
	if !allowed {
		return permission.ErrUnauthorized
	}
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
	ctx := r.Context()
	email := r.URL.Query().Get(":email")
	allowed := permission.Check(t, permission.PermUserUpdateQuota)
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
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermUserReadEvents, permission.Context(permTypes.CtxUser, email)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	limit, err := strconv.Atoi(InputValue(r, "limit"))
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "Invalid limit",
		}
	}
	err = servicemanager.UserQuota.SetLimit(ctx, user, limit)
	if err == quota.ErrLimitLowerThanAllocated {
		return &errors.HTTP{
			Code:    http.StatusForbidden,
			Message: err.Error(),
		}
	}
	return err
}

// title: application quota
// path: /apps/{app}/quota
// method: GET
// produce: application/json
// responses:
//   200: OK
//   401: Unauthorized
//   404: Application not found
func getAppQuota(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	a, err := getAppFromContext(r.URL.Query().Get(":app"), r)
	if err != nil {
		return err
	}
	canRead := permission.Check(t, permission.PermAppRead, contextsForApp(&a)...)
	if !canRead {
		return permission.ErrUnauthorized
	}
	w.Header().Set("Content-Type", "application/json")
	quota, err := a.GetQuota()
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(quota)
}

// title: update application quota
// path: /apps/{app}/quota
// method: PUT
// consume: application/x-www-form-urlencoded
// responses:
//   200: Quota updated
//   400: Invalid data
//   401: Unauthorized
//   403: Limit lower than allocated
//   404: Application not found
func changeAppQuota(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	appName := r.URL.Query().Get(":app")
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
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	limit, err := strconv.Atoi(InputValue(r, "limit"))
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "Invalid limit",
		}
	}
	err = a.SetQuotaLimit(limit)
	if err == quota.ErrLimitLowerThanAllocated {
		return &errors.HTTP{
			Code:    http.StatusForbidden,
			Message: err.Error(),
		}
	}
	return err
}
