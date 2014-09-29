// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tsuru/tsuru/db"
	"gopkg.in/mgo.v2"
)

type Plan struct {
	Name     string `bson:"_id"`
	Memory   int64
	Swap     int64
	CpuShare int
	Default  bool
}

type PlanValidationError struct{ field string }

func (p PlanValidationError) Error() string {
	return fmt.Sprintf("invalid value for %s", p.field)
}

var (
	ErrPlanNotFound      = errors.New("plan not found")
	ErrPlanAlreadyExists = errors.New("plan already exists")
)

func (plan *Plan) Save() error {
	if plan.Name == "" {
		return PlanValidationError{"name"}
	}
	if plan.Memory == 0 {
		return PlanValidationError{"memory"}
	}
	if plan.Swap == 0 {
		return PlanValidationError{"swap"}
	}
	if plan.CpuShare == 0 {
		return PlanValidationError{"cpushare"}
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Plans().Insert(plan)
	if err != nil && strings.Contains(err.Error(), "duplicate key") {
		return ErrPlanAlreadyExists
	}
	return err
}

func PlansList() ([]Plan, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var plans []Plan
	err = conn.Plans().Find(nil).All(&plans)
	return plans, err
}

func PlanRemove(planName string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Plans().RemoveId(planName)
	if err == mgo.ErrNotFound {
		return ErrPlanNotFound
	}
	return err
}
