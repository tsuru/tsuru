// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/tsuru/tsuru/storage/storagetest"
	check "gopkg.in/check.v1"
)

var _ = check.Suite(&storagetest.AppVersionSuite{
	AppVersionStorage: &appVersionStorage{},
	SuiteHooks:        &mongodbBaseTest{name: "appversion"},
})

type appVersionSuite struct {
	storagetest.SuiteHooks
}

var _ = check.Suite(&appVersionSuite{
	SuiteHooks: &mongodbBaseTest{name: "appversion-internal"},
})
