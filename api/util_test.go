// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/globocom/tsuru/fs"
	"github.com/globocom/tsuru/fs/testing"
	"launchpad.net/gocheck"
)

type UtilSuite struct {
	rfs *testing.RecordingFs
}

var _ = gocheck.Suite(&UtilSuite{})

func (s *UtilSuite) SetUpSuite(c *gocheck.C) {
	s.rfs = &testing.RecordingFs{}
	file, err := s.rfs.Create("/dev/urandom")
	c.Assert(err, gocheck.IsNil)
	file.Write([]byte{16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31})
	fsystem = s.rfs
}

func (s *UtilSuite) TearDownSuite(c *gocheck.C) {
	fsystem = nil
}

func (s *UtilSuite) TestFileSystem(c *gocheck.C) {
	fsystem = &testing.RecordingFs{}
	c.Assert(filesystem(), gocheck.DeepEquals, fsystem)
	fsystem = nil
	c.Assert(filesystem(), gocheck.DeepEquals, fs.OsFs{})
	fsystem = s.rfs
}
