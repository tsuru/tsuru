// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

type AppQuotaSuite struct {
	SuiteHooks
	AppQuotaStorage appTypes.AppQuotaStorage
	AppQuotaService appTypes.AppQuotaService
}

func (s *AppQuotaSuite) TestIncInUse(c *check.C) {
	q := &appTypes.AppQuota{AppName: "myapp", Limit: 2, InUse: 0}
	err := s.AppQuotaStorage.IncInUse(q, 1)
	c.Assert(err, check.Equals, nil)
}

func (s *AppQuotaSuite) TestSetLimit(c *check.C) {
	q := &appTypes.AppQuota{AppName: "myapp", Limit: 2, InUse: 0}
	err := s.AppQuotaStorage.SetLimit(q.AppName, 1)
	c.Assert(err, check.Equals, nil)
}
