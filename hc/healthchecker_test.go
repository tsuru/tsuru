// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hc

import (
	"errors"
	"testing"

	"launchpad.net/gocheck"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type HCSuite struct{}

var _ = gocheck.Suite(HCSuite{})

func (HCSuite) TestCheck(c *gocheck.C) {
	AddChecker(SuccessChecker{})
	AddChecker(&PointerChecker{})
	AddChecker(FailingChecker{})
	expected := []Result{
		{Name: "SuccessChecker", Status: HealthCheckOK},
		{Name: "PointerChecker", Status: HealthCheckOK},
		{Name: "FailingChecker", Status: "fail - something went wrong"},
	}
	result := Check()
	c.Assert(result, gocheck.DeepEquals, expected)
}

type SuccessChecker struct{}

func (SuccessChecker) HealthCheck() error {
	return nil
}

type PointerChecker struct{}

func (*PointerChecker) HealthCheck() error {
	return nil
}

type FailingChecker struct{}

func (FailingChecker) HealthCheck() error {
	return errors.New("something went wrong")
}
