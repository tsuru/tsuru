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
	appTypes "github.com/tsuru/tsuru/types/app"
)

// title: plan create
// path: /plans
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   201: Plan created
//   400: Invalid data
//   401: Unauthorized
//   409: Plan already exists
func addPlan(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	cpuShare, _ := strconv.Atoi(InputValue(r, "cpushare"))
	isDefault, _ := strconv.ParseBool(InputValue(r, "default"))
	memory := getSize(InputValue(r, "memory"))
	swap := getSize(InputValue(r, "swap"))
	plan := appTypes.Plan{
		Name:     InputValue(r, "name"),
		Memory:   memory,
		Swap:     swap,
		CpuShare: cpuShare,
		Default:  isDefault,
	}
	allowed := permission.Check(t, permission.PermPlanCreate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypePlan, Value: plan.Name},
		Kind:       permission.PermPlanCreate,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermPlanReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = servicemanager.Plan.Create(ctx, plan)
	if err == appTypes.ErrPlanAlreadyExists {
		return &errors.HTTP{
			Code:    http.StatusConflict,
			Message: err.Error(),
		}
	}
	if err == appTypes.ErrLimitOfMemory || err == appTypes.ErrLimitOfCpuShare {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	if err == nil {
		w.WriteHeader(http.StatusCreated)
	}
	return err
}

// title: plan list
// path: /plans
// method: GET
// produce: application/json
// responses:
//   200: OK
//   204: No content
func listPlans(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	plans, err := servicemanager.Plan.List(r.Context())
	if err != nil {
		return err
	}
	if len(plans) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(plans)
}

// title: remove plan
// path: /plans/{name}
// method: DELETE
// responses:
//   200: Plan removed
//   401: Unauthorized
//   404: Plan not found
func removePlan(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	allowed := permission.Check(t, permission.PermPlanDelete)
	if !allowed {
		return permission.ErrUnauthorized
	}
	planName := r.URL.Query().Get(":planname")
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypePlan, Value: planName},
		Kind:       permission.PermPlanDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermPlanReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = servicemanager.Plan.Remove(ctx, planName)
	if err == appTypes.ErrPlanNotFound {
		return &errors.HTTP{
			Code:    http.StatusNotFound,
			Message: err.Error(),
		}
	}
	return err
}

func getSize(formValue string) int64 {
	const OneKbInBytes = 1024
	value, err := strconv.ParseInt(formValue, 10, 64)
	if err != nil {
		unit := formValue[len(formValue)-1:]
		size, _ := strconv.ParseInt(formValue[0:len(formValue)-1], 10, 64)
		switch unit {
		case "K":
			return size * OneKbInBytes
		case "M":
			return size * OneKbInBytes * OneKbInBytes
		case "G":
			return size * OneKbInBytes * OneKbInBytes * OneKbInBytes
		default:
			return 0
		}
	}
	return value
}
