// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hc

import (
	"errors"
	"testing"

	check "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type HCSuite struct{}

var _ = check.Suite(HCSuite{})

func (HCSuite) SetUpTest(c *check.C) {
	checkers = nil
}

func (HCSuite) TestCheckAll(c *check.C) {
	AddChecker("success", successChecker)
	AddChecker("failing", failingChecker)
	AddChecker("disabled", disabledChecker)
	expected := []Result{
		{Name: "success", Status: HealthCheckOK},
		{Name: "failing", Status: "fail - something went wrong"},
	}
	result := Check("all")
	expected[0].Duration = result[0].Duration
	expected[1].Duration = result[1].Duration
	c.Assert(result, check.DeepEquals, expected)
	c.Assert(result[0].Duration, check.Not(check.Equals), 0)
	c.Assert(result[1].Duration, check.Not(check.Equals), 0)
}

func (HCSuite) TestCheckFiltered(c *check.C) {
	AddChecker("success1", successChecker)
	AddChecker("success2", successChecker)
	AddChecker("failing1", failingChecker)
	expected := []Result{
		{Name: "success1", Status: HealthCheckOK},
		{Name: "failing1", Status: "fail - something went wrong"},
	}
	result := Check("success1", "failing1")
	expected[0].Duration = result[0].Duration
	expected[1].Duration = result[1].Duration
	c.Assert(result, check.DeepEquals, expected)
	c.Assert(result[0].Duration, check.Not(check.Equals), 0)
	c.Assert(result[1].Duration, check.Not(check.Equals), 0)
	expected = []Result{
		{Name: "success2", Status: HealthCheckOK},
	}
	result = Check("success2")
	expected[0].Duration = result[0].Duration
	c.Assert(result, check.DeepEquals, expected)
}

func successChecker() error {
	return nil
}

func failingChecker() error {
	return errors.New("something went wrong")
}

func disabledChecker() error {
	return ErrDisabledComponent
}
