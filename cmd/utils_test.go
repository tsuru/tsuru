// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"io/ioutil"

	"github.com/tsuru/tsuru/fs/fstest"
	"gopkg.in/check.v1"
	"launchpad.net/gnuflag"
)

func (s *S) TestWriteToken(c *check.C) {
	rfs := &fstest.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	err := writeToken("abc")
	c.Assert(err, check.IsNil)
	tokenPath := JoinWithUserDir(".tsuru_token")
	c.Assert(err, check.IsNil)
	c.Assert(rfs.HasAction("create "+tokenPath), check.Equals, true)
	fil, _ := fsystem.Open(tokenPath)
	b, _ := ioutil.ReadAll(fil)
	c.Assert(string(b), check.Equals, "abc")
}

func (s *S) TestReadToken(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "123"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	token, err := ReadToken()
	c.Assert(err, check.IsNil)
	tokenPath := JoinWithUserDir(".tsuru_token")
	c.Assert(err, check.IsNil)
	c.Assert(rfs.HasAction("open "+tokenPath), check.Equals, true)
	c.Assert(token, check.Equals, "123")
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
