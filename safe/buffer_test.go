// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package safe

import (
	. "launchpad.net/gocheck"
	"testing"
)

type S struct{}

var _ = Suite(&S{})

func Test(t *testing.T) {
	TestingT(t)
}

func (s *S) TestSafeBufferBytes(c *C) {
	buf := NewBuffer([]byte("something"))
	c.Assert(buf.Bytes(), DeepEquals, []byte("something"))
}

func (s *S) TestSafeBufferLen(c *C) {
	buf := NewBuffer([]byte("something"))
	c.Assert(buf.Len(), Equals, 9)
}

func (s *S) TestSafeBufferNext(c *C) {
	buf := NewBuffer([]byte("something"))
	p := buf.Next(3)
	c.Assert(p, DeepEquals, []byte("som"))
	p = buf.Next(3)
	c.Assert(p, DeepEquals, []byte("eth"))
}

func (s *S) TestSafeBufferRead(c *C) {
	var p [8]byte
	input := []byte("something")
	buf := NewBuffer(input)
	n, err := buf.Read(p[:])
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 8)
	c.Assert(p[:], DeepEquals, input[:8])
	n, err = buf.Read(p[:])
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 1)
	c.Assert(p[0], Equals, byte('g'))
}

func (s *S) TestSafeBufferReadByte(c *C) {
	buf := NewBuffer([]byte("something"))
	b, err := buf.ReadByte()
	c.Assert(err, IsNil)
	c.Assert(b, Equals, byte('s'))
	b, err = buf.ReadByte()
	c.Assert(err, IsNil)
	c.Assert(b, Equals, byte('o'))
}

func (s *S) TestSafeBufferReadBytes(c *C) {
	buf := NewBuffer([]byte("something"))
	b, err := buf.ReadBytes('n')
	c.Assert(err, IsNil)
	c.Assert(b, DeepEquals, []byte("somethin"))
}

func (s *S) TestSafeBufferReadFrom(c *C) {
	var p [9]byte
	buf1 := NewBuffer([]byte("something"))
	buf2 := NewBuffer(nil)
	n, err := buf2.ReadFrom(buf1)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, int64(9))
	x, err := buf2.Read(p[:])
	c.Assert(err, IsNil)
	c.Assert(x, Equals, 9)
	c.Assert(p[:], DeepEquals, []byte("something"))
}

func (s *S) TestSafeBufferReadRune(c *C) {
	buf := NewBuffer([]byte("something"))
	r, n, err := buf.ReadRune()
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 1)
	c.Assert(r, Equals, 's')
	r, n, err = buf.ReadRune()
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 1)
	c.Assert(r, Equals, 'o')
}

func (s *S) TestSafeBufferReadString(c *C) {
	buf := NewBuffer([]byte("something"))
	v, err := buf.ReadString('n')
	c.Assert(err, IsNil)
	c.Assert(v, Equals, "somethin")
}

func (s *S) TestSafeBufferReset(c *C) {
	var (
		buf Buffer
		p   [10]byte
	)
	n, err := buf.Write([]byte("something"))
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 9)
	buf.Reset()
	n, err = buf.Write([]byte("otherthing"))
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 10)
	n, err = buf.Read(p[:])
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 10)
	c.Assert(p[:], DeepEquals, []byte("otherthing"))
}

func (s *S) TestSafeBufferString(c *C) {
	buf := NewBuffer([]byte("something"))
	c.Assert(buf.String(), Equals, "something")
}

func (s *S) TestSafeBufferTruncate(c *C) {
	var (
		buf Buffer
		p   [9]byte
	)
	n, err := buf.Write([]byte("something"))
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 9)
	buf.Truncate(4)
	n, err = buf.Write([]byte("thong"))
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 5)
	n, err = buf.Read(p[:])
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 9)
	c.Assert(p[:], DeepEquals, []byte("somethong"))
}

func (s *S) TestSafeBufferUnreadByte(c *C) {
	buf := NewBuffer([]byte("something"))
	b, err := buf.ReadByte()
	c.Assert(err, IsNil)
	c.Assert(b, Equals, byte('s'))
	err = buf.UnreadByte()
	c.Assert(err, IsNil)
	b, err = buf.ReadByte()
	c.Assert(err, IsNil)
	c.Assert(b, Equals, byte('s'))
}

func (s *S) TestSafeBufferUnreadRune(c *C) {
	buf := NewBuffer([]byte("something"))
	r, _, err := buf.ReadRune()
	c.Assert(err, IsNil)
	c.Assert(r, Equals, 's')
	err = buf.UnreadRune()
	c.Assert(err, IsNil)
	r, _, err = buf.ReadRune()
	c.Assert(err, IsNil)
	c.Assert(r, Equals, 's')
}

func (s *S) TestSafeBufferWrite(c *C) {
	var p [9]byte
	buf := NewBuffer(nil)
	n, err := buf.Write([]byte("something"))
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 9)
	n, err = buf.Read(p[:])
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 9)
	c.Assert(p[:], DeepEquals, []byte("something"))
}

func (s *S) TestSafeBufferWriteByte(c *C) {
	var buf Buffer
	err := buf.WriteByte(byte('a'))
	c.Assert(err, IsNil)
	b, err := buf.ReadByte()
	c.Assert(err, IsNil)
	c.Assert(b, Equals, byte('a'))
}

func (s *S) TestSafeBufferWriteRune(c *C) {
	var buf Buffer
	n, err := buf.WriteRune('a')
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 1)
	r, n, err := buf.ReadRune()
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 1)
	c.Assert(r, Equals, 'a')
}

func (s *S) TestSafeBufferWriteTo(c *C) {
	var p [9]byte
	buf1 := NewBuffer([]byte("something"))
	buf2 := NewBuffer(nil)
	n, err := buf1.WriteTo(buf2)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, int64(9))
	x, err := buf2.Read(p[:])
	c.Assert(err, IsNil)
	c.Assert(x, Equals, 9)
	c.Assert(p[:], DeepEquals, []byte("something"))
}
