package api

import (
	"encoding/json"
	"fmt"
	"net/http"

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

	canRead := permission.Check(t, permission.PermAppRead,
		contextsForApp(&a)...,
	)
	if !canRead {
		return permission.ErrUnauthorized
	}

	info, err := a.AutoScaleInfo(ctx)
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
	allowed := permission.Check(t, permission.PermAppUpdateUnitAutoscaleAdd,
		contextsForApp(&a)...,
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
	quota, err := a.GetQuota(ctx)
	if err != nil {
		return err
	}
	err = provision.ValidateAutoScaleSpec(&spec, quota.Limit, &a)
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
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	return a.AutoScale(ctx, spec)
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
	allowed := permission.Check(t, permission.PermAppUpdateUnitAutoscaleRemove,
		contextsForApp(&a)...,
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
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	return a.RemoveAutoScale(ctx, process)
}
