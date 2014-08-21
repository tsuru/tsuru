// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"os"
	"testing"

	"launchpad.net/gocheck"
)

type S struct{}

var _ = gocheck.Suite(&S{})

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

func (s *S) TestOsFsImplementsFS(c *gocheck.C) {
	var _ Fs = OsFs{}
}

func (s *S) TestOsFsCreatesTheFileInTheDisc(c *gocheck.C) {
	path := "/tmp/test-fs-tsuru"
	os.Remove(path)
	defer os.Remove(path)
	fs := OsFs{}
	f, err := fs.Create(path)
	c.Assert(err, gocheck.IsNil)
	defer f.Close()
	_, err = os.Stat(path)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestOsFsOpenFile(c *gocheck.C) {
	path := "/tmp/test-fs-tsuru"
	os.Remove(path)
	defer os.Remove(path)
	fs := OsFs{}
	f, err := fs.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	c.Assert(err, gocheck.IsNil)
	defer f.Close()
	_, ok := f.(*os.File)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestOsFsMkdirWritesTheDirectoryInTheDisc(c *gocheck.C) {
	path := "/tmp/test-fs-tsuru"
	os.RemoveAll(path)
	defer os.RemoveAll(path)
	fs := OsFs{}
	err := fs.Mkdir(path, 0755)
	c.Assert(err, gocheck.IsNil)
	fi, err := os.Stat(path)
	c.Assert(err, gocheck.IsNil)
	c.Assert(fi.IsDir(), gocheck.Equals, true)
}

func (s *S) TestOsFsMkdirAllWritesAllDirectoriesInTheDisc(c *gocheck.C) {
	root := "/tmp/test-fs-tsuru"
	path := root + "/path"
	paths := []string{root, path}
	for _, path := range paths {
		os.RemoveAll(path)
		defer os.RemoveAll(path)
	}
	fs := OsFs{}
	err := fs.MkdirAll(path, 0755)
	c.Assert(err, gocheck.IsNil)
	for _, path := range paths {
		fi, err := os.Stat(path)
		c.Assert(err, gocheck.IsNil)
		c.Assert(fi.IsDir(), gocheck.Equals, true)
	}
}

func (s *S) TestOsFsOpenOpensTheFileFromDisc(c *gocheck.C) {
	path := "/tmp/test-fs-tsuru"
	unknownPath := "/tmp/test-fs-tsuru-unknown"
	os.Remove(unknownPath)
	defer os.Remove(path)
	f, err := os.Create(path)
	c.Assert(err, gocheck.IsNil)
	f.Close()
	fs := OsFs{}
	file, err := fs.Open(path)
	c.Assert(err, gocheck.IsNil)
	file.Close()
	_, err = fs.Open(unknownPath)
	c.Assert(err, gocheck.NotNil)
	c.Assert(os.IsNotExist(err), gocheck.Equals, true)
}

func (s *S) TestOsFsRemoveDeletesTheFileFromDisc(c *gocheck.C) {
	path := "/tmp/test-fs-tsuru"
	unknownPath := "/tmp/test-fs-tsuru-unknown"
	os.Remove(unknownPath)
	// Remove the file even if the test fails.
	defer os.Remove(path)
	f, err := os.Create(path)
	c.Assert(err, gocheck.IsNil)
	f.Close()
	fs := OsFs{}
	err = fs.Remove(path)
	c.Assert(err, gocheck.IsNil)
	_, err = os.Stat(path)
	c.Assert(os.IsNotExist(err), gocheck.Equals, true)
	err = fs.Remove(unknownPath)
	c.Assert(err, gocheck.NotNil)
	c.Assert(os.IsNotExist(err), gocheck.Equals, true)
}

func (s *S) TestOsFsRemoveAllDeletesDirectoryFromDisc(c *gocheck.C) {
	path := "/tmp/tsuru/test-fs-tsuru"
	err := os.MkdirAll(path, 0755)
	c.Assert(err, gocheck.IsNil)
	// Remove the directory even if the test fails.
	defer os.RemoveAll(path)
	fs := OsFs{}
	err = fs.RemoveAll(path)
	c.Assert(err, gocheck.IsNil)
	_, err = os.Stat(path)
	c.Assert(os.IsNotExist(err), gocheck.Equals, true)
}

func (s *S) TestOsFsRename(c *gocheck.C) {
	path := "/tmp/tsuru/test-fs-tsuru"
	err := os.MkdirAll(path, 0755)
	c.Assert(err, gocheck.IsNil)
	defer os.RemoveAll(path + ".old")
	fs := OsFs{}
	err = fs.Rename(path, path+".old")
	c.Assert(err, gocheck.IsNil)
	_, err = os.Stat(path)
	c.Assert(os.IsNotExist(err), gocheck.Equals, true)
	_, err = os.Stat(path + ".old")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestOsFsStatChecksTheFileInTheDisc(c *gocheck.C) {
	path := "/tmp/test-fs-tsuru"
	unknownPath := "/tmp/test-fs-tsuru-unknown"
	os.Remove(unknownPath)
	defer os.Remove(path)
	f, err := os.Create(path)
	c.Assert(err, gocheck.IsNil)
	f.Close()
	fs := OsFs{}
	_, err = fs.Stat(path)
	c.Assert(err, gocheck.IsNil)
	_, err = fs.Stat(unknownPath)
	c.Assert(os.IsNotExist(err), gocheck.Equals, true)
}
