// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	check "gopkg.in/check.v1"
)

func (s *S) TestPasswordFromReaderUsingFile(c *check.C) {
	tmpdir, err := filepath.EvalSymlinks(os.TempDir())
	filename := path.Join(tmpdir, "password-reader.txt")
	c.Assert(err, check.IsNil)
	file, err := os.Create(filename)
	c.Assert(err, check.IsNil)
	defer os.Remove(filename)
	file.WriteString("hello")
	file.Seek(0, 0)
	password, err := PasswordFromReader(file)
	c.Assert(err, check.IsNil)
	c.Assert(password, check.Equals, "hello")
}

func (s *S) TestPasswordFromReaderUsingStringsReader(c *check.C) {
	reader := strings.NewReader("abcd\n")
	password, err := PasswordFromReader(reader)
	c.Assert(err, check.IsNil)
	c.Assert(password, check.Equals, "abcd")
}
