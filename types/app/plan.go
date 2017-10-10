// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"fmt"
)

type Plan struct {
	Name     string `json:"name"`
	Memory   int64  `json:"memory"`
	Swap     int64  `json:"swap"`
	CpuShare int    `json:"cpushare"`
	Default  bool   `json:"default,omitempty"`
}

type PlanService interface {
	Insert(Plan) error
	FindAll() ([]Plan, error)
	FindDefault() (*Plan, error)
	FindByName(string) (*Plan, error)
	Delete(Plan) error
}

type PlanValidationError struct {
	Field string
}

func (p PlanValidationError) Error() string {
	return fmt.Sprintf("invalid value for %s", p.Field)
}

var (
	ErrPlanNotFound         = errors.New("plan not found")
	ErrPlanAlreadyExists    = errors.New("plan already exists")
	ErrPlanDefaultAmbiguous = errors.New("more than one default plan found")
	ErrLimitOfCpuShare      = errors.New("The minimum allowed cpu-shares is 2")
	ErrLimitOfMemory        = errors.New("The minimum allowed memory is 4MB")
)
