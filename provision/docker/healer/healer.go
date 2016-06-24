// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"time"

	"github.com/tsuru/tsuru/event"
)

var (
	consecutiveHealingsTimeframe        = 5 * time.Minute
	consecutiveHealingsLimitInTimeframe = 3
)

func init() {
	event.SetThrottling(event.ThrottlingSpec{
		TargetName: "container",
		KindName:   "healer",
		Time:       consecutiveHealingsTimeframe,
		Max:        consecutiveHealingsLimitInTimeframe,
	})
	event.SetThrottling(event.ThrottlingSpec{
		TargetName: "node",
		KindName:   "healer",
		Time:       consecutiveHealingsTimeframe,
		Max:        consecutiveHealingsLimitInTimeframe,
	})
}

func ListHealingHistory(filter string) ([]event.Event, error) {
	evtFilter := event.Filter{
		KindName: "healer",
		KindType: event.KindTypeInternal,
	}
	if filter != "" {
		evtFilter.Target = event.Target{Name: filter}
	}
	return event.List(&evtFilter)
}
