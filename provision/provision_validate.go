package provision

import (
	"github.com/pkg/errors"
	provTypes "github.com/tsuru/tsuru/types/provision"
)

func ValidateAutoScaleDownSpec(autoScaleSpec *provTypes.AutoScaleSpec) error {
	if autoScaleSpec == nil {
		return nil
	}
	if autoScaleSpec.Behavior.ScaleDown == nil {
		return nil
	}
	scaleDown := autoScaleSpec.Behavior.ScaleDown
	if scaleDown.PercentagePolicyValue != nil && *scaleDown.PercentagePolicyValue < 0 {
		return errors.New("not enough percentage to scale down")
	}
	if scaleDown.StabilizationWindow != nil && *scaleDown.StabilizationWindow < 0 {
		return errors.New("not enough stabilization window to scale down")
	}
	if scaleDown.UnitsPolicyValue != nil && *scaleDown.UnitsPolicyValue < 0 {
		return errors.New("not enough units to scale down")
	}
	return nil
}
