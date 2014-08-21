// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package safe

import (
	"bytes"
	"io/ioutil"

	"launchpad.net/gocheck"
)

func (s *S) TestSafeWriter(c *gocheck.C) {
	var buf bytes.Buffer
	writer := NewWriter(&buf)
	writer.Write([]byte("hello world"))
	c.Assert(buf.String(), gocheck.Equals, "hello world")
}

func (s *S) TestSafeReader(c *gocheck.C) {
	buf := bytes.NewBufferString("hello world")
	reader := NewReader(buf)
	b, err := ioutil.ReadAll(reader)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(b), gocheck.Equals, "hello world")
}
