// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

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
	fsystem = s.rfs
}

func (s *S) TestRandomBytes(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: string([]byte{16, 17})}
	fsystem = rfs
	defer func() {
		fsystem = s.rfs
	}()
	b, err := randomBytes(2)
	c.Assert(err, gocheck.IsNil)
	expected := "\x10\x11"
	c.Assert(string(b), gocheck.Equals, expected)
}
