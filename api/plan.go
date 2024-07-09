// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	eventTypes "github.com/tsuru/tsuru/types/event"
	"k8s.io/apimachinery/pkg/api/resource"
)

// title: plan create
// path: /plans
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//
//	201: Plan created
//	400: Invalid data
//	401: Unauthorized
//	409: Plan already exists
func addPlan(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	isJSON := r.Header.Get("Content-Type") == "application/json"

	plan := appTypes.Plan{}

	if isJSON {
		err = ParseInput(r, &plan)
		if err != nil {
			return err
		}
	} else {
		cpuMilli, _ := strconv.Atoi(InputValue(r, "cpumilli"))

		isDefault, _ := strconv.ParseBool(InputValue(r, "default"))
		memory := getSize(InputValue(r, "memory"))

		plan = appTypes.Plan{
			Name:     InputValue(r, "name"),
			Memory:   memory,
			CPUMilli: cpuMilli,
			Default:  isDefault,
		}
	}

	allowed := permission.Check(ctx, t, permission.PermPlanCreate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     eventTypes.Target{Type: eventTypes.TargetTypePlan, Value: plan.Name},
		Kind:       permission.PermPlanCreate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermPlanReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	err = servicemanager.Plan.Create(ctx, plan)
	if err == appTypes.ErrPlanAlreadyExists {
		return &errors.HTTP{
			Code:    http.StatusConflict,
			Message: err.Error(),
		}
	}
	if err == appTypes.ErrLimitOfMemory {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	if err == nil {
		w.WriteHeader(http.StatusCreated)
		return json.NewEncoder(w).Encode(plan)
	}
	return err
}

// title: plan list
// path: /plans
// method: GET
// produce: application/json
// responses:
//
//	200: OK
//	204: No content
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
//
//	200: Plan removed
//	401: Unauthorized
//	404: Plan not found
func removePlan(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	allowed := permission.Check(ctx, t, permission.PermPlanDelete)
	if !allowed {
		return permission.ErrUnauthorized
	}
	planName := r.URL.Query().Get(":planname")
	evt, err := event.New(ctx, &event.Opts{
		Target:     eventTypes.Target{Type: eventTypes.TargetTypePlan, Value: planName},
		Kind:       permission.PermPlanDelete,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermPlanReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
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
	value, err := strconv.ParseInt(formValue, 10, 64)
	if err == nil {
		return value
	}
	if strings.HasSuffix(formValue, "K") ||
		strings.HasSuffix(formValue, "M") ||
		strings.HasSuffix(formValue, "G") {
		formValue = formValue + "i"
	}

	qtdy, _ := resource.ParseQuantity(formValue)
	v, _ := qtdy.AsInt64()
	return v
}
