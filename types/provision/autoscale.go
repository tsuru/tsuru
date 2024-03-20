// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package provision provides interfaces that need to be satisfied in order to
// implement a new provisioner on tsuru.

package provision

type AutoScaleSpec struct {
	Process    string              `json:"process"`
	MinUnits   uint                `json:"minUnits"`
	MaxUnits   uint                `json:"maxUnits"`
	AverageCPU string              `json:"averageCPU,omitempty"`
	Schedules  []AutoScaleSchedule `json:"schedules,omitempty"`
	Version    int                 `json:"version"`
}

type AutoScaleSchedule struct {
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
