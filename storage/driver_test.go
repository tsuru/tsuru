// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"testing"

	"github.com/tsuru/config"
	check "gopkg.in/check.v1"
)

type S struct {
}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) TearDownTest(c *check.C) {
	dbDrivers = make(map[string]DbDriver)
}

func (s *S) TestRegisterDbDriver(c *check.C) {
	RegisterDbDriver("mysql", DbDriver{})
	c.Assert(dbDrivers, check.HasLen, 1)
	RegisterDbDriver("postgres", DbDriver{})
	c.Assert(dbDrivers, check.HasLen, 2)
}

func (s *S) TestRegisterDbDriverIgnoresDuplicates(c *check.C) {
	RegisterDbDriver("mysql", DbDriver{})
	RegisterDbDriver("mysql", DbDriver{})
	RegisterDbDriver("mysql", DbDriver{})
	c.Assert(dbDrivers, check.HasLen, 1)
}

func (s *S) TestGetDbDriver(c *check.C) {
	RegisterDbDriver("mysql", DbDriver{})
	driver, err := GetDbDriver("mysql")
	c.Assert(err, check.IsNil)
	c.Assert(driver, check.NotNil)
	driver, err = GetDbDriver("postgres")
	c.Assert(err, check.NotNil)
	c.Assert(driver, check.IsNil)
}

func (s *S) TestGetCurrentDbDriver(c *check.C) {
	RegisterDbDriver("mysql", DbDriver{})
	driver, err := GetCurrentDbDriver()
	c.Assert(err, check.NotNil)
	c.Assert(driver, check.IsNil)
	config.Set("database:driver", "mysql")
	defer config.Unset("database:driver")
	driver, err = GetCurrentDbDriver()
	c.Assert(err, check.IsNil)
	c.Assert(driver, check.NotNil)
}

func (s *S) TestGetDefaultDbDriver(c *check.C) {
	RegisterDbDriver(DefaultDbDriverName, DbDriver{})
	driver, err := GetDefaultDbDriver()
	c.Assert(err, check.IsNil)
	c.Assert(driver, check.NotNil)
}
