// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"github.com/globocom/tsuru/fs/testing"
	"io/ioutil"
	. "launchpad.net/gocheck"
)

func (s *S) TestWriteToken(c *C) {
	rfs := &testing.RecordingFs{}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	err := writeToken("abc")
	c.Assert(err, IsNil)
	tokenPath, err := joinWithUserDir(".tsuru_token")
	c.Assert(err, IsNil)
	c.Assert(rfs.HasAction("create "+tokenPath), Equals, true)
	fil, _ := fsystem.Open(tokenPath)
	b, _ := ioutil.ReadAll(fil)
	c.Assert(string(b), Equals, "abc")
}

func (s *S) TestReadToken(c *C) {
	rfs := &testing.RecordingFs{FileContent: "123"}
	fsystem = rfs
	defer func() {
		fsystem = nil
	}()
	token, err := readToken()
	c.Assert(err, IsNil)
	tokenPath, err := joinWithUserDir(".tsuru_token")
	c.Assert(err, IsNil)
	c.Assert(rfs.HasAction("open "+tokenPath), Equals, true)
	c.Assert(token, Equals, "123")
}

func (s *S) TestShowServicesInstancesList(c *C) {
	expected := `+----------+-----------+
| Services | Instances |
+----------+-----------+
| mongodb  | my_nosql  |
+----------+-----------+
`
	b := `[{"service": "mongodb", "instances": ["my_nosql"]}]`
	result, err := ShowServicesInstancesList([]byte(b))
	c.Assert(err, IsNil)
	c.Assert(string(result), Equals, expected)
}
