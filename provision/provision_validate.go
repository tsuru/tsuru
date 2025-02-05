// Copyright 2025 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
	"github.com/tsuru/tsuru/validation"
)

func ValidateAutoScaleSpec(spec *provTypes.AutoScaleSpec, quotaLimit int, a *appTypes.App) error {
	if spec.MinUnits == 0 {
		return errors.New("minimum units must be greater than 0")
	}
	if spec.MaxUnits <= spec.MinUnits {
		return errors.New("maximum units must be greater than minimum units")
	}
	if quotaLimit > 0 && spec.MaxUnits > uint(quotaLimit) {
		return errors.New("maximum units cannot be greater than quota limit")
	}
	if spec.AverageCPU == "" && len(spec.Schedules) == 0 && len(spec.Prometheus) == 0 {
		return errors.New("you have to configure at least one trigger between cpu, schedule and prometheus")
	}
	if spec.AverageCPU != "" {
		_, err := CPUValueOfAutoScaleSpec(spec, a)
		if err != nil {
			return err
		}
	}

	err := ValidateAutoScaleSchedule(spec.Schedules)
	if err != nil {
		return err
	}

	err = ValidateAutoScalePrometheus(spec.Prometheus)
	if err != nil {
		return err
	}

	err = ValidateAutoScaleDownSpec(spec)
	if err != nil {
		return err
	}

	return nil
}

func ValidateAutoScaleSchedule(schedules []provTypes.AutoScaleSchedule) error {
	for _, schedule := range schedules {
		if !validation.ValidateName(schedule.Name) {
			return fmt.Errorf("\"%s\" is an invalid name, it must contain only lower case letters, numbers or dashes and starts with a letter", schedule.Name)
		}

		_, err := cron.ParseStandard(schedule.Start)
		if err != nil {
			return fmt.Errorf("invalid start time for schedule %q: %v", schedule.Name, err)
		}

		_, err = cron.ParseStandard(schedule.End)
		if err != nil {
			return fmt.Errorf("invalid end time for schedule %q: %v", schedule.Name, err)
		}
	}
	return nil
}

func ValidateAutoScalePrometheus(prometheus []provTypes.AutoScalePrometheus) error {
	for _, prom := range prometheus {
		if !validation.ValidateName(prom.Name) {
			return fmt.Errorf("\"%s\" is an invalid name, it must contain only lower case letters, numbers or dashes and starts with a letter", prom.Name)
		}

		if prom.Threshold <= 0 {
			return fmt.Errorf("prometheus threshold of name %q must be greater than 0", prom.Name)
		}

		if prom.ActivationThreshold < 0 {
			return fmt.Errorf("prometheus activationThreshold of name %q must be greater than 0", prom.Name)
		}
	}
	return nil
}

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
