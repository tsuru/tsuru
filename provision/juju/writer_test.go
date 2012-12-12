// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import (
	. "launchpad.net/gocheck"
	"net/http/httptest"
)

func (s *S) TestWriter(c *C) {
	recorder := httptest.NewRecorder()
	writer := Writer{recorder}
	data := []byte("ble")
	_, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.Bytes(), DeepEquals, data)
}

func (s *S) TestWriterShouldReturnTheDataSize(c *C) {
	recorder := httptest.NewRecorder()
	writer := Writer{recorder}
	data := []byte("ble")
	n, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, len(data))
}

func (s *S) TestWriterShouldNotFilterWhenTheContentTypeIsntText(c *C) {
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "application/xml")
	writer := Writer{recorder}
	data := []byte("2012-11-28 16:00:35,615 WARNING Ubuntu Cloud Image lookups encrypted but not authenticated")
	_, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.Bytes(), DeepEquals, data)
}

func (s *S) TestWriterShouldFilterWhenTheContentTypeIsText(c *C) {
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "text")
	writer := Writer{recorder}
	data := []byte("2012-11-28 16:00:35,615 WARNING Ubuntu Cloud Image lookups encrypted but not authenticated")
	_, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(len(recorder.Body.Bytes()), Equals, 0)
}

func (s *S) TestWriterShouldReturnTheOriginalLength(c *C) {
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "text")
	writer := Writer{recorder}
	data := []byte("2012-11-28 16:00:35,615 WARNING Ubuntu Cloud Image lookups encrypted but not authenticated")
	expected := len(data)
	n, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(len(recorder.Body.Bytes()), Equals, 0)
	c.Assert(n, Equals, expected)
}
