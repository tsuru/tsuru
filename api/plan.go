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

func addPlan(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	cpuShare, _ := strconv.Atoi(r.FormValue("cpushare"))
	isDefault, _ := strconv.ParseBool(r.FormValue("default"))
	memory, _ := strconv.ParseInt(r.FormValue("memory"), 10, 64)
	swap, _ := strconv.ParseInt(r.FormValue("swap"), 10, 64)
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

func listPlans(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	plans, err := app.PlansList()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(plans)
}

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

func listRouters(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowed := permission.Check(t, permission.PermPlanCreate)
	if !allowed {
		return permission.ErrUnauthorized
	}
	routers, err := router.List()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(routers)
}
