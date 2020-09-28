// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iaas

import (
	"context"
	"errors"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/hc"
	check "gopkg.in/check.v1"
)

func (s *S) TestBuildHealthCheck(c *check.C) {
	RegisterIaasProvider("hc", newTestHealthcheckIaaS)
	config.Set("iaas:hc", "something")
	fn := BuildHealthCheck("hc")
	err := fn(context.TODO())
	c.Assert(err, check.IsNil)
}

func (s *S) TestBuildHealthCheckFailure(c *check.C) {
	err := errors.New("fatal failure")
	RegisterIaasProvider("hc", newTestHealthcheckIaaS)
	iaas, getErr := getIaasProvider("hc")
	c.Assert(getErr, check.IsNil)
	hcIaas := iaas.(*TestHealthCheckerIaaS)
	hcIaas.err = err
	config.Set("iaas:hc", "something")
	fn := BuildHealthCheck("hc")
	hcErr := fn(context.TODO())
	c.Assert(hcErr, check.Equals, err)
}

func (s *S) TestBuildHealthCheckUnconfigured(c *check.C) {
	if oldValue, err := config.Get("iaas"); err == nil {
		defer config.Set("iaas", oldValue)
	}
	config.Unset("iaas")
	fn := BuildHealthCheck("hc")
	err := fn(context.TODO())
	c.Assert(err, check.Equals, hc.ErrDisabledComponent)
}

func (s *S) TestBuildHealthCheckNotChecker(c *check.C) {
	config.Set("iaas:test-iaas", "something")
	fn := BuildHealthCheck("test-iaas")
	err := fn(context.TODO())
	c.Assert(err, check.Equals, hc.ErrDisabledComponent)
}
