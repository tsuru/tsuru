// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package safe

import (
	"bytes"
	"io/ioutil"

	"gopkg.in/check.v1"
)

func (s *S) TestSafeWriter(c *check.C) {
	var buf bytes.Buffer
	writer := NewWriter(&buf)
	writer.Write([]byte("hello world"))
	c.Assert(buf.String(), check.Equals, "hello world")
}

func (s *S) TestSafeReader(c *check.C) {
	buf := bytes.NewBufferString("hello world")
	reader := NewReader(buf)
	b, err := ioutil.ReadAll(reader)
	c.Assert(err, check.IsNil)
	c.Assert(string(b), check.Equals, "hello world")
}
