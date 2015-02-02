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
	AddChecker("success", successChecker)
	AddChecker("failing", failingChecker)
	expected := []Result{
		{Name: "success", Status: HealthCheckOK},
		{Name: "failing", Status: "fail - something went wrong"},
	}
	result := Check()
	c.Assert(result, gocheck.DeepEquals, expected)
}


func successChecker() error {
	return nil
}

func failingChecker() error {
	return errors.New("something went wrong")
}
