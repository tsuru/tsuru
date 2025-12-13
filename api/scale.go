// Copyright 2025 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"

	provTypes "github.com/tsuru/tsuru/types/provision"
)

// title: units autoscale info
// path: /apps/{app}/units/autoscale
// method: GET
// produce: application/json
// responses:
//
//	200: Ok
//	401: Unauthorized
//	404: App not found
func autoScaleUnitsInfo(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}

	canRead := permission.Check(ctx, t, permission.PermAppRead,
		contextsForApp(a)...,
	)
	if !canRead {
		return permission.ErrUnauthorized
	}

	info, err := app.AutoScaleInfo(ctx, a)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(&info)
}

// title: add unit auto scale
// path: /apps/{app}/units/autoscale
// method: POST
// consume: application/json
// responses:
//
//	200: Ok
//	400: Invalid data
//	401: Unauthorized
//	404: App not found
func addAutoScaleUnits(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(ctx, t, permission.PermAppUpdateUnitAutoscaleAdd,
		contextsForApp(a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	var spec provTypes.AutoScaleSpec
	err = ParseInput(r, &spec)
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("unable to parse autoscale spec: %v", err),
		}
	}
	quota, err := app.GetQuota(ctx, a)
	if err != nil {
		return err
	}
	err = provision.ValidateAutoScaleSpec(&spec, quota.Limit, a)
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("unable to validate autoscale spec: %v", err),
		}
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateUnitAutoscaleAdd,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	return app.AutoScale(ctx, a, spec)
}

// title: swap unit auto scale
// path: /apps/{app}/units/autoscale/swap
// method: POST
// consume: application/json
// responses:
//
//	200: Ok
//	400: Invalid data
//	401: Unauthorized
//	404: App not found
func swapAutoScaleUnits(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	appName := r.URL.Query().Get(":app")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(ctx, t, permission.PermAppUpdateUnitAutoscaleSwap,
		contextsForApp(a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}

	versionStr := InputValue(r, "version")
	if versionStr == "" {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "version is required to swap autoscale",
		}
	}

	evt, err := event.New(ctx, &event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateUnitAutoscaleSwap,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(a)...),
	})
	if err != nil {
		return err
	}

	defer func() { evt.Done(ctx, err) }()
	return app.SwapAutoScale(ctx, a, versionStr)
}

// title: remove unit auto scale
// path: /apps/{app}/units/autoscale
// method: POST
// consume: application/json
// responses:
//
//	200: Ok
//	401: Unauthorized
//	404: App not found
func removeAutoScaleUnits(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	appName := r.URL.Query().Get(":app")
	process := InputValue(r, "process")
	a, err := getAppFromContext(appName, r)
	if err != nil {
		return err
	}
	allowed := permission.Check(ctx, t, permission.PermAppUpdateUnitAutoscaleRemove,
		contextsForApp(a)...,
	)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateUnitAutoscaleRemove,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	return app.RemoveAutoScale(ctx, a, process)
}
