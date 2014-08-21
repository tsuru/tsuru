// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"io/ioutil"

	"github.com/tsuru/tsuru/fs/testing"
	"launchpad.net/gocheck"
)

func (s *S) TestWriteToken(c *gocheck.C) {
	rfs := &testing.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	err := writeToken("abc")
	c.Assert(err, gocheck.IsNil)
	tokenPath := JoinWithUserDir(".tsuru_token")
	c.Assert(err, gocheck.IsNil)
	c.Assert(rfs.HasAction("create "+tokenPath), gocheck.Equals, true)
	fil, _ := fsystem.Open(tokenPath)
	b, _ := ioutil.ReadAll(fil)
	c.Assert(string(b), gocheck.Equals, "abc")
}

func (s *S) TestReadToken(c *gocheck.C) {
	rfs := &testing.RecordingFs{FileContent: "123"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	token, err := ReadToken()
	c.Assert(err, gocheck.IsNil)
	tokenPath := JoinWithUserDir(".tsuru_token")
	c.Assert(err, gocheck.IsNil)
	c.Assert(rfs.HasAction("open "+tokenPath), gocheck.Equals, true)
	c.Assert(token, gocheck.Equals, "123")
}

func (s *S) TestShowServicesInstancesList(c *gocheck.C) {
	expected := `+----------+-----------+
| Services | Instances |
+----------+-----------+
| mongodb  | my_nosql  |
+----------+-----------+
`
	b := `[{"service": "mongodb", "instances": ["my_nosql"]}]`
	result, err := ShowServicesInstancesList([]byte(b))
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(result), gocheck.Equals, expected)
}
