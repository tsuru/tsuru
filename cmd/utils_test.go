// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"io/ioutil"
	"os"
	"path"

	"github.com/tsuru/gnuflag"
	"github.com/tsuru/tsuru/fs/fstest"
	"gopkg.in/check.v1"
)

func (s *S) TestJoinWithUserDir(c *check.C) {
	expected := path.Join(os.Getenv("HOME"), "a", "b")
	path := JoinWithUserDir("a", "b")
	c.Assert(path, check.Equals, expected)
}

func (s *S) TestJoinWithUserDirHomePath(c *check.C) {
	defer os.Setenv("HOME", os.Getenv("HOME"))
	os.Setenv("HOME", "")
	os.Setenv("HOMEPATH", "/wat")
	path := JoinWithUserDir("a", "b")
	c.Assert(path, check.Equals, "/wat/a/b")
}

func (s *S) TestWriteToken(c *check.C) {
	rfs := &fstest.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	err := writeToken("abc")
	c.Assert(err, check.IsNil)
	tokenPath := JoinWithUserDir(".tsuru", "token")
	c.Assert(err, check.IsNil)
	c.Assert(rfs.HasAction("create "+tokenPath), check.Equals, true)
	fil, _ := fsystem.Open(tokenPath)
	b, _ := ioutil.ReadAll(fil)
	c.Assert(string(b), check.Equals, "abc")
}

func (s *S) TestReadToken(c *check.C) {
	os.Unsetenv("TSURU_TOKEN")
	rfs := &fstest.RecordingFs{FileContent: "123"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	token, err := ReadToken()
	c.Assert(err, check.IsNil)
	tokenPath := JoinWithUserDir(".tsuru", "token")
	c.Assert(err, check.IsNil)
	c.Assert(rfs.HasAction("open "+tokenPath), check.Equals, true)
	c.Assert(token, check.Equals, "123")
}

func (s *S) TestReadTokenFileNotFound(c *check.C) {
	os.Unsetenv("TSURU_TOKEN")
	errFs := &fstest.FileNotFoundFs{}
	fsystem = errFs
	defer func() {
		fsystem = nil
	}()
	token, err := ReadToken()
	c.Assert(err, check.IsNil)
	tokenPath := JoinWithUserDir(".tsuru", "token")
	c.Assert(err, check.IsNil)
	c.Assert(errFs.HasAction("open "+tokenPath), check.Equals, true)
	c.Assert(token, check.Equals, "")
}

func (s *S) TestShowServicesInstancesList(c *check.C) {
	expected := `+----------+-----------+
| Services | Instances |
+----------+-----------+
| mongodb  | my_nosql  |
+----------+-----------+
`
	b := `[{"service": "mongodb", "instances": ["my_nosql"]}]`
	result, err := ShowServicesInstancesList([]byte(b))
	c.Assert(err, check.IsNil)
	c.Assert(string(result), check.Equals, expected)
}

func (s *S) TestMergeFlagSet(c *check.C) {
	var x, y bool
	fs1 := gnuflag.NewFlagSet("x", gnuflag.ExitOnError)
	fs1.BoolVar(&x, "x", false, "Something")
	fs2 := gnuflag.NewFlagSet("y", gnuflag.ExitOnError)
	fs2.BoolVar(&y, "y", false, "Something")
	ret := MergeFlagSet(fs1, fs2)
	c.Assert(ret, check.Equals, fs1)
	fs1.Parse(true, []string{"-x", "-y"})
	c.Assert(x, check.Equals, true)
	c.Assert(y, check.Equals, true)
}
