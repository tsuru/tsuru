// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

type Plan struct {
	Name     string `json:"name"`
	Memory   int64  `json:"memory"`
	Swap     int64  `json:"swap"`
	CpuShare int    `json:"cpushare"`
	Default  bool   `json:"default,omitempty"`
}

type PlanService interface {
	Create(plan Plan) error
	List() ([]Plan, error)
	FindByName(name string) (*Plan, error)
	DefaultPlan() (*Plan, error)
	Remove(planName string) error
}

type PlanStorage interface {
	Insert(Plan) error
	FindAll() ([]Plan, error)
	FindDefault() (*Plan, error)
	FindByName(string) (*Plan, error)
	Delete(Plan) error
}
