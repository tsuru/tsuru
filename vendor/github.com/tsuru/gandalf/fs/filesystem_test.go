// Copyright 2015 gandalf authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"testing"

	tsurufs "github.com/tsuru/tsuru/fs"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (s *S) TestFsystemShouldSetGlobalFsystemWhenItsNil(c *check.C) {
	Fsystem = nil
	fsys := Filesystem()
	_, ok := fsys.(tsurufs.Fs)
	c.Assert(ok, check.Equals, true)
}
