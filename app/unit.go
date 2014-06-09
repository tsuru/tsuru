// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/tsuru/tsuru/provision"
)

// UnitSlice attaches the methods of sort.Interface to []Unit, sorting in increasing order.
type UnitSlice []provision.Unit

func (u UnitSlice) Len() int {
	return len(u)
}

func (u UnitSlice) Less(i, j int) bool {
	weight := map[provision.Status]int{
		provision.StatusDown:        0,
		provision.StatusBuilding:    1,
		provision.StatusUnreachable: 2,
		provision.StatusStarted:     3,
	}
	return weight[u[i].Status] < weight[u[j].Status]
}

func (u UnitSlice) Swap(i, j int) {
	u[i], u[j] = u[j], u[i]
}
