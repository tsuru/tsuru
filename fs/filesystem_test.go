// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"os"
	"testing"

	"gopkg.in/check.v1"
)

type S struct{}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) TestOsFsImplementsFS(c *check.C) {
	var _ Fs = OsFs{}
}

func (s *S) TestOsFsCreatesTheFileInTheDisc(c *check.C) {
	path := "/tmp/test-fs-tsuru"
	os.Remove(path)
	defer os.Remove(path)
	fs := OsFs{}
	f, err := fs.Create(path)
	c.Assert(err, check.IsNil)
	defer f.Close()
	_, err = os.Stat(path)
	c.Assert(err, check.IsNil)
}

func (s *S) TestOsFsOpenFile(c *check.C) {
	path := "/tmp/test-fs-tsuru"
	os.Remove(path)
	defer os.Remove(path)
	fs := OsFs{}
	f, err := fs.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	c.Assert(err, check.IsNil)
	defer f.Close()
	_, ok := f.(*os.File)
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestOsFsMkdirWritesTheDirectoryInTheDisc(c *check.C) {
	path := "/tmp/test-fs-tsuru"
	os.RemoveAll(path)
	defer os.RemoveAll(path)
	fs := OsFs{}
	err := fs.Mkdir(path, 0755)
	c.Assert(err, check.IsNil)
	fi, err := os.Stat(path)
	c.Assert(err, check.IsNil)
	c.Assert(fi.IsDir(), check.Equals, true)
}

func (s *S) TestOsFsMkdirAllWritesAllDirectoriesInTheDisc(c *check.C) {
	root := "/tmp/test-fs-tsuru"
	path := root + "/path"
	paths := []string{root, path}
	for _, path := range paths {
		os.RemoveAll(path)
		defer os.RemoveAll(path)
	}
	fs := OsFs{}
	err := fs.MkdirAll(path, 0755)
	c.Assert(err, check.IsNil)
	for _, path := range paths {
		fi, err := os.Stat(path)
		c.Assert(err, check.IsNil)
		c.Assert(fi.IsDir(), check.Equals, true)
	}
}

func (s *S) TestOsFsOpenOpensTheFileFromDisc(c *check.C) {
	path := "/tmp/test-fs-tsuru"
	unknownPath := "/tmp/test-fs-tsuru-unknown"
	os.Remove(unknownPath)
	defer os.Remove(path)
	f, err := os.Create(path)
	c.Assert(err, check.IsNil)
	f.Close()
	fs := OsFs{}
	file, err := fs.Open(path)
	c.Assert(err, check.IsNil)
	file.Close()
	_, err = fs.Open(unknownPath)
	c.Assert(err, check.NotNil)
	c.Assert(os.IsNotExist(err), check.Equals, true)
}

func (s *S) TestOsFsRemoveDeletesTheFileFromDisc(c *check.C) {
	path := "/tmp/test-fs-tsuru"
	unknownPath := "/tmp/test-fs-tsuru-unknown"
	os.Remove(unknownPath)
	// Remove the file even if the test fails.
	defer os.Remove(path)
	f, err := os.Create(path)
	c.Assert(err, check.IsNil)
	f.Close()
	fs := OsFs{}
	err = fs.Remove(path)
	c.Assert(err, check.IsNil)
	_, err = os.Stat(path)
	c.Assert(os.IsNotExist(err), check.Equals, true)
	err = fs.Remove(unknownPath)
	c.Assert(err, check.NotNil)
	c.Assert(os.IsNotExist(err), check.Equals, true)
}

func (s *S) TestOsFsRemoveAllDeletesDirectoryFromDisc(c *check.C) {
	path := "/tmp/tsuru/test-fs-tsuru"
	err := os.MkdirAll(path, 0755)
	c.Assert(err, check.IsNil)
	// Remove the directory even if the test fails.
	defer os.RemoveAll(path)
	fs := OsFs{}
	err = fs.RemoveAll(path)
	c.Assert(err, check.IsNil)
	_, err = os.Stat(path)
	c.Assert(os.IsNotExist(err), check.Equals, true)
}

func (s *S) TestOsFsRename(c *check.C) {
	path := "/tmp/tsuru/test-fs-tsuru"
	err := os.MkdirAll(path, 0755)
	c.Assert(err, check.IsNil)
	defer os.RemoveAll(path + ".old")
	fs := OsFs{}
	err = fs.Rename(path, path+".old")
	c.Assert(err, check.IsNil)
	_, err = os.Stat(path)
	c.Assert(os.IsNotExist(err), check.Equals, true)
	_, err = os.Stat(path + ".old")
	c.Assert(err, check.IsNil)
}

func (s *S) TestOsFsStatChecksTheFileInTheDisc(c *check.C) {
	path := "/tmp/test-fs-tsuru"
	unknownPath := "/tmp/test-fs-tsuru-unknown"
	os.Remove(unknownPath)
	defer os.Remove(path)
	f, err := os.Create(path)
	c.Assert(err, check.IsNil)
	f.Close()
	fs := OsFs{}
	_, err = fs.Stat(path)
	c.Assert(err, check.IsNil)
	_, err = fs.Stat(unknownPath)
	c.Assert(os.IsNotExist(err), check.Equals, true)
}
