// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package safe

import (
	"bytes"

	"gopkg.in/check.v1"
)

func (s *S) TestSafeBytesReaderLen(c *check.C) {
	content := []byte("something")
	reader := NewBytesReader(content)
	length := reader.Len()
	c.Assert(length, check.Equals, len(content))
}

func (s *S) TestSafeBytesReaderRead(c *check.C) {
	var buf [4]byte
	content := []byte("something")
	reader := NewBytesReader(content)
	n, err := reader.Read(buf[:])
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 4)
	c.Assert(string(buf[:]), check.Equals, "some")
}

func (s *S) TestSafeBytesReaderReadAt(c *check.C) {
	var buf [4]byte
	content := []byte("something")
	reader := NewBytesReader(content)
	n, err := reader.ReadAt(buf[:], 1)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 4)
	c.Assert(string(buf[:]), check.Equals, "omet")
}

func (s *S) TestSafeBytesReaderReadByte(c *check.C) {
	content := []byte("something")
	reader := NewBytesReader(content)
	b, err := reader.ReadByte()
	c.Assert(err, check.IsNil)
	c.Assert(b, check.Equals, content[0])
	b, err = reader.ReadByte()
	c.Assert(err, check.IsNil)
	c.Assert(b, check.Equals, content[1])
}

func (s *S) TestSafeBytesReaderReadRune(c *check.C) {
	content := []byte("something")
	reader := NewBytesReader(content)
	b, size, err := reader.ReadRune()
	c.Assert(err, check.IsNil)
	c.Assert(size, check.Equals, 1)
	c.Assert(b, check.Equals, 's')
}

func (s *S) TestSafeBytesReaderSeek(c *check.C) {
	content := []byte("something")
	reader := NewBytesReader(content)
	b, err := reader.ReadByte()
	c.Assert(err, check.IsNil)
	c.Assert(b, check.Equals, content[0])
	off, err := reader.Seek(0, 0)
	c.Assert(err, check.IsNil)
	c.Assert(off, check.Equals, int64(0))
	b, err = reader.ReadByte()
	c.Assert(err, check.IsNil)
	c.Assert(b, check.Equals, content[0])
}

func (s *S) TestSafeBytesReaderUnreadByte(c *check.C) {
	content := []byte("something")
	reader := NewBytesReader(content)
	b, err := reader.ReadByte()
	c.Assert(err, check.IsNil)
	c.Assert(b, check.Equals, content[0])
	err = reader.UnreadByte()
	c.Assert(err, check.IsNil)
	b, err = reader.ReadByte()
	c.Assert(err, check.IsNil)
	c.Assert(b, check.Equals, content[0])
}

func (s *S) TestSafeBytesReaderUnreadRune(c *check.C) {
	content := []byte("something")
	reader := NewBytesReader(content)
	b, size, err := reader.ReadRune()
	c.Assert(err, check.IsNil)
	c.Assert(size, check.Equals, 1)
	c.Assert(b, check.Equals, 's')
	err = reader.UnreadRune()
	c.Assert(err, check.IsNil)
	b, size, err = reader.ReadRune()
	c.Assert(err, check.IsNil)
	c.Assert(size, check.Equals, 1)
	c.Assert(b, check.Equals, 's')
}

func (s *S) TestSafeBytesReaderWriteTo(c *check.C) {
	var buf bytes.Buffer
	content := []byte("something")
	reader := NewBytesReader(content)
	n, err := reader.WriteTo(&buf)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, int64(len(content)))
	c.Assert(buf.String(), check.Equals, "something")
}
