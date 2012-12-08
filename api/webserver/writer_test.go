// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	. "launchpad.net/gocheck"
	"net/http/httptest"
)

func (s *S) TestFilteredWriter(c *C) {
	recorder := httptest.NewRecorder()
	writer := FilteredWriter{recorder, false}
	data := []byte("ble")
	_, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.Bytes(), DeepEquals, data)
	c.Assert(writer.wrote, Equals, true)
}

func (s *S) TestFilteredWriterShouldReturnTheDataSize(c *C) {
	recorder := httptest.NewRecorder()
	writer := FilteredWriter{recorder, false}
	data := []byte("ble")
	n, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, len(data))
}

func (s *S) TestFilteredWriterHeader(c *C) {
	recorder := httptest.NewRecorder()
	writer := FilteredWriter{recorder, false}
	writer.Header().Set("Content-Type", "application/xml")
	c.Assert(recorder.Header().Get("Content-Type"), Equals, "application/xml")
}

func (s *S) TestFilteredWriterWriteHeader(c *C) {
	recorder := httptest.NewRecorder()
	writer := FilteredWriter{recorder, false}
	expectedCode := 333
	writer.WriteHeader(expectedCode)
	c.Assert(recorder.Code, Equals, expectedCode)
	c.Assert(writer.wrote, Equals, true)
}

func (s *S) TestFilteredWriterShouldNotFilterWhenTheContentTypeIsntText(c *C) {
	recorder := httptest.NewRecorder()
	writer := FilteredWriter{recorder, false}
	writer.Header().Set("Content-Type", "application/xml")
	data := []byte("2012-11-28 16:00:35,615 WARNING Ubuntu Cloud Image lookups encrypted but not authenticated")
	_, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.Bytes(), DeepEquals, data)
}

func (s *S) TestFilteredWriterShouldFilterWhenTheContentTypeIsText(c *C) {
	recorder := httptest.NewRecorder()
	writer := FilteredWriter{recorder, false}
	writer.Header().Set("Content-Type", "text")
	data := []byte("2012-11-28 16:00:35,615 WARNING Ubuntu Cloud Image lookups encrypted but not authenticated")
	_, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(len(recorder.Body.Bytes()), Equals, 0)
}

func (s *S) TestFilteredWriterShouldReturnTheOriginalLength(c *C) {
	recorder := httptest.NewRecorder()
	writer := FilteredWriter{recorder, false}
	writer.Header().Set("Content-Type", "text")
	data := []byte("2012-11-28 16:00:35,615 WARNING Ubuntu Cloud Image lookups encrypted but not authenticated")
	expected := len(data)
	n, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(len(recorder.Body.Bytes()), Equals, 0)
	c.Assert(n, Equals, expected)
}
