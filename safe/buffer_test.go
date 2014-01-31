// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package safe

import (
	"launchpad.net/gocheck"
	"testing"
)

type S struct{}

var _ = gocheck.Suite(&S{})

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

func (s *S) TestSafeBufferBytes(c *gocheck.C) {
	buf := NewBuffer([]byte("something"))
	c.Assert(buf.Bytes(), gocheck.DeepEquals, []byte("something"))
}

func (s *S) TestSafeBufferLen(c *gocheck.C) {
	buf := NewBuffer([]byte("something"))
	c.Assert(buf.Len(), gocheck.Equals, 9)
}

func (s *S) TestSafeBufferNext(c *gocheck.C) {
	buf := NewBuffer([]byte("something"))
	p := buf.Next(3)
	c.Assert(p, gocheck.DeepEquals, []byte("som"))
	p = buf.Next(3)
	c.Assert(p, gocheck.DeepEquals, []byte("eth"))
}

func (s *S) TestSafeBufferRead(c *gocheck.C) {
	var p [8]byte
	input := []byte("something")
	buf := NewBuffer(input)
	n, err := buf.Read(p[:])
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 8)
	c.Assert(p[:], gocheck.DeepEquals, input[:8])
	n, err = buf.Read(p[:])
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 1)
	c.Assert(p[0], gocheck.Equals, byte('g'))
}

func (s *S) TestSafeBufferReadByte(c *gocheck.C) {
	buf := NewBuffer([]byte("something"))
	b, err := buf.ReadByte()
	c.Assert(err, gocheck.IsNil)
	c.Assert(b, gocheck.Equals, byte('s'))
	b, err = buf.ReadByte()
	c.Assert(err, gocheck.IsNil)
	c.Assert(b, gocheck.Equals, byte('o'))
}

func (s *S) TestSafeBufferReadBytes(c *gocheck.C) {
	buf := NewBuffer([]byte("something"))
	b, err := buf.ReadBytes('n')
	c.Assert(err, gocheck.IsNil)
	c.Assert(b, gocheck.DeepEquals, []byte("somethin"))
}

func (s *S) TestSafeBufferReadFrom(c *gocheck.C) {
	var p [9]byte
	buf1 := NewBuffer([]byte("something"))
	buf2 := NewBuffer(nil)
	n, err := buf2.ReadFrom(buf1)
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, int64(9))
	x, err := buf2.Read(p[:])
	c.Assert(err, gocheck.IsNil)
	c.Assert(x, gocheck.Equals, 9)
	c.Assert(p[:], gocheck.DeepEquals, []byte("something"))
}

func (s *S) TestSafeBufferReadRune(c *gocheck.C) {
	buf := NewBuffer([]byte("something"))
	r, n, err := buf.ReadRune()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 1)
	c.Assert(r, gocheck.Equals, 's')
	r, n, err = buf.ReadRune()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 1)
	c.Assert(r, gocheck.Equals, 'o')
}

func (s *S) TestSafeBufferReadString(c *gocheck.C) {
	buf := NewBuffer([]byte("something"))
	v, err := buf.ReadString('n')
	c.Assert(err, gocheck.IsNil)
	c.Assert(v, gocheck.Equals, "somethin")
}

func (s *S) TestSafeBufferReset(c *gocheck.C) {
	var (
		buf Buffer
		p   [10]byte
	)
	n, err := buf.Write([]byte("something"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 9)
	buf.Reset()
	n, err = buf.Write([]byte("otherthing"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 10)
	n, err = buf.Read(p[:])
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 10)
	c.Assert(p[:], gocheck.DeepEquals, []byte("otherthing"))
}

func (s *S) TestSafeBufferString(c *gocheck.C) {
	buf := NewBuffer([]byte("something"))
	c.Assert(buf.String(), gocheck.Equals, "something")
}

func (s *S) TestSafeBufferTruncate(c *gocheck.C) {
	var (
		buf Buffer
		p   [9]byte
	)
	n, err := buf.Write([]byte("something"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 9)
	buf.Truncate(4)
	n, err = buf.Write([]byte("thong"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 5)
	n, err = buf.Read(p[:])
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 9)
	c.Assert(p[:], gocheck.DeepEquals, []byte("somethong"))
}

func (s *S) TestSafeBufferUnreadByte(c *gocheck.C) {
	buf := NewBuffer([]byte("something"))
	b, err := buf.ReadByte()
	c.Assert(err, gocheck.IsNil)
	c.Assert(b, gocheck.Equals, byte('s'))
	err = buf.UnreadByte()
	c.Assert(err, gocheck.IsNil)
	b, err = buf.ReadByte()
	c.Assert(err, gocheck.IsNil)
	c.Assert(b, gocheck.Equals, byte('s'))
}

func (s *S) TestSafeBufferUnreadRune(c *gocheck.C) {
	buf := NewBuffer([]byte("something"))
	r, _, err := buf.ReadRune()
	c.Assert(err, gocheck.IsNil)
	c.Assert(r, gocheck.Equals, 's')
	err = buf.UnreadRune()
	c.Assert(err, gocheck.IsNil)
	r, _, err = buf.ReadRune()
	c.Assert(err, gocheck.IsNil)
	c.Assert(r, gocheck.Equals, 's')
}

func (s *S) TestSafeBufferWrite(c *gocheck.C) {
	var p [9]byte
	buf := NewBuffer(nil)
	n, err := buf.Write([]byte("something"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 9)
	n, err = buf.Read(p[:])
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 9)
	c.Assert(p[:], gocheck.DeepEquals, []byte("something"))
}

func (s *S) TestSafeBufferWriteByte(c *gocheck.C) {
	var buf Buffer
	err := buf.WriteByte(byte('a'))
	c.Assert(err, gocheck.IsNil)
	b, err := buf.ReadByte()
	c.Assert(err, gocheck.IsNil)
	c.Assert(b, gocheck.Equals, byte('a'))
}

func (s *S) TestSafeBufferWriteRune(c *gocheck.C) {
	var buf Buffer
	n, err := buf.WriteRune('a')
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 1)
	r, n, err := buf.ReadRune()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 1)
	c.Assert(r, gocheck.Equals, 'a')
}

func (s *S) TestSafeBufferWriteTo(c *gocheck.C) {
	var p [9]byte
	buf1 := NewBuffer([]byte("something"))
	buf2 := NewBuffer(nil)
	n, err := buf1.WriteTo(buf2)
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, int64(9))
	x, err := buf2.Read(p[:])
	c.Assert(err, gocheck.IsNil)
	c.Assert(x, gocheck.Equals, 9)
	c.Assert(p[:], gocheck.DeepEquals, []byte("something"))
}

func (s *S) TestSafeBufferWriteString(c *gocheck.C) {
	buf := NewBuffer(nil)
	n, err := buf.WriteString("something")
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 9)
	var p [9]byte
	n, err = buf.Read(p[:])
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 9)
	c.Assert(p[:], gocheck.DeepEquals, []byte("something"))
}
