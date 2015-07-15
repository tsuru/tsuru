// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

import (
	"bytes"
	"errors"

	"gopkg.in/check.v1"
)

func (s *S) TestNoErrorWriter(c *check.C) {
	var buf bytes.Buffer
	w := NoErrorWriter{Writer: &buf}
	n, err := w.Write([]byte("something"))
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 9)
	c.Assert(buf.String(), check.Equals, "something")
}

type errBuffer struct {
	bytes.Buffer
	err error
}

func (b *errBuffer) Write(data []byte) (int, error) {
	b.Buffer.Write(data)
	return len(data), b.err
}

func (s *S) TestNoErrorWriterAfterError(c *check.C) {
	buf := errBuffer{}
	w := NoErrorWriter{Writer: &buf}
	n, err := w.Write([]byte("a"))
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 1)
	c.Assert(buf.String(), check.Equals, "a")
	buf.err = errors.New("some")
	n, err = w.Write([]byte("b"))
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 1)
	c.Assert(buf.String(), check.Equals, "ab")
	n, err = w.Write([]byte("c"))
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 1)
	c.Assert(buf.String(), check.Equals, "ab")
}
