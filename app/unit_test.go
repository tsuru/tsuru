// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/tsuru/tsuru/provision"
	"launchpad.net/gocheck"
	"sort"
)

func (s *S) TestUnitSliceLen(c *gocheck.C) {
	units := UnitSlice{provision.Unit{}, provision.Unit{}}
	c.Assert(units.Len(), gocheck.Equals, 2)
}

func (s *S) TestUnitSliceLess(c *gocheck.C) {
	units := UnitSlice{
		provision.Unit{Name: "b", Status: provision.StatusDown},
		provision.Unit{Name: "d", Status: provision.StatusBuilding},
		provision.Unit{Name: "e", Status: provision.StatusStarted},
		provision.Unit{Name: "s", Status: provision.StatusUnreachable},
	}
	c.Assert(units.Less(0, 1), gocheck.Equals, true)
	c.Assert(units.Less(1, 2), gocheck.Equals, true)
	c.Assert(units.Less(2, 0), gocheck.Equals, false)
	c.Assert(units.Less(3, 2), gocheck.Equals, true)
	c.Assert(units.Less(3, 1), gocheck.Equals, false)
}

func (s *S) TestUnitSliceSwap(c *gocheck.C) {
	units := UnitSlice{
		provision.Unit{Name: "b", Status: provision.StatusDown},
		provision.Unit{Name: "f", Status: provision.StatusBuilding},
		provision.Unit{Name: "g", Status: provision.StatusStarted},
	}
	units.Swap(0, 1)
	c.Assert(units[0].Status, gocheck.Equals, provision.StatusBuilding)
	c.Assert(units[1].Status, gocheck.Equals, provision.StatusDown)
}

func (s *S) TestUnitSliceSort(c *gocheck.C) {
	units := UnitSlice{
		provision.Unit{Name: "f", Status: provision.StatusBuilding},
		provision.Unit{Name: "g", Status: provision.StatusStarted},
		provision.Unit{Name: "b", Status: provision.StatusDown},
	}
	c.Assert(sort.IsSorted(units), gocheck.Equals, false)
	sort.Sort(units)
	c.Assert(sort.IsSorted(units), gocheck.Equals, true)
}
