package provision_test

import (
	"github.com/tsuru/tsuru/provision"
	provTypes "github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
	"k8s.io/utils/ptr"
)

func (s *S) TestValidateAutoScaleUpSpec_ReturnError(c *check.C) {
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
		err := provision.ValidateAutoScaleDownSpec(tt.param)
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
		err := provision.ValidateAutoScaleDownSpec(tt.param)
		c.Assert(err, check.Equals, tt.expectErr)
	}
}
