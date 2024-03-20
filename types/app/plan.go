// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import "context"

type Plan struct {
	Name     string        `json:"name"`
	Memory   int64         `json:"memory"`
	CPUMilli int           `json:"cpumilli"`
	CPUBurst *CPUBurst     `json:"cpuBurst,omitempty"`
	Default  bool          `json:"default,omitempty"`
	Override *PlanOverride `json:"override,omitempty"`
}

type PlanOverride struct {
	Memory   *int64   `json:"memory"`
	CPUMilli *int     `json:"cpumilli"`
	CPUBurst *float64 `json:"cpuBurst"`
}

type CPUBurst struct {
	Default    float64 `json:"default"`
	MaxAllowed float64 `json:"maxAllowed"`
}

func (p *Plan) MergeOverride(po PlanOverride) {

	newOverride := p.Override
	if newOverride == nil {
		newOverride = &PlanOverride{}
	}
	if po.Memory != nil {
		if *po.Memory == 0 {
			newOverride.Memory = nil
		} else {
			newOverride.Memory = po.Memory
		}
	}
	if po.CPUMilli != nil {
		if *po.CPUMilli == 0 {
			newOverride.CPUMilli = nil
		} else {
			newOverride.CPUMilli = po.CPUMilli
		}
	}

	if po.CPUBurst != nil {
		if *po.CPUBurst == 0 {
			newOverride.CPUBurst = nil
		} else {
			newOverride.CPUBurst = po.CPUBurst
		}
	}

	if (*newOverride == PlanOverride{}) {
		p.Override = nil
	} else {
		p.Override = newOverride
	}
}

func (p Plan) GetMemory() int64 {
	if p.Override != nil && p.Override.Memory != nil {
		return *p.Override.Memory
	}
	return p.Memory
}

func (p Plan) GetMilliCPU() int {
	if p.Override != nil && p.Override.CPUMilli != nil {
		return *p.Override.CPUMilli
	}
	return p.CPUMilli
}

func (p Plan) GetCPUBurst() float64 {
	if p.Override != nil && p.Override.CPUBurst != nil {
		return *p.Override.CPUBurst
	}
	if p.CPUBurst != nil {
		return p.CPUBurst.Default
	}

	return 0
}

type PlanService interface {
	Create(ctx context.Context, plan Plan) error
	List(context.Context) ([]Plan, error)
	FindByName(ctx context.Context, name string) (*Plan, error)
	DefaultPlan(context.Context) (*Plan, error)
	Remove(ctx context.Context, planName string) error
}

type PlanStorage interface {
	Insert(context.Context, Plan) error
	FindAll(context.Context) ([]Plan, error)
	FindDefault(context.Context) (*Plan, error)
	FindByName(context.Context, string) (*Plan, error)
	Delete(context.Context, Plan) error
}
