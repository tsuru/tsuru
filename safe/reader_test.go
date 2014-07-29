// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package safe

import (
	"bytes"
	"launchpad.net/gocheck"
)

func (s *S) TestSafeBytesReaderLen(c *gocheck.C) {
	content := []byte("something")
	reader := NewBytesReader(content)
	length := reader.Len()
	c.Assert(length, gocheck.Equals, len(content))
}

func (s *S) TestSafeBytesReaderRead(c *gocheck.C) {
	var buf [4]byte
	content := []byte("something")
	reader := NewBytesReader(content)
	n, err := reader.Read(buf[:])
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 4)
	c.Assert(string(buf[:]), gocheck.Equals, "some")
}

func (s *S) TestSafeBytesReaderReadAt(c *gocheck.C) {
	var buf [4]byte
	content := []byte("something")
	reader := NewBytesReader(content)
	n, err := reader.ReadAt(buf[:], 1)
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 4)
	c.Assert(string(buf[:]), gocheck.Equals, "omet")
}

func (s *S) TestSafeBytesReaderReadByte(c *gocheck.C) {
	content := []byte("something")
	reader := NewBytesReader(content)
	b, err := reader.ReadByte()
	c.Assert(err, gocheck.IsNil)
	c.Assert(b, gocheck.Equals, content[0])
	b, err = reader.ReadByte()
	c.Assert(err, gocheck.IsNil)
	c.Assert(b, gocheck.Equals, content[1])
}

func (s *S) TestSafeBytesReaderReadRune(c *gocheck.C) {
	content := []byte("something")
	reader := NewBytesReader(content)
	b, size, err := reader.ReadRune()
	c.Assert(err, gocheck.IsNil)
	c.Assert(size, gocheck.Equals, 1)
	c.Assert(b, gocheck.Equals, 's')
}

func (s *S) TestSafeBytesReaderSeek(c *gocheck.C) {
	content := []byte("something")
	reader := NewBytesReader(content)
	b, err := reader.ReadByte()
	c.Assert(err, gocheck.IsNil)
	c.Assert(b, gocheck.Equals, content[0])
	off, err := reader.Seek(0, 0)
	c.Assert(err, gocheck.IsNil)
	c.Assert(off, gocheck.Equals, int64(0))
	b, err = reader.ReadByte()
	c.Assert(err, gocheck.IsNil)
	c.Assert(b, gocheck.Equals, content[0])
}

func (s *S) TestSafeBytesReaderUnreadByte(c *gocheck.C) {
	content := []byte("something")
	reader := NewBytesReader(content)
	b, err := reader.ReadByte()
	c.Assert(err, gocheck.IsNil)
	c.Assert(b, gocheck.Equals, content[0])
	err = reader.UnreadByte()
	c.Assert(err, gocheck.IsNil)
	b, err = reader.ReadByte()
	c.Assert(err, gocheck.IsNil)
	c.Assert(b, gocheck.Equals, content[0])
}

func (s *S) TestSafeBytesReaderUnreadRune(c *gocheck.C) {
	content := []byte("something")
	reader := NewBytesReader(content)
	b, size, err := reader.ReadRune()
	c.Assert(err, gocheck.IsNil)
	c.Assert(size, gocheck.Equals, 1)
	c.Assert(b, gocheck.Equals, 's')
	err = reader.UnreadRune()
	c.Assert(err, gocheck.IsNil)
	b, size, err = reader.ReadRune()
	c.Assert(err, gocheck.IsNil)
	c.Assert(size, gocheck.Equals, 1)
	c.Assert(b, gocheck.Equals, 's')
}

func (s *S) TestSafeBytesReaderWriteTo(c *gocheck.C) {
	var buf bytes.Buffer
	content := []byte("something")
	reader := NewBytesReader(content)
	n, err := reader.WriteTo(&buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, int64(len(content)))
	c.Assert(buf.String(), gocheck.Equals, "something")
}
