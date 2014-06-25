// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"errors"
	"launchpad.net/gocheck"
	"reflect"
	"sort"
	"testing"
)

type ProvisionSuite struct{}

var _ = gocheck.Suite(ProvisionSuite{})

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

func (ProvisionSuite) TestRegisterAndGetProvisioner(c *gocheck.C) {
	var p Provisioner
	Register("my-provisioner", p)
	got, err := Get("my-provisioner")
	c.Assert(err, gocheck.IsNil)
	c.Check(got, gocheck.DeepEquals, p)
	_, err = Get("unknown-provisioner")
	c.Check(err, gocheck.NotNil)
	expectedMessage := `Unknown provisioner: "unknown-provisioner".`
	c.Assert(err.Error(), gocheck.Equals, expectedMessage)
}

func (ProvisionSuite) TestRegistry(c *gocheck.C) {
	var p1, p2 Provisioner
	Register("my-provisioner", p1)
	Register("your-provisioner", p2)
	provisioners := Registry()
	alt1 := []Provisioner{p1, p2}
	alt2 := []Provisioner{p2, p1}
	if !reflect.DeepEqual(provisioners, alt1) && !reflect.DeepEqual(provisioners, alt2) {
		c.Errorf("Registry(): Expected %#v. Got %#v.", alt1, provisioners)
	}
}

func (ProvisionSuite) TestError(c *gocheck.C) {
	errs := []*Error{
		{Reason: "something", Err: errors.New("went wrong")},
		{Reason: "something went wrong"},
	}
	expected := []string{"went wrong: something", "something went wrong"}
	for i := range errs {
		c.Check(errs[i].Error(), gocheck.Equals, expected[i])
	}
}

func (ProvisionSuite) TestErrorImplementsError(c *gocheck.C) {
	var _ error = &Error{}
}

func (ProvisionSuite) TestStatusString(c *gocheck.C) {
	var s Status = "pending"
	c.Assert(s.String(), gocheck.Equals, "pending")
}

func (ProvisionSuite) TestStatusUnreachable(c *gocheck.C) {
	c.Assert(StatusUnreachable.String(), gocheck.Equals, "unreachable")
}

func (ProvisionSuite) TestStatusBuilding(c *gocheck.C) {
	c.Assert(StatusBuilding.String(), gocheck.Equals, "building")
}

func (ProvisionSuite) TestUnitAvailable(c *gocheck.C) {
	var tests = []struct {
		input    Status
		expected bool
	}{
		{StatusStarted, true},
		{StatusUnreachable, true},
		{StatusBuilding, false},
		{StatusDown, false},
		{StatusError, false},
	}
	for _, test := range tests {
		u := Unit{Status: test.input}
		c.Check(u.Available(), gocheck.Equals, test.expected)
	}
}

func (ProvisionSuite) TestUnitGetIp(c *gocheck.C) {
	u := Unit{Ip: "10.3.3.1"}
	c.Assert(u.Ip, gocheck.Equals, u.GetIp())
}

func (ProvisionSuite) TestUnitSliceLen(c *gocheck.C) {
	units := UnitSlice{Unit{}, Unit{}}
	c.Assert(units.Len(), gocheck.Equals, 2)
}

func (ProvisionSuite) TestUnitSliceLess(c *gocheck.C) {
	units := UnitSlice{
		Unit{Name: "b", Status: StatusDown},
		Unit{Name: "d", Status: StatusBuilding},
		Unit{Name: "e", Status: StatusStarted},
		Unit{Name: "s", Status: StatusUnreachable},
	}
	c.Assert(units.Less(0, 1), gocheck.Equals, true)
	c.Assert(units.Less(1, 2), gocheck.Equals, true)
	c.Assert(units.Less(2, 0), gocheck.Equals, false)
	c.Assert(units.Less(3, 2), gocheck.Equals, true)
	c.Assert(units.Less(3, 1), gocheck.Equals, false)
}

func (ProvisionSuite) TestUnitSliceSwap(c *gocheck.C) {
	units := UnitSlice{
		Unit{Name: "b", Status: StatusDown},
		Unit{Name: "f", Status: StatusBuilding},
		Unit{Name: "g", Status: StatusStarted},
	}
	units.Swap(0, 1)
	c.Assert(units[0].Status, gocheck.Equals, StatusBuilding)
	c.Assert(units[1].Status, gocheck.Equals, StatusDown)
}

func (ProvisionSuite) TestUnitSliceSort(c *gocheck.C) {
	units := UnitSlice{
		Unit{Name: "f", Status: StatusBuilding},
		Unit{Name: "g", Status: StatusStarted},
		Unit{Name: "b", Status: StatusDown},
	}
	c.Assert(sort.IsSorted(units), gocheck.Equals, false)
	sort.Sort(units)
	c.Assert(sort.IsSorted(units), gocheck.Equals, true)
}
