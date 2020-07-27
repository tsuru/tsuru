// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

type Plan struct {
	Name   string `json:"name"`
	Memory int64  `json:"memory"`
	Swap   int64  `json:"swap"`
	// CpuShare is DEPRECATED, use CPUMilli instead
	CpuShare int          `json:"cpushare"`
	CPUMilli int          `json:"cpumilli"`
	Default  bool         `json:"default,omitempty"`
	Override PlanOverride `json:"override,omitempty"`
}

type PlanOverride struct {
	Memory   *int64 `json:"memory"`
	CPUMilli *int   `json:"cpumilli"`
}

func (p *Plan) MergeOverride(po PlanOverride) {
	if po.Memory != nil {
		if *po.Memory == 0 {
			p.Override.Memory = nil
		} else {
			p.Override.Memory = po.Memory
		}
	}
	if po.CPUMilli != nil {
		if *po.CPUMilli == 0 {
			p.Override.CPUMilli = nil
		} else {
			p.Override.CPUMilli = po.CPUMilli
		}
	}
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
