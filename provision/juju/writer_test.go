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
	writer := Writer{recorder, false}
	data := []byte("ble")
	_, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.Bytes(), DeepEquals, data)
	c.Assert(writer.wrote, Equals, true)
}

func (s *S) TestWriterShouldReturnTheDataSize(c *C) {
	recorder := httptest.NewRecorder()
	writer := Writer{recorder, false}
	data := []byte("ble")
	n, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, len(data))
}

func (s *S) TestWriterHeader(c *C) {
	recorder := httptest.NewRecorder()
	writer := Writer{recorder, false}
	writer.Header().Set("Content-Type", "application/xml")
	c.Assert(recorder.Header().Get("Content-Type"), Equals, "application/xml")
}

func (s *S) TestWriterWriteHeader(c *C) {
	recorder := httptest.NewRecorder()
	writer := Writer{recorder, false}
	expectedCode := 333
	writer.WriteHeader(expectedCode)
	c.Assert(recorder.Code, Equals, expectedCode)
	c.Assert(writer.wrote, Equals, true)
}

func (s *S) TestWriterShouldNotFilterWhenTheContentTypeIsntText(c *C) {
	recorder := httptest.NewRecorder()
	writer := Writer{recorder, false}
	writer.Header().Set("Content-Type", "application/xml")
	data := []byte("2012-11-28 16:00:35,615 WARNING Ubuntu Cloud Image lookups encrypted but not authenticated")
	_, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.Bytes(), DeepEquals, data)
}

func (s *S) TestWriterShouldFilterWhenTheContentTypeIsText(c *C) {
	recorder := httptest.NewRecorder()
	writer := Writer{recorder, false}
	writer.Header().Set("Content-Type", "text")
	data := []byte("2012-11-28 16:00:35,615 WARNING Ubuntu Cloud Image lookups encrypted but not authenticated")
	_, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(len(recorder.Body.Bytes()), Equals, 0)
}

func (s *S) TestWriterShouldReturnTheOriginalLength(c *C) {
	recorder := httptest.NewRecorder()
	writer := Writer{recorder, false}
	writer.Header().Set("Content-Type", "text")
	data := []byte("2012-11-28 16:00:35,615 WARNING Ubuntu Cloud Image lookups encrypted but not authenticated")
	expected := len(data)
	n, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(len(recorder.Body.Bytes()), Equals, 0)
	c.Assert(n, Equals, expected)
}
