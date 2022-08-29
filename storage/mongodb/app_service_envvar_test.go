// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	check "gopkg.in/check.v1"

	"github.com/tsuru/tsuru/storage/storagetest"
)

var _ = check.Suite(&storagetest.AppServiceEnvVarSuite{
	AppServiceEnvVarStorage: &appServiceEnvVarStorage{},
	SuiteHooks:              &mongodbBaseTest{name: "app_service_envvar"},
})
