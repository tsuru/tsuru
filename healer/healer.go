// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/event"
)

var (
	consecutiveHealingsTimeframe        = 300
	consecutiveHealingsLimitInTimeframe = 3

	HealerInstance *NodeHealer
)

func Initialize() (*NodeHealer, error) {
	if HealerInstance != nil {
		return nil, errors.New("healer already initialized")
	}
	autoHealingNodes, err := config.GetBool("docker:healing:heal-nodes")
	if err != nil {
		autoHealingNodes = true
	}
	if !autoHealingNodes {
		return nil, nil
	}
	disabledSeconds, _ := config.GetInt("docker:healing:disabled-time")
	if disabledSeconds <= 0 {
		disabledSeconds = 30
	}
	maxFailures, _ := config.GetInt("docker:healing:max-failures")
	if maxFailures <= 0 {
		maxFailures = 5
	}
	waitSecondsNewMachine, _ := config.GetInt("docker:healing:wait-new-time")
	if waitSecondsNewMachine <= 0 {
		waitSecondsNewMachine = 5 * 60
	}
	event.SetThrottlingFromConfig(event.ThrottlingSpec{
		TargetType: event.TargetTypeNode,
		KindName:   "healer",
		Time:       time.Duration(consecutiveHealingsTimeframe) * time.Second,
		Max:        consecutiveHealingsLimitInTimeframe,
		AllTargets: false,
		WaitFinish: false,
	}, "docker:healing:node-rate-limit")
	HealerInstance = newNodeHealer(nodeHealerArgs{
		DisabledTime:          time.Duration(disabledSeconds) * time.Second,
		WaitTimeNewMachine:    time.Duration(waitSecondsNewMachine) * time.Second,
		FailuresBeforeHealing: maxFailures,
	})
	shutdown.Register(HealerInstance)
	return HealerInstance, nil
}
