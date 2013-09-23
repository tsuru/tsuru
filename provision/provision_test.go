// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"errors"
	"launchpad.net/gocheck"
	"reflect"
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
