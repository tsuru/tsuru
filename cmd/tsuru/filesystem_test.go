// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/fs"
	"github.com/globocom/tsuru/fs/testing"
	"launchpad.net/gocheck"
)

func (s *S) TestFileSystem(c *gocheck.C) {
	fsystem = &testing.RecordingFs{}
	c.Assert(filesystem(), gocheck.DeepEquals, fsystem)
	fsystem = nil
	c.Assert(filesystem(), gocheck.DeepEquals, fs.OsFs{})
}
