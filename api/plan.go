// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
)

type Plan struct {
	Name     string `bson:"_id"`
	Memory   int64
	Swap     int64
	CpuShare int
}

func addPlan(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	var plan Plan
	err := json.NewDecoder(r.Body).Decode(&plan)
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "unable to parse request body",
		}
	}
	if plan.Name == "" {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "name can't be empty",
		}
	}
	if plan.Memory == 0 {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "memory can't be 0",
		}
	}
	if plan.Swap == 0 {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "swap can't be 0",
		}
	}
	if plan.CpuShare == 0 {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "cpushare can't be 0",
		}
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Plans().Insert(plan)
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusCreated)
	return nil
}

func listPlans(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var plans []Plan
	err = conn.Plans().Find(nil).All(&plans)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(plans)
}

func removePlan(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return nil
}
