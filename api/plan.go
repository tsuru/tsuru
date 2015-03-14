// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/router"
)

func addPlan(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	var plan app.Plan
	err := json.NewDecoder(r.Body).Decode(&plan)
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "unable to parse request body",
		}
	}
	err = plan.Save()
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
	routers, err := router.GetList()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(routers)
}
