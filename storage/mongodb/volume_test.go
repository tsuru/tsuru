// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"github.com/tsuru/tsuru/storage/storagetest"
	check "gopkg.in/check.v1"
)

var _ = check.Suite(&storagetest.VolumeSuite{
	VolumeStorage: &volumeStorage{},
	SuiteHooks:    &mongodbBaseTest{},
})
