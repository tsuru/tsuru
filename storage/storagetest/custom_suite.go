// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	check "gopkg.in/check.v1"
)

type SuiteHooks interface {
	SetUpSuite(c *check.C)
	SetUpTest(c *check.C)
	TearDownTest(c *check.C)
	TearDownSuite(c *check.C)
}
