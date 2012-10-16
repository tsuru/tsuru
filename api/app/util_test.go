// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"github.com/globocom/tsuru/fs/testing"
	. "launchpad.net/gocheck"
)

func (s *S) TestnewUUID(c *C) {
	rfs := &testing.RecordingFs{FileContent: string([]byte{16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31})}
	fsystem = rfs
	defer func() {
		fsystem = s.rfs
	}()
	uuid, err := newUUID()
	c.Assert(err, IsNil)
	expected := "101112131415161718191a1b1c1d1e1f"
	c.Assert(uuid, Equals, expected)
}

func (s *S) TestRandomBytes(c *C) {
	rfs := &testing.RecordingFs{FileContent: string([]byte{16, 17})}
	fsystem = rfs
	defer func() {
		fsystem = s.rfs
	}()
	b, err := randomBytes(2)
	c.Assert(err, IsNil)
	expected := "\x10\x11"
	c.Assert(string(b), Equals, expected)
}
