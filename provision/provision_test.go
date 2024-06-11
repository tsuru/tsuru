// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"reflect"
	"testing"

	"github.com/pkg/errors"
	provTypes "github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
)

type ProvisionSuite struct{}

var _ = check.Suite(ProvisionSuite{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s ProvisionSuite) SetUpTest(c *check.C) {
	provisioners = make(map[string]provisionerFactory)
}

func (ProvisionSuite) TestRegisterAndGetProvisioner(c *check.C) {
	var p Provisioner
	Register("my-provisioner", func() (Provisioner, error) { return p, nil })
	got, err := Get("my-provisioner")
	c.Assert(err, check.IsNil)
	c.Check(got, check.DeepEquals, p)
	_, err = Get("unknown-provisioner")
	c.Check(err, check.NotNil)
	expectedMessage := `unknown provisioner: "unknown-provisioner"`
	c.Assert(err.Error(), check.Equals, expectedMessage)
}

func (ProvisionSuite) TestRegistry(c *check.C) {
	var p1, p2 Provisioner
	Register("my-provisioner", func() (Provisioner, error) { return p1, nil })
	Register("your-provisioner", func() (Provisioner, error) { return p2, nil })
	provisioners, err := Registry()
	c.Assert(err, check.IsNil)
	alt1 := []Provisioner{p1, p2}
	alt2 := []Provisioner{p2, p1}
	if !reflect.DeepEqual(provisioners, alt1) && !reflect.DeepEqual(provisioners, alt2) {
		c.Errorf("Registry(): Expected %#v. Got %#v.", alt1, provisioners)
	}
}

func (ProvisionSuite) TestError(c *check.C) {
	errs := []*Error{
		{Reason: "something", Err: errors.New("went wrong")},
		{Reason: "something went wrong"},
	}
	expected := []string{"went wrong: something", "something went wrong"}
	for i := range errs {
		c.Check(errs[i].Error(), check.Equals, expected[i])
	}
}

func (ProvisionSuite) TestErrorImplementsError(c *check.C) {
	var _ error = &Error{}
}

func (ProvisionSuite) TestStatusString(c *check.C) {
	var s provTypes.UnitStatus = "pending"
	c.Assert(s.String(), check.Equals, "pending")
}

func (ProvisionSuite) TestStatuses(c *check.C) {
	c.Check(provTypes.UnitStatusCreated.String(), check.Equals, "created")
	c.Check(provTypes.UnitStatusBuilding.String(), check.Equals, "building")
	c.Check(provTypes.UnitStatusError.String(), check.Equals, "error")
	c.Check(provTypes.UnitStatusStarted.String(), check.Equals, "started")
	c.Check(provTypes.UnitStatusStopped.String(), check.Equals, "stopped")
	c.Check(provTypes.UnitStatusStarting.String(), check.Equals, "starting")
}

func (ProvisionSuite) TestParseStatus(c *check.C) {
	var tests = []struct {
		input  string
		output provTypes.UnitStatus
		err    error
	}{
		{"created", provTypes.UnitStatusCreated, nil},
		{"building", provTypes.UnitStatusBuilding, nil},
		{"error", provTypes.UnitStatusError, nil},
		{"started", provTypes.UnitStatusStarted, nil},
		{"stopped", provTypes.UnitStatusStopped, nil},
		{"starting", provTypes.UnitStatusStarting, nil},
		{"something", provTypes.UnitStatus(""), provTypes.ErrInvalidUnitStatus},
		{"otherthing", provTypes.UnitStatus(""), provTypes.ErrInvalidUnitStatus},
	}
	for _, t := range tests {
		got, err := provTypes.ParseUnitStatus(t.input)
		c.Check(got, check.Equals, t.output)
		c.Check(err, check.Equals, t.err)
	}
}

func (ProvisionSuite) TestUnitAvailable(c *check.C) {
	var tests = []struct {
		input    provTypes.UnitStatus
		expected bool
	}{
		{provTypes.UnitStatusCreated, false},
		{provTypes.UnitStatusStarting, true},
		{provTypes.UnitStatusStarted, true},
		{provTypes.UnitStatusBuilding, false},
		{provTypes.UnitStatusError, true},
	}
	for _, test := range tests {
		u := provTypes.Unit{Status: test.input}
		c.Check(u.Available(), check.Equals, test.expected)
	}
}

func (ProvisionSuite) TestUnitGetIp(c *check.C) {
	u := provTypes.Unit{IP: "10.3.3.1"}
	c.Assert(u.IP, check.Equals, u.GetIp())
}

func (ProvisionSuite) TestUnitNotFoundError(c *check.C) {
	var err error = &UnitNotFoundError{ID: "some unit"}
	c.Assert(err.Error(), check.Equals, `unit "some unit" not found`)
}

func (ProvisionSuite) TestValidate(c *check.C) {
	var tests = []struct {
		input    provTypes.AutoScaleSpec
		expected string
	}{
		{
			provTypes.AutoScaleSpec{
				MinUnits: 0,
				MaxUnits: 10,
			},
			"minimum units must be greater than 0",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 11,
				MaxUnits: 10,
			},
			"maximum units must be greater than minimum units",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 10,
				MaxUnits: 10,
			},
			"maximum units must be greater than minimum units",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 10,
				MaxUnits: 20,
			},
			"maximum units cannot be greater than quota limit",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 2,
			},
			"you have to configure at least one trigger between cpu, schedule and prometheus",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 2,
				Prometheus: []provTypes.AutoScalePrometheus{{
					Name: "Invalid-Name",
				}},
			},
			"\"Invalid-Name\" is an invalid name, it must contain only lower case letters, numbers or dashes and starts with a letter",
		},
		{
			provTypes.AutoScaleSpec{
				MinUnits: 1,
				MaxUnits: 2,
				Prometheus: []provTypes.AutoScalePrometheus{{
					Name: "valid-name",
				}, {
					Name: "another$invalid",
				}},
			},
			"\"another$invalid\" is an invalid name, it must contain only lower case letters, numbers or dashes and starts with a letter",
		},
	}

	for _, test := range tests {
		err := ValidateAutoScaleSpec(&test.input, 10, nil)
		c.Assert(err, check.NotNil)
		c.Assert(err.Error(), check.Equals, test.expected)
	}
}
