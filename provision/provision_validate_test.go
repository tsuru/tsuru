// Copyright 2025 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
	"k8s.io/utils/ptr"
)

var testApp = appTypes.App{
	Name: "myapp",
	Plan: appTypes.Plan{
		CPUMilli: 10000,
	},
}

func (ProvisionSuite) TestValidateWhenErrors(c *check.C) {
	var tests = []struct {
		input    provTypes.AutoScaleSpec
		expected string
	}{
		{
			provTypes.AutoScaleSpec{
				MinUnits: 0,
				MaxUnits: 10,
			},
			"minimum units must be greater than 0",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 11,
				MaxUnits: 10,
			},
			"maximum units must be greater than minimum units",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 10,
				MaxUnits: 10,
			},
			"maximum units must be greater than minimum units",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 10,
				MaxUnits: 20,
			},
			"maximum units cannot be greater than quota limit",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 2,
			},
			"you have to configure at least one trigger between cpu, schedule and prometheus",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits:   1,
				MaxUnits:   2,
				AverageCPU: "99",
			},
			"autoscale cpu value cannot be greater than 95%",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits:   1,
				MaxUnits:   2,
				AverageCPU: "15",
			},
			"autoscale cpu value cannot be less than 20%",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 2,
				Prometheus: []provTypes.AutoScalePrometheus{{
					Name: "Invalid-Name",
				}},
			},
			"\"Invalid-Name\" is an invalid name, it must contain only lower case letters, numbers or dashes and starts with a letter",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 2,
				Prometheus: []provTypes.AutoScalePrometheus{{
					Name:      "valid-name",
					Threshold: 10,
				}, {
					Name:      "another$invalid",
					Threshold: 10,
				}},
			},
			"\"another$invalid\" is an invalid name, it must contain only lower case letters, numbers or dashes and starts with a letter",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 2,
				Prometheus: []provTypes.AutoScalePrometheus{{
					Name:      "valid-name",
					Threshold: 10,
				}, {
					Name: "valid-name2",
				}},
			},
			"prometheus threshold of name \"valid-name2\" must be greater than 0",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 10,
				Prometheus: []provTypes.AutoScalePrometheus{{
					Name:      "valid-name",
					Threshold: -1,
				}},
			},
			"prometheus threshold of name \"valid-name\" must be greater than 0",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 10,
				Prometheus: []provTypes.AutoScalePrometheus{{
					Name:                "valid-name",
					Threshold:           1,
					ActivationThreshold: -1,
				}},
			},
			"prometheus activationThreshold of name \"valid-name\" must be greater than 0",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 10,
				Schedules: []provTypes.AutoScaleSchedule{{
					Name:  "valid-name",
					Start: "0 0 0 0 0",
					End:   "0 0 0 0 0",
				}},
			},
			"invalid start time for schedule \"valid-name\": beginning of range (0) below minimum (1): 0",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 10,
				Schedules: []provTypes.AutoScaleSchedule{{
					Name:  "valid-name",
					Start: "* * * * 1",
					End:   "* * * * 7",
				}},
			},
			"invalid end time for schedule \"valid-name\": end of range (7) above maximum (6): 7",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 10,
				Schedules: []provTypes.AutoScaleSchedule{{
					Name:  "valid-name",
					Start: "* * * 12 *",
					End:   "* * * 13 *",
				}},
			},
			"invalid end time for schedule \"valid-name\": end of range (13) above maximum (12): 13",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 10,
				Schedules: []provTypes.AutoScaleSchedule{{
					Name:  "valid-name",
					Start: "* * * 0 *",
					End:   "* * * 1 *",
				}},
			},
			"invalid start time for schedule \"valid-name\": beginning of range (0) below minimum (1): 0",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 10,
				Schedules: []provTypes.AutoScaleSchedule{{
					Name:  "valid-name",
					Start: "* * * * *",
					End:   "* * 32 * *",
				}},
			},
			"invalid end time for schedule \"valid-name\": end of range (32) above maximum (31): 32",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 10,
				Schedules: []provTypes.AutoScaleSchedule{{
					Name:  "valid-name",
					Start: "* 1 * * *",
					End:   "* 24 * * *",
				}},
			},
			"invalid end time for schedule \"valid-name\": end of range (24) above maximum (23): 24",
		},
	}

	for _, test := range tests {
		err := ValidateAutoScaleSpec(&test.input, 10, &testApp)
		c.Assert(err, check.NotNil)
		c.Assert(err.Error(), check.Equals, test.expected)
	}
}

func (ProvisionSuite) TestValidateWhenNoErrors(c *check.C) {
	var tests = []struct {
		input provTypes.AutoScaleSpec
	}{
		{
			provTypes.AutoScaleSpec{
				MinUnits:   1,
				MaxUnits:   10,
				AverageCPU: "40",
			},
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 10,
				Schedules: []provTypes.AutoScaleSchedule{{
					Name:  "valid-name",
					Start: "5 * * * *",
					End:   "10 * * * *",
				}},
			},
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 10,
				Schedules: []provTypes.AutoScaleSchedule{{
					Name:  "valid-name",
					Start: "* * * * 0",
					End:   "* * * * 6",
				}},
			},
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 10,
				Schedules: []provTypes.AutoScaleSchedule{{
					Name:  "valid-name",
					Start: "* * * * 0-6",
					End:   "* * * * 0-6",
				}},
			},
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 10,
				Schedules: []provTypes.AutoScaleSchedule{{
					Name:  "valid-name",
					Start: "0 * * * *",
					End:   "59 * * * *",
				}},
			},
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 10,
				Schedules: []provTypes.AutoScaleSchedule{{
					Name:  "valid-name",
					Start: "* * * 1 *",
					End:   "* * * 12 *",
				}},
			},
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 10,
				Schedules: []provTypes.AutoScaleSchedule{{
					Name:  "valid-name",
					Start: "* * 1 * *",
					End:   "* * 31 * *",
				}},
			},
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 10,
				Schedules: []provTypes.AutoScaleSchedule{{
					Name:  "valid-name",
					Start: "* 0 * * *",
					End:   "* 23 * * *",
				}},
			},
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 10,
				Prometheus: []provTypes.AutoScalePrometheus{{
					Name:      "valid-name",
					Threshold: 10,
				}},
			},
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 10,
				Prometheus: []provTypes.AutoScalePrometheus{{
					Name:                "valid-name",
					Threshold:           10,
					ActivationThreshold: 0,
				}},
			},
		},
	}

	for _, test := range tests {
		err := ValidateAutoScaleSpec(&test.input, 10, &testApp)
		c.Assert(err, check.IsNil)
	}
}

func (s *S) TestValidateAutoScaleDownSpec_ReturnError(c *check.C) {
	tests := []struct {
		param     *provTypes.AutoScaleSpec
		expectErr string
	}{
		{
			param: &provTypes.AutoScaleSpec{Behavior: provTypes.BehaviorAutoScaleSpec{ScaleDown: &provTypes.ScaleDownPolicy{
				StabilizationWindow: ptr.To(int32(-1)),
			}}},
			expectErr: "not enough stabilization window to scale down",
		},
		{
			param: &provTypes.AutoScaleSpec{Behavior: provTypes.BehaviorAutoScaleSpec{ScaleDown: &provTypes.ScaleDownPolicy{
				PercentagePolicyValue: ptr.To(int32(-1)),
			}}},
			expectErr: "not enough percentage to scale down",
		},
		{
			param: &provTypes.AutoScaleSpec{Behavior: provTypes.BehaviorAutoScaleSpec{ScaleDown: &provTypes.ScaleDownPolicy{
				UnitsPolicyValue: ptr.To(int32(-1)),
			}}},
			expectErr: "not enough units to scale down",
		},
	}
	for _, tt := range tests {
		err := ValidateAutoScaleDownSpec(tt.param)
		c.Assert(err, check.ErrorMatches, tt.expectErr)
	}
}

func (s *S) TestValidateAutoScaleDownSpec_NotReturnError(c *check.C) {
	tests := []struct {
		param     *provTypes.AutoScaleSpec
		expectErr error
	}{
		{
			param:     nil,
			expectErr: nil,
		},
		{
			param:     &provTypes.AutoScaleSpec{},
			expectErr: nil,
		},
		{
			param:     &provTypes.AutoScaleSpec{MaxUnits: 1},
			expectErr: nil,
		},
		{
			param:     &provTypes.AutoScaleSpec{Behavior: provTypes.BehaviorAutoScaleSpec{}},
			expectErr: nil,
		},
		{
			param:     &provTypes.AutoScaleSpec{Behavior: provTypes.BehaviorAutoScaleSpec{ScaleDown: &provTypes.ScaleDownPolicy{}}},
			expectErr: nil,
		},
		{
			param:     &provTypes.AutoScaleSpec{Behavior: provTypes.BehaviorAutoScaleSpec{ScaleDown: &provTypes.ScaleDownPolicy{StabilizationWindow: new(int32)}}},
			expectErr: nil,
		},
		{
			param:     &provTypes.AutoScaleSpec{Behavior: provTypes.BehaviorAutoScaleSpec{ScaleDown: &provTypes.ScaleDownPolicy{PercentagePolicyValue: new(int32)}}},
			expectErr: nil,
		},
		{
			param:     &provTypes.AutoScaleSpec{Behavior: provTypes.BehaviorAutoScaleSpec{ScaleDown: &provTypes.ScaleDownPolicy{UnitsPolicyValue: new(int32)}}},
			expectErr: nil,
		},
		{
			param:     &provTypes.AutoScaleSpec{Behavior: provTypes.BehaviorAutoScaleSpec{ScaleDown: &provTypes.ScaleDownPolicy{StabilizationWindow: new(int32), PercentagePolicyValue: new(int32), UnitsPolicyValue: new(int32)}}},
			expectErr: nil,
		},
		{
			param: &provTypes.AutoScaleSpec{Behavior: provTypes.BehaviorAutoScaleSpec{ScaleDown: &provTypes.ScaleDownPolicy{
				StabilizationWindow:   ptr.To(int32(50)),
				PercentagePolicyValue: ptr.To(int32(27)),
				UnitsPolicyValue:      ptr.To(int32(3)),
			}}},
			expectErr: nil,
		},
		{
			param: &provTypes.AutoScaleSpec{Behavior: provTypes.BehaviorAutoScaleSpec{ScaleDown: &provTypes.ScaleDownPolicy{
				StabilizationWindow:   ptr.To(int32(0)),
				PercentagePolicyValue: ptr.To(int32(0)),
				UnitsPolicyValue:      ptr.To(int32(0)),
			}}},
			expectErr: nil,
		},
	}
	for _, tt := range tests {
		err := ValidateAutoScaleDownSpec(tt.param)
		c.Assert(err, check.Equals, tt.expectErr)
	}
}
