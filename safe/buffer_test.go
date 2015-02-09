// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package safe

import (
	"testing"

	"gopkg.in/check.v1"
)

type S struct{}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) TestSafeBufferBytes(c *check.C) {
	buf := NewBuffer([]byte("something"))
	c.Assert(buf.Bytes(), check.DeepEquals, []byte("something"))
}

func (s *S) TestSafeBufferLen(c *check.C) {
	buf := NewBuffer([]byte("something"))
	c.Assert(buf.Len(), check.Equals, 9)
}

func (s *S) TestSafeBufferNext(c *check.C) {
	buf := NewBuffer([]byte("something"))
	p := buf.Next(3)
	c.Assert(p, check.DeepEquals, []byte("som"))
	p = buf.Next(3)
	c.Assert(p, check.DeepEquals, []byte("eth"))
}

func (s *S) TestSafeBufferRead(c *check.C) {
	var p [8]byte
	input := []byte("something")
	buf := NewBuffer(input)
	n, err := buf.Read(p[:])
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 8)
	c.Assert(p[:], check.DeepEquals, input[:8])
	n, err = buf.Read(p[:])
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 1)
	c.Assert(p[0], check.Equals, byte('g'))
}

func (s *S) TestSafeBufferReadByte(c *check.C) {
	buf := NewBuffer([]byte("something"))
	b, err := buf.ReadByte()
	c.Assert(err, check.IsNil)
	c.Assert(b, check.Equals, byte('s'))
	b, err = buf.ReadByte()
	c.Assert(err, check.IsNil)
	c.Assert(b, check.Equals, byte('o'))
}

func (s *S) TestSafeBufferReadBytes(c *check.C) {
	buf := NewBuffer([]byte("something"))
	b, err := buf.ReadBytes('n')
	c.Assert(err, check.IsNil)
	c.Assert(b, check.DeepEquals, []byte("somethin"))
}

func (s *S) TestSafeBufferReadFrom(c *check.C) {
	var p [9]byte
	buf1 := NewBuffer([]byte("something"))
	buf2 := NewBuffer(nil)
	n, err := buf2.ReadFrom(buf1)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, int64(9))
	x, err := buf2.Read(p[:])
	c.Assert(err, check.IsNil)
	c.Assert(x, check.Equals, 9)
	c.Assert(p[:], check.DeepEquals, []byte("something"))
}

func (s *S) TestSafeBufferReadRune(c *check.C) {
	buf := NewBuffer([]byte("something"))
	r, n, err := buf.ReadRune()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 1)
	c.Assert(r, check.Equals, 's')
	r, n, err = buf.ReadRune()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 1)
	c.Assert(r, check.Equals, 'o')
}

func (s *S) TestSafeBufferReadString(c *check.C) {
	buf := NewBuffer([]byte("something"))
	v, err := buf.ReadString('n')
	c.Assert(err, check.IsNil)
	c.Assert(v, check.Equals, "somethin")
}

func (s *S) TestSafeBufferReset(c *check.C) {
	var (
		buf Buffer
		p   [10]byte
	)
	n, err := buf.Write([]byte("something"))
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 9)
	buf.Reset()
	n, err = buf.Write([]byte("otherthing"))
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 10)
	n, err = buf.Read(p[:])
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 10)
	c.Assert(p[:], check.DeepEquals, []byte("otherthing"))
}

func (s *S) TestSafeBufferString(c *check.C) {
	buf := NewBuffer([]byte("something"))
	c.Assert(buf.String(), check.Equals, "something")
}

func (s *S) TestSafeBufferTruncate(c *check.C) {
	var (
		buf Buffer
		p   [9]byte
	)
	n, err := buf.Write([]byte("something"))
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 9)
	buf.Truncate(4)
	n, err = buf.Write([]byte("thong"))
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 5)
	n, err = buf.Read(p[:])
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 9)
	c.Assert(p[:], check.DeepEquals, []byte("somethong"))
}

func (s *S) TestSafeBufferUnreadByte(c *check.C) {
	buf := NewBuffer([]byte("something"))
	b, err := buf.ReadByte()
	c.Assert(err, check.IsNil)
	c.Assert(b, check.Equals, byte('s'))
	err = buf.UnreadByte()
	c.Assert(err, check.IsNil)
	b, err = buf.ReadByte()
	c.Assert(err, check.IsNil)
	c.Assert(b, check.Equals, byte('s'))
}

func (s *S) TestSafeBufferUnreadRune(c *check.C) {
	buf := NewBuffer([]byte("something"))
	r, _, err := buf.ReadRune()
	c.Assert(err, check.IsNil)
	c.Assert(r, check.Equals, 's')
	err = buf.UnreadRune()
	c.Assert(err, check.IsNil)
	r, _, err = buf.ReadRune()
	c.Assert(err, check.IsNil)
	c.Assert(r, check.Equals, 's')
}

func (s *S) TestSafeBufferWrite(c *check.C) {
	var p [9]byte
	buf := NewBuffer(nil)
	n, err := buf.Write([]byte("something"))
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 9)
	n, err = buf.Read(p[:])
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 9)
	c.Assert(p[:], check.DeepEquals, []byte("something"))
}

func (s *S) TestSafeBufferWriteByte(c *check.C) {
	var buf Buffer
	err := buf.WriteByte(byte('a'))
	c.Assert(err, check.IsNil)
	b, err := buf.ReadByte()
	c.Assert(err, check.IsNil)
	c.Assert(b, check.Equals, byte('a'))
}

func (s *S) TestSafeBufferWriteRune(c *check.C) {
	var buf Buffer
	n, err := buf.WriteRune('a')
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 1)
	r, n, err := buf.ReadRune()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 1)
	c.Assert(r, check.Equals, 'a')
}

func (s *S) TestSafeBufferWriteTo(c *check.C) {
	var p [9]byte
	buf1 := NewBuffer([]byte("something"))
	buf2 := NewBuffer(nil)
	n, err := buf1.WriteTo(buf2)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, int64(9))
	x, err := buf2.Read(p[:])
	c.Assert(err, check.IsNil)
	c.Assert(x, check.Equals, 9)
	c.Assert(p[:], check.DeepEquals, []byte("something"))
}

func (s *S) TestSafeBufferWriteString(c *check.C) {
	buf := NewBuffer(nil)
	n, err := buf.WriteString("something")
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 9)
	var p [9]byte
	n, err = buf.Read(p[:])
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 9)
	c.Assert(p[:], check.DeepEquals, []byte("something"))
}
