// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package provision provides interfaces that need to be satisfied in order to
// implement a new provisioner on tsuru.

package provision

type AutoScaleSpec struct {
	Process    string                `json:"process"`
	MinUnits   uint                  `json:"minUnits"`
	MaxUnits   uint                  `json:"maxUnits"`
	AverageCPU string                `json:"averageCPU,omitempty"`
	Schedules  []AutoScaleSchedule   `json:"schedules,omitempty"`
	Prometheus []AutoScalePrometheus `json:"prometheus,omitempty"`
	Version    int                   `json:"version"`
	Behavior   BehaviorAutoScaleSpec `json:"behavior,omitempty"`
}

type BehaviorAutoScaleSpec struct {
	ScaleDown *ScaleDownPoliciy `json:"scaleDown"`
}

type ScaleDownPoliciy struct {
	StabilizationWindow   *int32 `json:"stabilizationWindow"`
	PercentagePolicyValue *int32 `json:"percentagePolicyValue"`
	UnitsPolicyValue      *int32 `json:"unitsPolicyValue"`
}

type AutoScalePrometheus struct {
	Name                string  `json:"name"`
	Query               string  `json:"query"`
	Threshold           float64 `json:"threshold"`
	ActivationThreshold float64 `json:"activationThreshold,omitempty"`
	PrometheusAddress   string  `json:"prometheusAddress,omitempty"`
}

type AutoScaleSchedule struct {
	Name        string `json:"name,omitempty"`
	MinReplicas int    `json:"minReplicas"`
	Start       string `json:"start"`
	End         string `json:"end"`
	Timezone    string `json:"timezone,omitempty"`
}

type RecommendedResources struct {
	Process         string                        `json:"process"`
	Recommendations []RecommendedProcessResources `json:"recommendations"`
}

type RecommendedProcessResources struct {
	Type   string `json:"type"`
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}
