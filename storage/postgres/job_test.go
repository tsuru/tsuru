// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"github.com/tsuru/tsuru/storage/storagetest"
	check "gopkg.in/check.v1"
)

var _ = check.Suite(&storagetest.JobSuite{
	JobStorage: &JobStorage{},
	SuiteHooks: &postgresBaseTest{},
})
