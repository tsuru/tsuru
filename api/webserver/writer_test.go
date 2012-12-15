// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	. "launchpad.net/gocheck"
	"net/http/httptest"
)

func (s *S) TestFlushingWriter(c *C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{recorder, false}
	data := []byte("ble")
	_, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.Bytes(), DeepEquals, data)
	c.Assert(writer.wrote, Equals, true)
}

func (s *S) TestFlushingWriterShouldReturnTheDataSize(c *C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{recorder, false}
	data := []byte("ble")
	n, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, len(data))
}

func (s *S) TestFlushingWriterHeader(c *C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{recorder, false}
	writer.Header().Set("Content-Type", "application/xml")
	c.Assert(recorder.Header().Get("Content-Type"), Equals, "application/xml")
}

func (s *S) TestFlushingWriterWriteHeader(c *C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{recorder, false}
	expectedCode := 333
	writer.WriteHeader(expectedCode)
	c.Assert(recorder.Code, Equals, expectedCode)
	c.Assert(writer.wrote, Equals, true)
}
