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
		TargetType: event.TargetTypeNode,
		KindName:   "healer",
		Time:       consecutiveHealingsTimeframe,
		Max:        consecutiveHealingsLimitInTimeframe,
	})
}
