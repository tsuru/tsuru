// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"launchpad.net/gocheck"
)

func (s *S) TestFilter(c *gocheck.C) {
	var buf bytes.Buffer
	w := filter{w: &buf, content: []byte("gopher")}
	w.Write([]byte("hello there\n"))
	c.Assert(w.filtered, gocheck.Equals, false)
	n, err := w.Write([]byte("my name is gopher\n"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 18)
	c.Assert(w.filtered, gocheck.Equals, true)
	w.Write([]byte("what's your name?"))
	w.Write([]byte("my name is Gopher\n"))
	c.Assert(buf.String(), gocheck.Equals, "hello there\nwhat's your name?")
}
