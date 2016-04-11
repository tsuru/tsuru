// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/router"
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
func addPlan(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	cpuShare, _ := strconv.Atoi(r.FormValue("cpushare"))
	isDefault, _ := strconv.ParseBool(r.FormValue("default"))
	memory := getSize(r.FormValue("memory"))
	swap := getSize(r.FormValue("swap"))
	plan := app.Plan{
		Name:     r.FormValue("name"),
		Memory:   memory,
		Swap:     swap,
		CpuShare: cpuShare,
		Default:  isDefault,
		Router:   r.FormValue("router"),
	}
	allowed := permission.Check(t, permission.PermPlanCreate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	err := plan.Save()
	if _, ok := err.(app.PlanValidationError); ok {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	if err == app.ErrPlanAlreadyExists {
		return &errors.HTTP{
			Code:    http.StatusConflict,
			Message: err.Error(),
		}
	}
	if err == app.ErrLimitOfMemory || err == app.ErrLimitOfCpuShare {
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
	plans, err := app.PlansList()
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
func removePlan(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowed := permission.Check(t, permission.PermPlanDelete)
	if !allowed {
		return permission.ErrUnauthorized
	}
	planName := r.URL.Query().Get(":planname")
	err := app.PlanRemove(planName)
	if err == app.ErrPlanNotFound {
		return &errors.HTTP{
			Code:    http.StatusNotFound,
			Message: err.Error(),
		}
	}
	return err
}

// title: router list
// path: /plans/routers
// method: GET
// produce: application/json
// responses:
//   200: OK
//   204: No content
func listRouters(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowed := permission.Check(t, permission.PermPlanCreate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	routers, err := router.List()
	if err != nil {
		return err
	}
	if len(routers) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(routers)
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
