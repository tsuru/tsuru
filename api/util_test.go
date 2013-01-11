// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/globocom/tsuru/fs"
	"github.com/globocom/tsuru/fs/testing"
	. "launchpad.net/gocheck"
)

type UtilSuite struct {
	rfs *testing.RecordingFs
}

var _ = Suite(&UtilSuite{})

func (s *UtilSuite) SetUpSuite(c *C) {
	s.rfs = &testing.RecordingFs{}
	file, err := s.rfs.Open("/dev/urandom")
	c.Assert(err, IsNil)
	file.Write([]byte{16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31})
	fsystem = s.rfs
}

func (s *UtilSuite) TearDownSuite(c *C) {
	fsystem = nil
}

func (s *UtilSuite) TestFileSystem(c *C) {
	fsystem = &testing.RecordingFs{}
	c.Assert(filesystem(), DeepEquals, fsystem)
	fsystem = nil
	c.Assert(filesystem(), DeepEquals, fs.OsFs{})
	fsystem = s.rfs
}
