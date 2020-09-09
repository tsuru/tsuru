package api

import (
	"fmt"
	"net/http"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
)

// title: add unit auto scale
// path: /apps/{app}/units/autoscale
// method: POST
// consume: application/json
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: App not found
func addAutoScaleUnits(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
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
	var spec provision.AutoScaleSpec
	err = ParseInput(r, &spec)
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("unable to parse autoscale spec: %v", err),
		}
	}
	err = spec.Validate()
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("unable to validate autoscale spec: %v", err),
		}
	}
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateUnitAutoscaleAdd,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	return a.AutoScale(spec)
}

// title: remove unit auto scale
// path: /apps/{app}/units/autoscale
// method: POST
// consume: application/json
// responses:
//   200: Ok
//   401: Unauthorized
//   404: App not found
func removeAutoScaleUnits(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
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
	evt, err := event.New(&event.Opts{
		Target:     appTarget(appName),
		Kind:       permission.PermAppUpdateUnitAutoscaleRemove,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	return a.RemoveAutoScale(process)
}
