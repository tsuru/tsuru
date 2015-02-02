// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iaas

import (
	"errors"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/hc"
	"launchpad.net/gocheck"
)

func (s *S) TestBuildHealthCheck(c *gocheck.C) {
	RegisterIaasProvider("hc", TestHealthCheckerIaaS{err: nil})
	config.Set("iaas:hc", "something")
	fn := BuildHealthCheck("hc")
	err := fn()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestBuildHealthCheckFailure(c *gocheck.C) {
	err := errors.New("fatal failure")
	RegisterIaasProvider("hc", TestHealthCheckerIaaS{err: err})
	config.Set("iaas:hc", "something")
	fn := BuildHealthCheck("hc")
	hcErr := fn()
	c.Assert(hcErr, gocheck.Equals, err)
}

func (s *S) TestBuildHealthCheckUnconfigured(c *gocheck.C) {
	if oldValue, err := config.Get("iaas"); err == nil {
		defer config.Set("iaas", oldValue)
	}
	config.Unset("iaas")
	fn := BuildHealthCheck("hc")
	err := fn()
	c.Assert(err, gocheck.Equals, hc.ErrDisabledComponent)
}

func (s *S) TestBuildHealthCheckNotChecker(c *gocheck.C) {
	RegisterIaasProvider("test-iaas", TestIaaS{})
	config.Set("iaas:test-iaas", "something")
	fn := BuildHealthCheck("test-iaas")
	err := fn()
	c.Assert(err, gocheck.Equals, hc.ErrDisabledComponent)
}
