// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"reflect"
	"testing"

	"github.com/pkg/errors"
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
	var s Status = "pending"
	c.Assert(s.String(), check.Equals, "pending")
}

func (ProvisionSuite) TestStatuses(c *check.C) {
	c.Check(StatusCreated.String(), check.Equals, "created")
	c.Check(StatusBuilding.String(), check.Equals, "building")
	c.Check(StatusError.String(), check.Equals, "error")
	c.Check(StatusStarted.String(), check.Equals, "started")
	c.Check(StatusStopped.String(), check.Equals, "stopped")
	c.Check(StatusStarting.String(), check.Equals, "starting")
}

func (ProvisionSuite) TestParseStatus(c *check.C) {
	var tests = []struct {
		input  string
		output Status
		err    error
	}{
		{"created", StatusCreated, nil},
		{"building", StatusBuilding, nil},
		{"error", StatusError, nil},
		{"started", StatusStarted, nil},
		{"stopped", StatusStopped, nil},
		{"starting", StatusStarting, nil},
		{"something", Status(""), ErrInvalidStatus},
		{"otherthing", Status(""), ErrInvalidStatus},
	}
	for _, t := range tests {
		got, err := ParseStatus(t.input)
		c.Check(got, check.Equals, t.output)
		c.Check(err, check.Equals, t.err)
	}
}

func (ProvisionSuite) TestUnitAvailable(c *check.C) {
	var tests = []struct {
		input    Status
		expected bool
	}{
		{StatusCreated, false},
		{StatusStarting, true},
		{StatusStarted, true},
		{StatusBuilding, false},
		{StatusError, true},
	}
	for _, test := range tests {
		u := Unit{Status: test.input}
		c.Check(u.Available(), check.Equals, test.expected)
	}
}

func (ProvisionSuite) TestUnitGetIp(c *check.C) {
	u := Unit{IP: "10.3.3.1"}
	c.Assert(u.IP, check.Equals, u.GetIp())
}

func (ProvisionSuite) TestUnitNotFoundError(c *check.C) {
	var err error = &UnitNotFoundError{ID: "some unit"}
	c.Assert(err.Error(), check.Equals, `unit "some unit" not found`)
}

func (ProvisionSuite) TestValidate(c *check.C) {
	var tests = []struct {
		input    AutoScaleSpec
		expected string
	}{
		{
			AutoScaleSpec{
				MinUnits: 11,
				MaxUnits: 10,
			},
			"maximum units must be greater than minimum units",
		},
		{
			AutoScaleSpec{
				MinUnits: 10,
				MaxUnits: 10,
			},
			"maximum units must be greater than minimum units",
		},
		{
			AutoScaleSpec{
				MinUnits: 10,
				MaxUnits: 20,
			},
			"maximum units cannot be greater than quota limit",
		},
	}

	for _, test := range tests {
		err := test.input.Validate(10, nil)
		c.Check(err, check.ErrorMatches, test.expected)
	}
}
