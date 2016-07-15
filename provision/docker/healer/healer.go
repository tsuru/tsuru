// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"fmt"
	"time"

	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision/docker/container"
)

const (
	containerTargetName = "container"
	nodeTargetName      = "node"
)

var (
	consecutiveHealingsTimeframe        = 5 * time.Minute
	consecutiveHealingsLimitInTimeframe = 3
)

type HealingEvent struct {
	ID               interface{}
	StartTime        time.Time
	EndTime          time.Time
	Action           string
	Reason           string
	Extra            interface{}
	FailingNode      cluster.Node
	CreatedNode      cluster.Node
	FailingContainer container.Container
	CreatedContainer container.Container
	Successful       bool
	Error            string
}

func init() {
	event.SetThrottling(event.ThrottlingSpec{
		TargetName: containerTargetName,
		KindName:   "healer",
		Time:       consecutiveHealingsTimeframe,
		Max:        consecutiveHealingsLimitInTimeframe,
	})
	event.SetThrottling(event.ThrottlingSpec{
		TargetName: nodeTargetName,
		KindName:   "healer",
		Time:       consecutiveHealingsTimeframe,
		Max:        consecutiveHealingsLimitInTimeframe,
	})
}

func toHealingEvt(evt *event.Event) (HealingEvent, error) {
	healingEvt := HealingEvent{
		ID:         evt.UniqueID,
		StartTime:  evt.StartTime,
		EndTime:    evt.EndTime,
		Action:     fmt.Sprintf("%s-healing", evt.Target.Name),
		Successful: evt.Error == "",
		Error:      evt.Error,
	}
	switch evt.Target.Name {
	case containerTargetName:
		err := evt.StartData(&healingEvt.FailingContainer)
		if err != nil {
			return healingEvt, err
		}
		err = evt.EndData(&healingEvt.CreatedContainer)
		if err != nil {
			return healingEvt, err
		}
	case nodeTargetName:
		var data nodeHealerCustomData
		err := evt.StartData(&data)
		if err != nil {
			return healingEvt, err
		}
		healingEvt.Extra = data.LastCheck
		healingEvt.Reason = data.Reason
		if data.Node != nil {
			healingEvt.FailingNode = *data.Node
		}
		var createdNode cluster.Node
		err = evt.EndData(&createdNode)
		if err != nil {
			return healingEvt, err
		}
		healingEvt.CreatedNode = createdNode
	}

	return healingEvt, nil
}

func ListHealingHistory(filter string) ([]HealingEvent, error) {
	evtFilter := event.Filter{
		KindName: "healer",
		KindType: event.KindTypeInternal,
	}
	if filter != "" {
		evtFilter.Target = event.Target{Name: filter}
	}
	evts, err := event.List(&evtFilter)
	if err != nil {
		return nil, err
	}
	healingEvts := make([]HealingEvent, len(evts))
	for i := range evts {
		healingEvts[i], err = toHealingEvt(&evts[i])
		if err != nil {
			return nil, err
		}
	}
	return healingEvts, nil
}
